package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

			geoLoc, err := fetchGeolocFromNominatim(searchKey)
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

func fetchGeolocFromNominatim(query string) (*models.MongoGeolocation, error) {
	client := &http.Client{}
	requestUrl := fmt.Sprintf("https://nominatim.openstreetmap.org/search.php?q=%s&format=jsonv2", url.QueryEscape(query))
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
	// try to figure out whether the result is unambiguous enough
	j := 0
	max := min(len(places), 2)
	countries := map[string]bool{}
	for i := range max {
		// extract country from display name and filter out irrelevant results
		if isValidAddressType(places[i].AddressType) {
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
			return nil, fmt.Errorf("ambiguous results for coordinates of city %s. Found two possible countries: %v", query, countries)
		}
	}
	return nil, fmt.Errorf("no relevant coordinates found for %s", query)
}

func isValidAddressType(addressType string) bool {
	// we only want to accept cities, towns and villages
	validTypes := []string{"city", "town", "village"}
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
	params.Set("format", "jsonv2")
	if country != "" {
		params.Set("country", country)
	}
	requestUrl := "https://nominatim.openstreetmap.org/search?" + params.Encode()
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
		if places[0].AddressType != "amenity" {
			return nil, fmt.Errorf("first result is not an amenity: %s", places[0].AddressType)
		}

		// TODO also check the type
		// e.g.     "type": "bicycle_rental", we don't want that

		lonFloat, err := strconv.ParseFloat(places[0].Lon, 64)
		if err != nil {
			return nil, err
		}
		latFloat, err := strconv.ParseFloat(places[0].Lat, 64)
		if err != nil {
			return nil, err
		}

		// TODO: don't rely on the display name, but rather use the address struct
		// that is returned by Nominatim when using the "addressdetails=1" parameter.
		addressParts := strings.Split(places[0].DisplayName, ", ")

		// let's do some sanity checks
		if len(addressParts) < 5 {
			return nil, fmt.Errorf("not enough address parts found in display name: %s", places[0].DisplayName)
		}

		nomCountry := addressParts[len(addressParts)-1]
		if country != "" && !strings.EqualFold(nomCountry, country) {
			return nil, fmt.Errorf("country mismatch: expected %s, got %s", country, nomCountry)
		}

		// addressParts[len(addressParts)-5] is not really always the city, so we compare it for now
		//
		// nomCity := addressParts[len(addressParts)-5]
		// if strings.ToLower(nomCity) != strings.ToLower(city) {
		// 	return nil, fmt.Errorf("city mismatch: expected %s, got %s", city, nomCity)
		// }

		nomPostalCode := addressParts[len(addressParts)-2]
		nomRegion := addressParts[len(addressParts)-3]
		nomStreet := addressParts[1]
		// if nomStreet contains digits, append addressParts[2] to it
		if strings.ContainsAny(nomStreet, "0123456789") && len(addressParts) > 2 {
			nomStreet += ", " + addressParts[2]
		}

		venue := &models.Venue{
			// For now, instead of places[0].Name, we use the location parameter.
			// We assume that if we found a venue in Nominatim, it is the one we are looking for
			// and we prefer to use the provided location name instead of the one from Nominatim
			Name: location,
			Address: models.Address{
				Locality:   city,
				Country:    nomCountry,
				Region:     nomRegion,
				PostalCode: nomPostalCode,
				Street:     nomStreet,
				Geolocacation: models.MongoGeolocation{
					GeoJSONType: "Point",
					Coordinates: []float64{lonFloat, latFloat},
				},
			},
		}

		return venue, nil
	}
	return nil, fmt.Errorf("no relevant coordinates found for venue %s in city %s", location, city)
}
