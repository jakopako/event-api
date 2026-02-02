package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"slices"

	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/models"
	cache "github.com/patrickmn/go-cache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	nominatimSearchURL = "https://nominatim.openstreetmap.org/search?"
)

type GeolocCache struct {
	// for cityMemCache it's ok to use a map since we only
	// add existing locations to it and there are only so many locations
	// in the world.
	cityMemCache map[string]*models.MongoGeolocation
	// for the negative cache (non-existing locations & cities) we use a cache library
	// to be able to set expiration times and not worry about memory
	negMemCache   *cache.Cache
	cityColl      *mongo.Collection
	cityMu        sync.RWMutex
	venueMemCache map[string]*models.Venue
	venueColl     *mongo.Collection
	venueMu       sync.RWMutex
}

var GC *GeolocCache

func InitGeolocCache() {
	// this code assumes that the DB has already been initialized
	GC = &GeolocCache{
		cityMemCache:  make(map[string]*models.MongoGeolocation),
		negMemCache:   cache.New(10*time.Minute, 15*time.Minute),
		cityColl:      config.MI.DB.Collection("cities"),
		venueMemCache: make(map[string]*models.Venue),
		venueColl:     config.MI.DB.Collection("venues"),
	}
}

func LookupCityCoordinates(city, country string) (*models.MongoGeolocation, error) {
	// this function is used when inserting new events and not when a user enters a search.
	// Otherwise we risk flooding the external geo service.
	city = strings.ToLower(city)
	country = strings.ToLower(country)
	searchKey := city
	if country != "" {
		searchKey += fmt.Sprintf("+%s", country)
	}
	searchKey = strings.ReplaceAll(searchKey, " ", "+")

	// check memory cache
	GC.cityMu.RLock()
	coords, found := GC.cityMemCache[searchKey]
	GC.cityMu.RUnlock()
	if found {
		return coords, nil
	}

	// check database
	filter := bson.D{{Key: "name", Value: city}, {Key: "country", Value: country}}
	var result models.City
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := GC.cityColl.FindOne(ctx, filter).Decode(&result)

	// fetch if not in database and write
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// check if we've gotten a negative result from nominatim in the past
			nominatimErr, found := GC.negMemCache.Get(searchKey)
			if found {
				return nil, nominatimErr.(error)
			}

			geoLoc, err := queryNominatimForCityGeoloc(city, country)
			if err != nil {
				// write error to negative cache
				// we don't want to flood the external service
				GC.negMemCache.Set(searchKey, err, cache.DefaultExpiration)
				return nil, err
			}
			// write city to database and cache
			GC.cityMu.Lock()
			GC.cityMemCache[searchKey] = geoLoc
			GC.cityMu.Unlock()
			newCity := models.City{Name: city, Country: country, Geolocation: *geoLoc}
			_, err = GC.cityColl.InsertOne(ctx, newCity)
			return geoLoc, err
		} else {
			return nil, err
		}
	}
	// write to cache
	GC.cityMu.Lock()
	GC.cityMemCache[searchKey] = &result.Geolocation
	GC.cityMu.Unlock()
	return &result.Geolocation, nil
}

func AllMatchesCityCoordinates(city, country string) ([]*models.MongoGeolocation, error) {
	// We want all cities with the given name.
	// Since we do not know how many there are we search the database without consulting
	// the cache.
	city = strings.ToLower(city)
	country = strings.ToLower(country)
	var filter primitive.D
	if country == "" {
		filter = bson.D{{Key: "name", Value: city}}
	} else {
		filter = bson.D{{Key: "name", Value: city}, {Key: "country", Value: country}}
	}
	var geolocs []*models.MongoGeolocation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cursor, err := GC.cityColl.Find(ctx, filter, &options.FindOptions{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var city models.City
		cursor.Decode(&city)
		geolocs = append(geolocs, &city.Geolocation)
	}
	return geolocs, nil
}

func queryNominatimForCityGeoloc(city, country string) (*models.MongoGeolocation, error) {
	client := &http.Client{}
	params := url.Values{}
	params.Set("city", city)
	if country != "" {
		params.Set("country", country)
	}
	params.Set("format", "jsonv2")

	requestUrl := nominatimSearchURL + params.Encode()
	req, _ := http.NewRequest(http.MethodGet, requestUrl, nil)
	req.Header.Set("accept-language", "en-US")
	req.Header.Set("user-agent", "https://github.com/jakopako/event-api (uses Nominatim for geocoding)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nominatim returned non-200 status code: %d", resp.StatusCode)
	}

	var places []models.NominatimPlace
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &places); err != nil {
		return nil, err
	}
	// try to figure out whether the result is unambiguous enough
	j := 0
	max := min(len(places), 2)
	countries := map[string]bool{}
	for i := range max {
		// extract country from display name and filter out irrelevant results
		if isValidLocalityAddressType(places[i].AddressType) {
			if places[i].Importance > 0.4 || max == 1 {
				places[j] = places[i]
				j++
				tokens := strings.Split(places[i].DisplayName, ", ")
				countries[tokens[len(tokens)-1]] = true
			}
		}
	}
	places = places[:j]
	if len(places) > 0 {
		if len(countries) == 1 {
			lonFloat, err := strconv.ParseFloat(places[0].Lon, 64)
			if err != nil {
				return nil, err
			}
			latFloat, err := strconv.ParseFloat(places[0].Lat, 64)
			if err != nil {
				return nil, err
			}
			return &models.MongoGeolocation{
				GeoJSONType: "Point",
				Coordinates: []float64{lonFloat, latFloat},
			}, nil
		} else {
			return nil, fmt.Errorf("ambiguous results for coordinates of city %s. Found two possible countries: %v", city, countries)
		}
	}
	return nil, fmt.Errorf("no relevant coordinates found for %s, %s", city, country)
}

func isValidLocalityAddressType(addressType string) bool {
	// we only want to accept cities, towns and villages
	// UPDATE: I've added 'county' since Oslo, Norway has
	// addresstype 'county' in Nominatim
	validTypes := []string{"city", "town", "village", "county"}
	return slices.Contains(validTypes, addressType)
}

// LookupVenueLocation tries to find coordinates for a specific venue (location) in a city using Nominatim.
// Returns a Venue struct if found, otherwise returns nil and an error.
func LookupVenueLocation(location, city, country string) (*models.Address, error) {
	if location == "" || city == "" {
		return nil, fmt.Errorf("location and city must be provided for venue lookup")
	}
	venueKey := strings.ToLower(location) + "+" + strings.ToLower(city)
	if country != "" {
		venueKey += "+" + strings.ToLower(country)
	}
	venueKey = strings.ReplaceAll(venueKey, " ", "+")

	// Check memory cache first
	GC.cityMu.RLock()
	venueCached, found := GC.venueMemCache[venueKey]
	GC.cityMu.RUnlock()
	if found && venueCached != nil {
		return &venueCached.Address, nil
	}

	// Check database
	filter := bson.D{{Key: "name", Value: location}, {Key: "address.locality", Value: city}}
	if country != "" {
		filter = append(filter, bson.E{Key: "address.country", Value: country})
	}

	var result models.Venue
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := GC.venueColl.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// check if we've gotten a negative result from nominatim in the past
			nominatimErr, found := GC.negMemCache.Get(venueKey)
			if found {
				return nil, nominatimErr.(error)
			}

			// If not found in database, query Nominatim
			venue, err := queryNominatimForVenue(location, city, country)
			if err != nil {
				// Cache the error in negative cache to avoid flooding Nominatim
				GC.negMemCache.Set(venueKey, err, cache.DefaultExpiration)
				return nil, err
			}
			// Cache the venue in memory and database
			GC.venueMu.Lock()
			GC.venueMemCache[venueKey] = venue
			GC.venueMu.Unlock()
			_, err = GC.venueColl.InsertOne(ctx, venue)
			if err != nil {
				return nil, fmt.Errorf("failed to insert venue into database: %w", err)
			}
			return &venue.Address, nil
		} else {
			return nil, fmt.Errorf("failed to find venue in database: %w", err)
		}
	}
	// If found in database, cache it
	GC.venueMu.Lock()
	GC.venueMemCache[venueKey] = &result
	GC.venueMu.Unlock()
	return &result.Address, nil
}

func queryNominatimForVenue(location, city, country string) (*models.Venue, error) {
	client := &http.Client{}
	params := url.Values{}
	params.Set("amenity", location)
	params.Set("city", city)
	params.Set("addressdetails", "1")
	params.Set("format", "jsonv2")
	if country != "" {
		params.Set("country", country)
	}
	requestUrl := nominatimSearchURL + params.Encode()
	req, _ := http.NewRequest(http.MethodGet, requestUrl, nil)
	req.Header.Set("accept-language", "en-US")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var places []models.NominatimPlace
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &places); err != nil {
		return nil, err
	}
	if len(places) > 0 {
		for _, place := range places {
			if place.AddressType != "amenity" {
				continue
			}
			if !isValidAmenityType(place.Type) {
				continue
			}

			slog.Info("Found venue in Nominatim",
				"location", location,
				"city", city,
				"country", country,
				"place", place.Name,
				"address", place.Address,
				"importance", place.Importance,
				"type", place.Type,
			)

			lonFloat, err := strconv.ParseFloat(place.Lon, 64)
			if err != nil {
				return nil, err
			}
			latFloat, err := strconv.ParseFloat(place.Lat, 64)
			if err != nil {
				return nil, err
			}

			locality := place.Address.City
			if locality == "" {
				locality = place.Address.Town
			}
			if locality == "" {
				locality = place.Address.Village
			}
			if locality == "" {
				locality = city // fallback to the provided city if no locality is found
			}

			venue := &models.Venue{
				// For now, instead of place.Name, we use the location parameter.
				// We assume that if we found a venue in Nominatim, it is the one we are looking for
				// and we prefer to use the provided location name instead of the one from Nominatim
				Name: location,
				Type: place.Type,
				Address: models.Address{
					Locality:    locality,
					Country:     place.Address.Country,
					State:       place.Address.State,
					PostCode:    place.Address.Postcode,
					Street:      place.Address.Road,
					HouseNumber: place.Address.HouseNumber,
					Geolocacation: models.MongoGeolocation{
						GeoJSONType: "Point",
						Coordinates: []float64{lonFloat, latFloat},
					},
				},
			}

			return venue, nil
		}
	}
	slog.Warn("No relevant venue found in Nominatim",
		"location", location,
		"city", city,
		"country", country,
	)
	return nil, fmt.Errorf("no relevant info found for venue %s in city %s", location, city)
}

func isValidAmenityType(amenityType string) bool {
	validTypes := []string{"arts_centre", "bar", "cafe", "community_centre", "concert_hall", "events_centre", "events_venue", "mobility_hub", "music_school", "music_venue", "nightclub", "place_of_worship", "pub", "restaurant", "social_centre", "theatre", "university"}
	return slices.Contains(validTypes, amenityType)
}
