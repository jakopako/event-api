package controllers

import (
	"context"
	"fmt"
	"log/slog"
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
// @Param radius query int false "radius around given city or coordinates in kilometers"
// @Param lat query number false "latitude of the search location (used together with lon and radius)"
// @Param lon query number false "longitude of the search location (used together with lat and radius)"
// @Param date query string false "Deprecated: date search string in RFC3339 format. Cannot be combined with fromTime or toTime."
// @Param fromTime query string false "Inclusive start time in RFC3339 format (2006-01-02T15:04:05Z07:00)"
// @Param toTime query string false "Inclusive end time in RFC3339 format (2006-01-02T15:04:05Z07:00). Requires fromTime."
// @Param genres query string false "comma-separated list of genres; events matching at least one genre are returned"
// @Param page query int false "page number"
// @Param limit query int false "page size"
// @Success 200 {object} models.GetEventsResponseSuccess
// @Failure 400 {object} models.GenericResponse
// @Router /api/events [get]
func GetAllEvents(c *fiber.Ctx) error {
	radius, _ := strconv.Atoi(c.Query("radius", "0"))
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limitInt, _ := strconv.Atoi(c.Query("limit", "10"))
	var limit int64 = int64(limitInt)

	queryDate := c.Query("date")
	var startDate, endDate *time.Time
	excludeStartDate := false
	if queryDate != "" && (c.Query("fromTime") != "" || c.Query("toTime") != "") {
		return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to fetch events",
			Error:   "date cannot be combined with fromTime or toTime",
		})
	}
	if queryDate != "" {
		d, err := time.Parse(time.RFC3339, queryDate)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
				Success: false,
				Message: "failed fetch events",
				Error:   fmt.Sprintf("couldn't parse date: %v", err),
			})
		}
		startDate = &d
		plusOneDay := d.Add(time.Hour * 24)
		endDate = &plusOneDay
	} else if c.Query("fromTime") != "" || c.Query("toTime") != "" {
		var err error
		startDate, endDate, err = parseTimeRange(c.Query("fromTime"), c.Query("toTime"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
				Success: false,
				Message: "failed to fetch events",
				Error:   err.Error(),
			})
		}
	} else {
		now := time.Now().UTC()
		startDate = &now
		excludeStartDate = true
	}
	query := models.Query{
		Title:            c.Query("title"),
		City:             c.Query("city"),
		Country:          c.Query("country"),
		Location:         c.Query("location"),
		Type:             c.Query("type"),
		StartDate:        startDate,
		EndDate:          endDate,
		ExcludeStartDate: excludeStartDate,
		Radius:           radius,
		Page:             page,
		Limit:            limit,
	}
	if latStr := c.Query("lat"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			query.Lat = &lat
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
				Success: false,
				Message: "failed to fetch events",
				Error:   fmt.Sprintf("couldn't parse lat: %v", err),
			})
		}
	}
	if lonStr := c.Query("lon"); lonStr != "" {
		if lon, err := strconv.ParseFloat(lonStr, 64); err == nil {
			query.Lon = &lon
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
				Success: false,
				Message: "failed to fetch events",
				Error:   fmt.Sprintf("couldn't parse lon: %v", err),
			})
		}
	}
	if (query.Lat == nil) != (query.Lon == nil) {
		return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to fetch events",
			Error:   "lat and lon must both be provided or both be absent",
		})
	}
	if genresParam := c.Query("genres"); genresParam != "" {
		for _, g := range strings.Split(genresParam, ",") {
			if g = strings.TrimSpace(g); g != "" {
				query.Genres = append(query.Genres, g)
			}
		}
	}
	events, total, last, err := shared.FetchEvents(query)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
			Success: false,
			Message: "failed fetch events",
			Error:   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(models.GetEventsResponseSuccess{
		Data:     events,
		Total:    total,
		Page:     page,
		LastPage: last,
		Limit:    limit,
	})
}

// ValidateEvents func for validating events without inserting them into the database.
// @Description This endpoint validates events.
// @Summary Validate events.
// @Tags events
// @Accept json
// @Produce json
// @Param message body []models.Event true "event list"
// @Success 200 {object} models.ValidateAndAddEventsResponse
// @Failure 400 {object} models.ValidateAndAddEventsResponse
// @Router /api/events/validate [post]
func ValidateEvents(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	events := new([]models.Event)

	if err := c.BodyParser(events); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ValidateAndAddEventsResponse{
			Success: false,
			Message: "failed to parse body",
			Error:   err.Error(),
		})
	}

	validatedEvents, validationErrs := validateAndSanitizeEvents(ctx, events)

	if len(*validationErrs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ValidateAndAddEventsResponse{
			Success:          false,
			Message:          "some events have not been validated successfully",
			ValidationErrors: *validationErrs,
			ValidatedEvents:  *validatedEvents,
		})
	}
	return c.Status(fiber.StatusOK).JSON(models.ValidateAndAddEventsResponse{
		Success:         true,
		Message:         "events validated successfully",
		ValidatedEvents: *validatedEvents,
	})
}

// AddEvent func for adding new events to the database.
// @Description Add new events to the database.
// @Summary Add new events.
// @Tags events
// @Accept json
// @Produce json
// @Security BasicAuth
// @Param message body []models.Event true "event list"
// @Success 201 {object} models.ValidateAndAddEventsResponse
// @Failure 400 {object} models.ValidateAndAddEventsResponse
// @Failure 500 {object} models.ValidateAndAddEventsResponse
// @Router /api/events [post]
func AddEvents(c *fiber.Ctx) error {
	eventCollection := config.MI.DB.Collection("events")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	events := new([]models.Event)

	if err := c.BodyParser(events); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ValidateAndAddEventsResponse{
			Success: false,
			Message: "failed to parse body",
			Error:   err.Error(),
		})
	}

	validatedEvents, validationErrs := validateAndSanitizeEvents(ctx, events)

	slog.Debug("writing events to DB", "numEvents", len(*validatedEvents))
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

	if len(operations) > 0 {
		var err error
		bulkOption := options.BulkWriteOptions{}
		bulkOption.SetOrdered(true)
		_, err = eventCollection.BulkWrite(ctx, operations, &bulkOption)

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
				Success: false,
				Message: "failed to insert events",
				Error:   err.Error(),
			})
		}
	}

	if len(*validationErrs) > 0 {
		return c.Status(400).JSON(models.ValidateAndAddEventsResponse{
			Success:          false,
			Message:          "some events were not inserted successfully into the database",
			ValidationErrors: *validationErrs,
		})
	}
	return c.Status(fiber.StatusCreated).JSON(models.ValidateAndAddEventsResponse{
		Success: true,
		Message: "events inserted successfully",
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
	if err != nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"response_type": "ephemeral",
			"text":          "Sorry, something went wrong.",
		})
	}
	defer cursor.Close(ctx)

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
					"text": getMarkdownSummary(events),
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
// @Param datetime query string false "Deprecated: datetime string in YYYY-MM-DD HH:MM format. Cannot be combined with fromTime or toTime."
// @Param fromTime query string false "Inclusive start time in RFC3339 format (2006-01-02T15:04:05Z07:00)"
// @Param toTime query string false "Inclusive end time in RFC3339 format (2006-01-02T15:04:05Z07:00). Requires fromTime."
// @Success 200 {object} models.GenericResponse
// @Failure 400 {object} models.GenericResponse
// @Failure 500 {object} models.GenericResponse
// @Router /api/events [delete]
func DeleteEvents(c *fiber.Ctx) error {
	src := c.Query("sourceUrl")

	datetimeString := c.Query("datetime")
	if datetimeString != "" && (c.Query("fromTime") != "" || c.Query("toTime") != "") {
		return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
			Success: false,
			Message: "couldn't parse datetime",
			Error:   "datetime cannot be combined with fromTime or toTime",
		})
	}
	var filter bson.M
	if c.Query("fromTime") != "" || c.Query("toTime") != "" {
		fromTime, toTime, err := parseTimeRange(c.Query("fromTime"), c.Query("toTime"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
				Success: false,
				Message: "couldn't parse time range",
				Error:   err.Error(),
			})
		}
		dateFilter := bson.M{"$gte": fromTime}
		if toTime != nil {
			dateFilter["$lte"] = toTime
		}
		filter = bson.M{"date": dateFilter}
		if src != "" {
			filter["sourceUrl"] = src
		}
	} else if datetimeString == "" {
		filter = bson.M{"sourceUrl": src}
	} else {
		t, err := time.Parse("2006-01-02 15:04", datetimeString)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
				Success: false,
				Message: "couldn't parse datetime",
				Error:   err.Error(),
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

	eventsCollection := config.MI.DB.Collection("events")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := eventsCollection.DeleteMany(ctx, filter)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: fmt.Sprintf("failed to delete events from source %s", src),
			Error:   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(models.GenericResponse{
		Success: true,
		Message: fmt.Sprintf("successfully deleted %d events with source %s", result.DeletedCount, src),
	})
}

func parseTimeRange(fromTime, toTime string) (*time.Time, *time.Time, error) {
	if toTime != "" && fromTime == "" {
		return nil, nil, fmt.Errorf("fromTime must be provided when toTime is used")
	}

	var from, to *time.Time
	if fromTime != "" {
		parsed, err := time.Parse(time.RFC3339, fromTime)
		if err != nil {
			return nil, nil, fmt.Errorf("couldn't parse fromTime as RFC3339: %w", err)
		}
		from = &parsed
	}
	if toTime != "" {
		parsed, err := time.Parse(time.RFC3339, toTime)
		if err != nil {
			return nil, nil, fmt.Errorf("couldn't parse toTime as RFC3339: %w", err)
		}
		to = &parsed
	}
	return from, to, nil
}

// GetDistinct func for getting distinct field values.
// @Description This endpoint returns all distinct values for the given field. Note that past events are not considered for this query.
// @Summary Get distinct field values.
// @Tags events
// @Produce json
// @Param field path string true "field name, can only be location, city or genres"
// @Success 200 {object} models.GetDistinctFieldResponse
// @Failure 400 {object} models.GenericResponse
// @Failure 500 {object} models.GenericResponse
// @Router /api/events/{field} [get]
func GetDistinct(c *fiber.Ctx) error {
	eventsCollection := config.MI.DB.Collection("events")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	field := c.Params("field")
	if field != "location" && field != "city" && field != "genres" {
		return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
			Success: false,
			Message: "invalid value for the field parameter",
			Error:   "the field parameter has to be 'location', 'city' or 'genres'",
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
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to query database.",
			Error:   err.Error(),
		})
	}

	distinctValues := []string{}
	for _, r := range result {
		if str, ok := r.(string); ok {
			distinctValues = append(distinctValues, str)
		}
	}

	return c.Status(fiber.StatusOK).JSON(models.GetDistinctFieldResponse{
		Success: true,
		Data:    distinctValues,
	})
}

func getMarkdownSummary(events []models.Event) string {
	var result strings.Builder
	for _, c := range events {
		fmt.Fprintf(&result, "<%s|%s> @%s, %s\n", c.URL, c.Title, c.Location, c.Date)
	}
	return result.String()
}

// validateAndSanitizeEvents validates and sanitizes events
func validateAndSanitizeEvents(ctx context.Context, events *[]models.Event) (*[]models.Event, *[]models.ValidateEventError) {
	slog.Debug("validating events", "numEvents", len(*events))
	validate := validator.New()
	validationErrs := []models.ValidateEventError{}
	validatedEvents := []models.Event{}

	for _, event := range *events {
		err := validate.Struct(event)
		if err != nil {
			validationErrs = append(validationErrs, models.ValidateEventError{
				Message: fmt.Sprintf("failed to validate event %+v", event),
				Error:   err.Error(),
			})
			continue
		}

		// lower case type
		event.Type = strings.ToLower(event.Type)

		// Lookup the city coordinates
		// We need to lookup the city coordinates in order to make sure that the radius search works correctly
		cityGeoLoc, err := geo.LookupCityCoordinates(event.City, event.State, event.Country)
		if err != nil {
			validationErrs = append(validationErrs, models.ValidateEventError{
				Message: fmt.Sprintf("failed to find relevant coordinates for city {city: \"%s\", state: \"%s\", country: \"%s\"} (event %+v)", event.City, event.State, event.Country, event),
				Error:   err.Error(),
			})
			continue
		}

		// Lookup venue
		address, err := geo.LookupVenueLocation(event.Location, event.City, event.State, event.Country)
		if err == nil && address != nil {
			event.Address = *address
		} else {
			// If venue lookup fails, fall back to city coordinates
			event.Address.Geolocacation = *cityGeoLoc
		}

		// lookup genres if not given and if the event type is 'concert'
		if len(event.Genres) == 0 && event.Type == "concert" {
			genres, err := genre.LookupGenres(ctx, event)
			if err != nil {
				validationErrs = append(validationErrs, models.ValidateEventError{
					Message: fmt.Sprintf("failed to find genre for event %+v", event),
					Error:   err.Error(),
				})
			}
			event.Genres = genres
		}

		// add offset
		_, offset := event.Date.Zone()
		event.Offset = offset

		// add normalized title for diacritic-insensitive search
		event.NormalizedTitle = shared.RemoveDiacritics(event.Title)

		// append to validated events
		validatedEvents = append(validatedEvents, event)
	}

	return &validatedEvents, &validationErrs
}
