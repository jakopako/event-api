package controllers

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/geo"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/go-playground/validator.v9"
)

// GetAllEvents func gets all events.
// @Description This endpoint returns all events matching the search terms. Note that only events from today on will be returned, ie no past events.
// @Summary Get all events.
// @Tags events
// @Accept json
// @Produce json
// @Param title query string false "title search string"
// @Param location query string false "location search string"
// @Param city query string false "city search string"
// @Param country query string false "country search string"
// @Param radius query int false "radius around given city in kilometers"
// @Param date query string false "date search string"
// @Param page query int false "page number"
// @Param limit query int false "page size"
// @Success 200 {array} models.Event
// @Failure 404 {object} string "No events found"
// @Router /api/events [get]
func GetAllEvents(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var events []models.Event
	var filter primitive.M
	if dateString := c.Query("date"); dateString == "" {
		d := time.Now()
		today := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())

		filter = bson.M{
			"$and": []bson.M{
				{
					"date": bson.M{
						"$gt": today,
					},
				},
			},
		}
	} else {
		d, err := time.Parse(time.RFC3339, dateString)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Couldn't parse date",
				"error":   err.Error(),
			})
		}
		dayStart := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		dayEnd := time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, d.Location())
		filter = bson.M{
			"$and": []bson.M{
				{
					"date": bson.M{
						"$gte": dayStart,
					},
				},
				{
					"date": bson.M{
						"$lte": dayEnd,
					},
				},
			},
		}
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{"date", 1}})

	for _, searchString := range []string{"title", "location", "country"} {
		if s := c.Query(searchString); s != "" {
			filter["$and"] = append(filter["$and"].([]bson.M), bson.M{
				searchString: bson.M{
					"$regex": primitive.Regex{
						Pattern: s,
						Options: "i",
					},
				},
			})
		}
	}

	if city := c.Query("city"); city != "" {
		cityFilter := bson.M{
			"$or": []bson.M{
				{
					"city": bson.M{
						"$regex": primitive.Regex{
							Pattern: c.Query("city"),
							Options: "i",
						},
					},
				},
			},
		}
		radius, _ := strconv.Atoi(c.Query("radius", "0"))
		if radius > 0 {
			// near in or not supported: https://jira.mongodb.org/browse/SERVER-13974
			if geolocs, err := geo.AllMatchesCityCoordinates(c.Query("city"), c.Query("country")); err == nil && len(geolocs) > 0 {
				earthRadiusKm := 6378.1
				radiusFilter := bson.D{
					{"geolocation", bson.D{
						{"$geoWithin", bson.D{ // we need to use geoWithin for CountDocuments to properly work, see https://www.mongodb.com/docs/manual/reference/method/db.collection.countDocuments/#query-restrictions
							{"$centerSphere", bson.A{geolocs[0].Coordinates, float64(radius) / earthRadiusKm}},
						}},
					}},
				}
				cityFilter["$or"] = append(cityFilter["$or"].([]bson.M), radiusFilter.Map())
			}
		}
		filter["$and"] = append(filter["$and"].([]bson.M), cityFilter)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limitVal, _ := strconv.Atoi(c.Query("limit", "10"))
	var limit int64 = int64(limitVal)

	total, _ := eventCollection.CountDocuments(ctx, filter)

	findOptions.SetSkip((int64(page) - 1) * limit)
	findOptions.SetLimit(limit)

	cursor, err := eventCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "events not found",
			"error":   err.Error(),
		})
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var event models.Event
		cursor.Decode(&event)
		events = append(events, event)
	}

	last := math.Ceil(float64(total) / float64(limit))
	if last < 1 && total > 0 {
		last = 1
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data":      events,
		"total":     total,
		"page":      page,
		"last_page": last,
		"limit":     limit,
	})
}

// AddEvent func for adding new events to the database.
// @Description Add new events to the database.
// @Summary Add new events.
// @Tags events
// @Accept json
// @Produce json
// @Security BasicAuth
// @Param message body []models.Event true "Event Info"
// @Failure 400 {object} string "Failed to parse body"
// @Failure 500 {object} string "Failed to insert events"
// @Router /api/events [post]
func AddEvents(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	events := new([]models.Event)

	if err := c.BodyParser(events); err != nil {
		//log.Println(err)
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Failed to parse body",
			"error":   err.Error(),
		})
	}

	var operations []mongo.WriteModel
	validate := validator.New()
	for _, event := range *events {
		err := validate.Struct(event)
		if err != nil {
			//log.Println(err)
			return c.Status(400).JSON(fiber.Map{
				"succes":  false,
				"message": "Failed to parse body",
				"error":   err.Error(),
			})
		}

		// check geolocation
		if len(event.Geolocation) != 2 {
			// lookup location based on city AND cache this info to not flood the geoloc service
			// It's the client's responsibility to provide enough info (ie country if necessary)
			// for the following function to find the right coordinates
			geoLoc, err := geo.LookupCityCoordinates(event.City, event.Country)
			if err != nil {
				// maybe we shouldn't return on error but just leave the location empty...
				return c.Status(500).JSON(fiber.Map{
					"success": false,
					"message": fmt.Sprintf("Failed to find city location of city %s", event.City),
					"error":   err.Error(),
				})
			}
			event.MongoGeolocation = *geoLoc

		} else {
			event.MongoGeolocation.GeoJSONType = "Point"
			event.MongoGeolocation.Coordinates = event.Geolocation[:]
		}

		op := mongo.NewReplaceOneModel()
		// The filter ignores the comment assuming that the comment might be updated over time.
		// In future versions we might need to take more factors into account to decide whether
		// an existing event needs to be updated or a new event needs to be added.
		// UPDATE: the following filter does not work anymore with the additional geolocation field.
		// filterEvent := models.Event{
		// 	Title:     event.Title,
		// 	Date:      event.Date,
		// 	Location:  event.Location,
		// 	URL:       event.URL,
		// 	SourceURL: event.SourceURL,
		// }
		filterEvent := bson.D{
			{"title", event.Title},
			{"date", event.Date},
			{"location", event.Location},
			{"url", event.URL},
			{"sourceUrl", event.SourceURL}}
		op.SetFilter(filterEvent)
		op.SetUpsert(true)
		op.SetReplacement(event)
		operations = append(operations, op)
	}

	bulkOption := options.BulkWriteOptions{}
	bulkOption.SetOrdered(true)
	result, err := eventCollection.BulkWrite(ctx, operations, &bulkOption)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to insert events",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data":    result,
		"success": true,
		"message": "Events inserted successfully",
	})

}

// GetTodayseventsSlack func for retrieving today's events, formatted as md for slack.
// @Description This endpoint returns today's events in a format that slack needs for its slash command. Currently, Zurich is hardcoded as city (will be changed).
// @Summary Get today's events.
// @Tags events
// @Accept json
// @Produce json
// @Success 200 {object} string "A json with the results"
// @Router /api/events/today/slack [post]
func GetTodaysEventsSlack(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var events []models.Event
	d := time.Now()
	today := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
	tomorrow := time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, d.Location())

	city := "zurich" // TODO: read from post body.

	filter := bson.M{
		"$and": []bson.M{
			{
				"date": bson.M{
					"$gte": today,
				},
			},
			{
				"date": bson.M{
					"$lte": tomorrow,
				},
			},
			{
				"city": bson.M{
					"$regex": primitive.Regex{
						Pattern: city,
						Options: "i",
					},
				},
			},
		},
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{"date", 1}})

	total, _ := eventCollection.CountDocuments(ctx, filter)
	if total == 0 {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"response_type": "ephemeral",
			"text":          "Sorry, no events tonight.",
		})
	}

	cursor, err := eventCollection.Find(ctx, filter, findOptions)
	defer cursor.Close(ctx)

	if err != nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"response_type": "ephemeral",
			"text":          "Sorry, something went wrong.",
		})
	}

	for cursor.Next(ctx) {
		var event models.Event
		cursor.Decode(&event)
		events = append(events, event)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"response_type": "ephemeral",
		"blocks": []fiber.Map{
			{
				"type": "section",
				"text": fiber.Map{
					"type": "mrkdwn",
					"text": GetMarkdownSummary(events),
				},
			},
		},
	})
}

// DeleteEvents func for deleting events.
// @Description Delete events.
// @Summary Delete events.
// @Tags events
// @Accept json
// @Produce json
// @Security BasicAuth
// @Param sourceUrl query string false "sourceUrl string"
// @Param datetime query string false "datetime string"
// @Success 200 {object} string "A success message"
// @Failure 500 {object} string "Failed to delete events"
// @Router /api/events [delete]
func DeleteEvents(c *fiber.Ctx) error {
	eventsCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	src := c.Query("sourceUrl")

	datetimeString := c.Query("datetime")
	var filter bson.M
	if datetimeString == "" {
		filter = bson.M{"sourceUrl": src}
	} else {
		t, err := time.Parse("2006-01-02 15:04", datetimeString)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Couldn't parse datetime",
				"error":   err,
			})
		}
		if src == "" {
			filter = bson.M{"date": bson.M{"$gte": t}}
		} else {
			filter = bson.M{
				"$and": []bson.M{
					{
						"date": bson.M{
							"$gte": t,
						},
					},
					{
						"sourceUrl": src,
					},
				},
			}
		}
	}

	result, err := eventsCollection.DeleteMany(ctx, filter)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": fmt.Sprintf("Failed to delete events from source %s", src),
			"error":   err,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("Successfully deleted %d events with source %s", result.DeletedCount, src),
	})
}

// GetDistinct func for getting distinct field values.
// @Description This endpoint returns all distinct values for the given field. Note that past events are not considered for this query.
// @Summary Get distinct field values.
// @Tags events
// @Produce json
// @Param field path string true "field name, can only be location or city"
// @Failure 500 {object} string "Failed to retrieve values"
// @Failure 400 {object} string "Bad request"
// @Router /api/events/{field} [get]
func GetDistinct(c *fiber.Ctx) error {
	eventsCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	field := c.Params("field")
	if field != "location" && field != "city" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid value for the field parameter",
			"error":   "The field parameter has to be 'location' or 'city'",
		})
	}

	d := time.Now()
	today := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())

	filter := bson.M{
		"$and": []bson.M{
			{
				"date": bson.M{
					"$gt": today,
				},
			},
		},
	}

	result, err := eventsCollection.Distinct(ctx, field, filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to query database.",
			"error":   err,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data":    result,
		"success": true,
	})
}

func GetMarkdownSummary(events []models.Event) string {
	result := ""
	for _, c := range events {
		result += fmt.Sprintf("<%s|%s> @%s, %s\n", c.URL, c.Title, c.Location, c.Date)
	}
	return result
}
