package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func fetchGeolocFromNominatim(name string) (*models.MongoGeolocation, error) {
	// this will probably have to be refined in the future
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/search.php?q=%s&format=jsonv2", name)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var places []models.NominatimPlace
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &places); err != nil {
		return nil, err
	}
	if len(places) > 0 {
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
	}
	return nil, fmt.Errorf("no coordinates found for %s", name)
}

func translateCityToGeoLoc(name string, cityCollection *mongo.Collection, ctx context.Context) (*models.MongoGeolocation, error) {
	// check database
	filter := bson.D{{"name", name}}
	var result models.City
	err := cityCollection.FindOne(ctx, filter).Decode(&result)
	// fetch if not in database and write
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// fetch from nominatim openstreetmap
			// https://nominatim.openstreetmap.org/search.php?q=leuven&format=jsonv2
			geoLoc, err := fetchGeolocFromNominatim(name)
			if err != nil {
				return nil, err
			}
			// write city to database
			newCity := models.City{Name: name, Geolocation: *geoLoc}
			_, err = cityCollection.InsertOne(ctx, newCity)
			return geoLoc, err
		} else {
			return nil, err
		}
	}
	return &result.Geolocation, nil
}
