package controllers

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/croncert-api/config"
	"github.com/jakopako/croncert-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/go-playground/validator.v9"
)

// GetAllEvents func gets all events.
// @Description Get all events.
// @Summary Get all events.
// @Tags events
// @Accept json
// @Produce json
// @Param title query string false "title search string"
// @Param location query string false "location search string"
// @Param city query string false "city search string"
// @Param page query int false "page number"
// @Param limit query int false "page size"
// @Success 200 {array} models.Event
// @Router /api/events [get]
func GetAllEvents(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var events []models.Event
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

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{"date", 1}})

	for _, searchString := range []string{"title", "location", "city"} {
		if s := c.Query(searchString); s != "" {
			filter["$and"] = append(filter["$and"].([]bson.M), bson.M{
				"$or": []bson.M{
					{
						searchString: bson.M{
							"$regex": primitive.Regex{
								Pattern: s,
								Options: "i",
							},
						},
					},
				},
			})
		}
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limitVal, _ := strconv.Atoi(c.Query("limit", "10"))
	var limit int64 = int64(limitVal)

	total, _ := eventCollection.CountDocuments(ctx, filter)

	findOptions.SetSkip((int64(page) - 1) * limit)
	findOptions.SetLimit(limit)

	cursor, err := eventCollection.Find(ctx, filter, findOptions)
	defer cursor.Close(ctx)

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "events not found",
			"error":   err,
		})
	}

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

// AddEvent func for adding a new event to the database.
// @Description Add a new event.
// @Summary Add a new event.
// @Tags events
// @Accept json
// @Produce json
// @Param message body models.Event true "Event Info"
// @Failure 400 {object} string "Failed to parse body"
// @Failure 500 {object} string "Failed to insert event"
// @Router /api/events [post]
func AddEvent(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	event := new(models.Event)

	if err := c.BodyParser(event); err != nil {
		//log.Println(err)
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Failed to parse body",
			"error":   err,
		})
	}

	validate := validator.New()
	err := validate.Struct(event)

	if err != nil {
		//log.Println(err)
		return c.Status(400).JSON(fiber.Map{
			"succes":  false,
			"message": "Failed to parse body",
			"error":   fmt.Sprint(err),
		})
	}

	opts := options.Replace().SetUpsert(true)
	// The filter ignores the comment assuming that the comment might be updated over time.
	// In future versions we might need to take more factors into account to decide whether
	// an existing event needs to be updated or a new event needs to be added.
	filterEvent := models.Event{
		Title:    event.Title,
		Date:     event.Date,
		Location: event.Location,
		URL:      event.URL,
	}
	result, err := eventCollection.ReplaceOne(ctx, filterEvent, event, opts)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to insert event",
			"error":   err,
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data":    result,
		"success": true,
		"message": "Event inserted successfully",
	})

}

// GetTodayseventsSlack func for retrieving today's events, formatted as md for slack.
// @Description Get today's events.
// @Summary Get today's events.
// @Tags events
// @Accept json
// @Produce json
// @Success 200 {string} string "A json with the results"
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

func GetMarkdownSummary(events []models.Event) string {
	result := ""
	for _, c := range events {
		result += fmt.Sprintf("<%s|%s> @%s, %s\n", c.URL, c.Title, c.Location, c.Date)
	}
	return result
}
