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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GeolocCache struct {
	memCache map[string]*models.MongoGeolocation
	cityColl *mongo.Collection
	mu       sync.RWMutex
}

var GC *GeolocCache

func InitGeolocCache() {
	// this code assumes that the DB has already been initialized
	GC = &GeolocCache{
		memCache: make(map[string]*models.MongoGeolocation),
		cityColl: config.MI.DB.Collection("cities"),
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
	GC.mu.RLock()
	coords, found := GC.memCache[searchKey]
	GC.mu.RUnlock()
	if found {
		return coords, nil
	}

	// check database
	filter := bson.D{{"name", city}, {"country", country}}
	var result models.City
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err := GC.cityColl.FindOne(ctx, filter).Decode(&result)

	// fetch if not in database and write
	if err != nil {
		if err == mongo.ErrNoDocuments {
			geoLoc, err := fetchGeolocFromNominatim(searchKey)
			if err != nil {
				return nil, err
			}
			// write city to database and cache
			GC.mu.Lock()
			GC.memCache[searchKey] = geoLoc
			GC.mu.Unlock()
			newCity := models.City{Name: city, Country: country, Geolocation: *geoLoc}
			_, err = GC.cityColl.InsertOne(ctx, newCity)
			return geoLoc, err
		} else {
			return nil, err
		}
	}
	// write to cache
	GC.mu.Lock()
	GC.memCache[searchKey] = &result.Geolocation
	GC.mu.Unlock()
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
		filter = bson.D{{"name", city}}
	} else {
		filter = bson.D{{"name", city}, {"country", country}}
	}
	var geolocs []*models.MongoGeolocation
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
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
