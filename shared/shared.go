package shared

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"
	"unicode"

	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/geo"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	EventCollectionName         = "events"
	NotificationCollectionName  = "notifications"
	ScraperStatusCollectionName = "status"
)

// RemoveDiacritics removes diacritical marks from a string
func RemoveDiacritics(s string) string {
	remover := runes.Remove(runes.Predicate(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}))
	t := transform.Chain(norm.NFD, remover, norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

func FetchEvents(q models.Query) ([]models.Event, int64, int64, error) {
	eventCollection := config.MI.DB.Collection(EventCollectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var events []models.Event

	if q.Page < 1 {
		return events, 0, 0, errors.New("page parameter must be greater than 0")
	}
	if q.Limit < 1 {
		return events, 0, 0, errors.New("limit parameter must be greater than 0")
	}
	if q.Radius < 0 {
		return events, 0, 0, errors.New("radius parameter must be greater than or equal to 0")
	}

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
	findOptions.SetSort(bson.D{{Key: "date", Value: 1}})

	// Special handling for title to include normalized search
	if q.Title != "" {
		normalizedTitle := RemoveDiacritics(q.Title)
		filter["$and"] = append(filter["$and"].([]bson.M), bson.M{
			"$or": []bson.M{
				{
					"title": bson.M{
						"$regex": primitive.Regex{
							Pattern: regexp.QuoteMeta(q.Title),
							Options: "i",
						},
					},
				},
				{
					"normalizedTitle": bson.M{
						"$regex": primitive.Regex{
							Pattern: regexp.QuoteMeta(normalizedTitle),
							Options: "i",
						},
					},
				},
			},
		})
	}

	for searchKey, searchValue := range map[string]string{"location": q.Location, "country": q.Country, "type": q.Type} {
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

	last := int64(math.Ceil(float64(total) / float64(q.Limit)))
	if last < 1 && total > 0 {
		last = 1
	}
	return events, total, last, nil
}
