package shared

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/geo"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func FetchEvents(q models.Query) ([]models.Event, int64, float64, error) {
	eventCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var events []models.Event
	var filter primitive.M
	if q.StartDate != nil {
		if q.EndDate == nil {
			filter = bson.M{
				"$and": []bson.M{
					{
						"date": bson.M{
							"$gt": q.StartDate,
						},
					},
				},
			}
		} else {
			filter = bson.M{
				"$and": []bson.M{
					{
						"date": bson.M{
							"$gte": q.StartDate,
						},
					},
					{
						"date": bson.M{
							"$lte": q.EndDate,
						},
					},
				},
			}
		}
	} else {
		filter = bson.M{
			"$and": []bson.M{},
		}
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{"date", 1}})

	for searchKey, searchValue := range map[string]string{"title": q.Title, "location": q.Location, "country": q.Country, "type": q.Type} {
		if searchValue != "" {
			filter["$and"] = append(filter["$and"].([]bson.M), bson.M{
				searchKey: bson.M{
					"$regex": primitive.Regex{
						Pattern: regexp.QuoteMeta(searchValue),
						Options: "i",
					},
				},
			})
		}
	}

	if q.City != "" {
		cityFilter := bson.M{
			"$or": []bson.M{
				{
					"city": bson.M{
						"$regex": primitive.Regex{
							Pattern: q.City,
							Options: "i",
						},
					},
				},
			},
		}
		if q.Radius > 0 {
			// near in or not supported: https://jira.mongodb.org/browse/SERVER-13974
			if geolocs, err := geo.AllMatchesCityCoordinates(q.City, q.Country); err == nil && len(geolocs) > 0 {
				earthRadiusKm := 6378.1
				radiusFilter := bson.D{
					{Key: "address.geolocation", Value: bson.D{
						{Key: "$geoWithin", Value: bson.D{ // we need to use geoWithin for CountDocuments to properly work, see https://www.mongodb.com/docs/manual/reference/method/db.collection.countDocuments/#query-restrictions
							{Key: "$centerSphere", Value: bson.A{geolocs[0].Coordinates, float64(q.Radius) / earthRadiusKm}},
						}},
					}},
				}
				cityFilter["$or"] = append(cityFilter["$or"].([]bson.M), radiusFilter.Map())
			}
		}
		filter["$and"] = append(filter["$and"].([]bson.M), cityFilter)
	}

	total, _ := eventCollection.CountDocuments(ctx, filter)

	findOptions.SetSkip((int64(q.Page) - 1) * q.Limit)
	findOptions.SetLimit(q.Limit)

	cursor, err := eventCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return events, 0, 0, fmt.Errorf("events not found: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var event models.Event
		cursor.Decode(&event)
		events = append(events, event)
	}

	last := math.Ceil(float64(total) / float64(q.Limit))
	if last < 1 && total > 0 {
		last = 1
	}
	return events, total, last, nil
}
