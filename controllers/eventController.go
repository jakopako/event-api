package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/genre"
	"github.com/jakopako/event-api/geo"
	"github.com/jakopako/event-api/models"
	"github.com/jakopako/event-api/shared"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/go-playground/validator.v9"
)

// GetAllEvents func gets all events.
// @Description This endpoint returns all events matching the search terms. Note that only events from today on will be returned if no date is passed, ie no past events.
// @Summary Get all events.
// @Tags events
// @Accept json
// @Produce json
// @Param title query string false "title search string"
// @Param location query string false "location search string"
// @Param type query string false "type search string"
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
	radius, _ := strconv.Atoi(c.Query("radius", "0"))
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limitInt, _ := strconv.Atoi(c.Query("limit", "10"))
	var limit int64 = int64(limitInt)

	// TODO: push defining start date and end date to the caller of this endpoint
	queryDate := c.Query("date")
	var startDate, endDate *time.Time
	if queryDate == "" {
		now := time.Now().UTC()
		startDate = &now
	} else {
		d, err := time.Parse(time.RFC3339, queryDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"success": false,
				"message": "failed fetch events",
				"error":   fmt.Sprintf("couldn't parse date: %v", err),
			})
		}
		startDate = &d
		plusOneDay := d.Add(time.Hour * 24)
		endDate = &plusOneDay
	}
	query := models.Query{
		Title:     c.Query("title"),
		City:      c.Query("city"),
		Country:   c.Query("country"),
		Location:  c.Query("location"),
		Type:      c.Query("type"),
		StartDate: startDate,
		EndDate:   endDate,
		Radius:    radius,
		Page:      page,
		Limit:     limit,
	}
	events, total, last, err := shared.FetchEvents(query)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "failed fetch events",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data":      events,
		"total":     total,
		"page":      page,
		"last_page": last,
		"limit":     limit,
	})
}

// ValidateEvents func for validating events without inserting them into the database.
// @Description This endpoint validates events.
// @Summary Validate events.
// @Tags events
// @Accept json
// @Produce json
// @Param message body []models.Event true "Event Info"
// @Success 200 {object} string "A json with the results"
// @Failure 400 {object} string "failed to validate events"
// @Router /api/events/validate [post]
func ValidateEvents(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	events := new([]models.Event)

	if err := c.BodyParser(events); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "failed to parse body",
			"error":   err.Error(),
		})
	}

	validatedEvents, validationErrs := validateAndSanitizeEvents(ctx, events)

	if len(validationErrs) > 0 {
		return c.Status(400).JSON(fiber.Map{
			"succes":  false,
			"message": "some events have not been validated successfully",
			"errors":  validationErrs,
			"data":    validatedEvents,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success":         true,
		"message":         "events validated successfully",
		"validatedEvents": validatedEvents,
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
// @Success 201 {object} string "A json with the results"
// @Failure 400 {object} string "failed to parse body"
// @Failure 500 {object} string "failed to insert events"
// @Router /api/events [post]
func AddEvents(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	events := new([]models.Event)

	if err := c.BodyParser(events); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "failed to parse body",
			"error":   err.Error(),
		})
	}

	validatedEvents, validationErrs := validateAndSanitizeEvents(ctx, events)

	var operations []mongo.WriteModel
	for _, event := range *validatedEvents {
		op := mongo.NewReplaceOneModel()
		// The filter ignores the comment assuming that the comment might be updated over time.
		// In future versions we might need to take more factors into account to decide whether
		// an existing event needs to be updated or a new event needs to be added.
		filterEvent := bson.D{
			{Key: "title", Value: event.Title},
			{Key: "date", Value: event.Date},
			{Key: "location", Value: event.Location},
			{Key: "url", Value: event.URL},
			{Key: "sourceUrl", Value: event.SourceURL},
		}
		op.SetFilter(filterEvent)
		op.SetUpsert(true)
		op.SetReplacement(event)
		operations = append(operations, op)
	}

	var result *mongo.BulkWriteResult
	if len(operations) > 0 {
		var err error
		bulkOption := options.BulkWriteOptions{}
		bulkOption.SetOrdered(true)
		result, err = eventCollection.BulkWrite(ctx, operations, &bulkOption)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "failed to insert events",
				"error":   err.Error(),
			})
		}
	}

	if len(validationErrs) > 0 {
		return c.Status(400).JSON(fiber.Map{
			"succes":  false,
			"data":    result,
			"message": "some events were not inserted successfully into the database",
			"errors":  validationErrs,
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data":    result,
		"success": true,
		"message": "events inserted successfully",
	})

}

// GetTodayseventsSlack func for retrieving today's events, formatted as md for slack.
// @Description This endpoint returns today's events for a given city in a format that slack needs for its slash command.
// @Summary Get today's events.
// @Tags events
// @Accept x-www-form-urlencoded
// @Produce json
// @Param slackRequest formData models.SlackRequest true "Slack Request Info"
// @Success 200 {object} string "A json with the results"
// @Router /api/events/today/slack [post]
func GetTodaysEventsSlack(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var events []models.Event
	now := time.Now()
	plus24h := now.Add(24 * time.Hour)
	s := new(models.SlackRequest)
	if err := c.BodyParser(s); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"response_type": "ephemeral",
			"text":          "Failed to parse request body.",
		})
	}

	city := strings.TrimSpace(s.Text)
	if city == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"response_type": "ephemeral",
			"text":          "Please provide a city.",
		})
	}

	filter := bson.M{
		"$and": []bson.M{
			{
				"date": bson.M{
					"$gte": now,
				},
			},
			{
				"date": bson.M{
					"$lte": plus24h,
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
	findOptions.SetSort(bson.D{{Key: "date", Value: 1}})

	total, _ := eventCollection.CountDocuments(ctx, filter)
	if total == 0 {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"response_type": "ephemeral",
			"text":          fmt.Sprintf("Sorry, no events tonight for %s.", city),
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
// @Failure 500 {object} string "failed to delete events"
// @Router /api/events [delete]
func DeleteEvents(c *fiber.Ctx) error {
	eventsCollection := config.MI.DB.Collection("events")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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
				"message": "couldn't parse datetime",
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
			"message": fmt.Sprintf("failed to delete events from source %s", src),
			"error":   err,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("successfully deleted %d events with source %s", result.DeletedCount, src),
	})
}

// GetDistinct func for getting distinct field values.
// @Description This endpoint returns all distinct values for the given field. Note that past events are not considered for this query.
// @Summary Get distinct field values.
// @Tags events
// @Produce json
// @Param field path string true "field name, can only be location or city"
// @Failure 500 {object} string "failed to retrieve values"
// @Failure 400 {object} string "Bad request"
// @Router /api/events/{field} [get]
func GetDistinct(c *fiber.Ctx) error {
	eventsCollection := config.MI.DB.Collection("events")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	field := c.Params("field")
	if field != "location" && field != "city" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "invalid value for the field parameter",
			"error":   "the field parameter has to be 'location' or 'city'",
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
			"message": "failed to query database.",
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

// validateAndSanitizeEvents validates and sanitizes events
func validateAndSanitizeEvents(ctx context.Context, events *[]models.Event) (*[]models.Event, []fiber.Map) {
	validate := validator.New()
	validationErrs := []fiber.Map{}
	validatedEvents := []models.Event{}

	for _, event := range *events {
		err := validate.Struct(event)
		if err != nil {
			validationErrs = append(validationErrs, fiber.Map{
				"message": fmt.Sprintf("failed to validate event %+v", event),
				"error":   err.Error(),
			})
			continue
		}

		// lower case type
		event.Type = strings.ToLower(event.Type)

		// First, try to lookup venue
		venue, err := geo.LookupVenueLocation(event.Location, event.City, event.Country)
		if err == nil && venue != nil {
			event.Address = venue.Address
		} else {
			// If venue lookup fails, fall back to city coordinates
			geoLoc, err := geo.LookupCityCoordinates(event.City, event.Country)
			if err != nil {
				validationErrs = append(validationErrs, fiber.Map{
					"message": fmt.Sprintf("failed to find relevant coordinates for venue '%s' or city {city: \"%s\", country: \"%s\"} (event %+v)", event.Location, event.City, event.Country, event),
					"error":   err.Error(),
				})
				continue
			}
			event.Address.Geolocacation = *geoLoc
		}

		// lookup genres if not given and if the event type is 'concert'
		if len(event.Genres) == 0 && event.Type == "concert" {
			genres, err := genre.LookupGenres(ctx, event)
			if err != nil {
				validationErrs = append(validationErrs, fiber.Map{
					"message": fmt.Sprintf("failed to find genre for event %+v", event),
					"error":   err.Error(),
				})
			}
			event.Genres = genres
		}

		// add offset
		_, offset := event.Date.Zone()
		event.Offset = offset

		// append to validated events
		validatedEvents = append(validatedEvents, event)
	}

	return &validatedEvents, validationErrs
}
