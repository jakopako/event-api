package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/genre"
	"github.com/jakopako/event-api/geo"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestGetAllEventsBadRequests(t *testing.T) {
	app := fiber.New()
	app.Get("/", GetAllEvents)
	tests := []struct {
		name, query, message, errorText string
	}{
		{"missing from", "?toTime=2026-01-01T00:00:00Z", "failed to fetch events", "fromTime must be provided"},
		{"bad from", "?fromTime=bad", "failed to fetch events", "couldn't parse fromTime"},
		{"bad to", "?fromTime=2026-01-01T00:00:00Z&toTime=bad", "failed to fetch events", "couldn't parse toTime"},
		{"bad lat", "?lat=bad&lon=2", "failed to fetch events", "couldn't parse lat"},
		{"bad lon", "?lat=1&lon=bad", "failed to fetch events", "couldn't parse lon"},
		{"latitude alone", "?lat=1", "failed to fetch events", "lat and lon must both"},
		{"longitude alone", "?lon=2", "failed to fetch events", "lat and lon must both"},
		{"invalid page", "?page=bad", "failed fetch events", "page parameter must be greater than 0"},
		{"zero page", "?page=0", "failed fetch events", "page parameter must be greater than 0"},
		{"invalid limit", "?limit=bad", "failed fetch events", "limit parameter must be greater than 0"},
		{"negative limit", "?limit=-1", "failed fetch events", "limit parameter must be greater than 0"},
		{"negative radius", "?radius=-1", "failed fetch events", "radius parameter must be greater than or equal to 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := handlerRequest(t, app, http.MethodGet, "/"+tt.query, "", "")
			got := decodeResponse[models.GenericResponse](t, resp, http.StatusBadRequest)
			if got.Success || got.Message != tt.message || !strings.Contains(got.Error, tt.errorText) {
				t.Fatalf("response = %+v", got)
			}
		})
	}
}

func TestValidateEventsHandler(t *testing.T) {
	app := fiber.New()
	app.Post("/", ValidateEvents)
	tests := []struct {
		name, body, contentType, message string
		status, errors                   int
		success                          bool
	}{
		{"malformed JSON", "[", "application/json", "failed to parse body", 400, 0, false},
		{"wrong JSON type", "{}", "application/json", "failed to parse body", 400, 0, false},
		{"unsupported content type", "[]", "text/plain", "failed to parse body", 400, 0, false},
		{"empty list", "[]", "application/json", "events validated successfully", 200, 0, true},
		{"invalid events", "[{},{}]", "application/json", "some events have not been validated successfully", 400, 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := handlerRequest(t, app, http.MethodPost, "/", tt.contentType, tt.body)
			got := decodeResponse[models.ValidateAndAddEventsResponse](t, resp, tt.status)
			if got.Success != tt.success || got.Message != tt.message || len(got.ValidationErrors) != tt.errors || len(got.ValidatedEvents) != 0 {
				t.Fatalf("response = %+v", got)
			}
			if tt.message == "failed to parse body" && got.Error == "" {
				t.Fatal("expected parse error details")
			}
		})
	}
}

func TestDeleteEventsRejectsInvalidTimeRanges(t *testing.T) {
	app := fiber.New()
	app.Delete("/", DeleteEvents)
	for _, query := range []string{
		"?toTime=2026-01-01T00:00:00Z",
		"?fromTime=bad",
		"?fromTime=2026-01-01T00:00:00Z&toTime=bad",
	} {
		t.Run(query, func(t *testing.T) {
			got := decodeResponse[models.GenericResponse](t, handlerRequest(t, app, http.MethodDelete, "/"+query, "", ""), 400)
			if got.Success || got.Message != "couldn't parse time range" || got.Error == "" {
				t.Fatalf("response = %+v", got)
			}
		})
	}
}

func TestEventsHandlersWithMockMongo(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("list events and query mapping", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(
			mockCursor(mt, "events", bson.D{{Key: "n", Value: 3}}),
			mockCursor(mt, "events", bson.D{{Key: "title", Value: "Concert"}, {Key: "city", Value: "Berlin"}}),
		)
		app := fiber.New()
		app.Get("/", GetAllEvents)
		resp := handlerRequest(mt.T, app, http.MethodGet, "/?page=2&limit=1&genres=jazz,%20rock,,&lat=52&lon=13", "", "")
		got := decodeResponse[models.GetEventsResponseSuccess](mt.T, resp, 200)
		if got.Total != 3 || got.Page != 2 || got.LastPage != 3 || got.Limit != 1 || len(got.Data) != 1 || got.Data[0].Title != "Concert" {
			mt.Fatalf("response = %+v", got)
		}
		count := mt.GetStartedEvent()
		find := mt.GetStartedEvent()
		if count == nil || count.CommandName != "aggregate" || find == nil || find.CommandName != "find" {
			mt.Fatalf("unexpected commands: count=%v find=%v", count, find)
		}
		if skip, ok := find.Command.Lookup("skip").Int64OK(); !ok || skip != 1 {
			mt.Fatalf("find skip = %v, want 1", find.Command.Lookup("skip"))
		}
		if limit, ok := find.Command.Lookup("limit").Int64OK(); !ok || limit != 1 {
			mt.Fatalf("find limit = %v, want 1", find.Command.Lookup("limit"))
		}
		filter := find.Command.Lookup("filter").Document()
		var decoded bson.M
		if err := bson.Unmarshal(filter, &decoded); err != nil {
			mt.Fatal(err)
		}
		conditions, ok := decoded["$and"].(bson.A)
		if !ok || len(conditions) != 2 {
			mt.Fatalf("unexpected filter: %#v", decoded)
		}
		genreCondition := conditions[0].(bson.M)["genres"].(bson.M)["$in"]
		if genres, ok := genreCondition.(bson.A); !ok || len(genres) != 2 || genres[0] != "jazz" || genres[1] != "rock" {
			mt.Fatalf("genres = %#v", genreCondition)
		}
		coords := conditions[1].(bson.M)["address.geolocation.coordinates"]
		if coordinates, ok := coords.(bson.A); !ok || len(coordinates) != 2 || coordinates[0] != 13.0 || coordinates[1] != 52.0 {
			mt.Fatalf("coordinates = %#v", coords)
		}
	})
	mt.Run("list database failure", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mockCursor(mt, "events"), mockDBError())
		app := fiber.New()
		app.Get("/", GetAllEvents)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/", "", ""), 400)
		if got.Success || !strings.Contains(got.Error, "database failure") {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("validate fails when city lookup fails", func(mt *mtest.T) {
		useMockDatabase(mt)
		oldGeo := geo.GC
		mt.Cleanup(func() { geo.GC = oldGeo })
		geo.InitGeolocCache()
		mt.AddMockResponses(mockDBError())
		app := fiber.New()
		app.Post("/", ValidateEvents)
		body := `[{"title":"Show","location":"Hall","city":"Berlin","date":"2026-09-03T12:00:00Z","url":"https://example.com/event","sourceUrl":"https://example.com","type":"concert"}]`
		got := decodeResponse[models.ValidateAndAddEventsResponse](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/json", body), 400)
		if got.Success || len(got.ValidatedEvents) != 0 || len(got.ValidationErrors) != 1 || !strings.Contains(got.ValidationErrors[0].Error, "database failure") {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("add invalid events", func(mt *mtest.T) {
		useMockDatabase(mt)
		app := fiber.New()
		app.Post("/", AddEvents)
		got := decodeResponse[models.ValidateAndAddEventsResponse](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/json", "[{}]"), 400)
		if got.Success || len(got.ValidationErrors) != 1 || mt.GetStartedEvent() != nil {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("add empty list", func(mt *mtest.T) {
		useMockDatabase(mt)
		app := fiber.New()
		app.Post("/", AddEvents)
		got := decodeResponse[models.ValidateAndAddEventsResponse](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/json", "[]"), 201)
		if !got.Success || mt.GetStartedEvent() != nil {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("add malformed body", func(mt *mtest.T) {
		useMockDatabase(mt)
		app := fiber.New()
		app.Post("/", AddEvents)
		got := decodeResponse[models.ValidateAndAddEventsResponse](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/json", "["), 400)
		if got.Success || got.Message != "failed to parse body" || got.Error == "" || mt.GetStartedEvent() != nil {
			mt.Fatalf("response = %+v", got)
		}
	})
	for _, tt := range []struct {
		name, path string
		handler    fiber.Handler
		status     int
		mixed      bool
	}{
		{"validate enriched event", "/validate", ValidateEvents, 200, false},
		{"add enriched event", "/", AddEvents, 201, false},
		{"add mixed batch", "/", AddEvents, 400, true},
	} {
		mt.Run(tt.name, func(mt *mtest.T) {
			useMockDatabase(mt)
			oldGeo, oldGenre := geo.GC, genre.GC
			mt.Cleanup(func() { geo.GC, genre.GC = oldGeo, oldGenre })
			mt.Setenv("LOOKUP_SPOTIFY_GENRE", "false")
			geo.InitGeolocCache()
			genre.InitGenreCache()
			city := bson.D{{Key: "geolocation", Value: bson.D{
				{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{13.4, 52.5}},
			}}}
			venue := bson.D{{Key: "name", Value: "Hall"}, {Key: "address", Value: bson.D{
				{Key: "locality", Value: "Berlin"},
				{Key: "geolocation", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{13.41, 52.51}}}},
			}}}
			mt.AddMockResponses(mockCursor(mt, "cities", city), mockCursor(mt, "venues", venue))
			if tt.path == "/" {
				mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
			}
			body := `{"title":"Café","location":"Hall","city":"Berlin","date":"2026-09-03T12:00:00+02:00","url":"https://example.com/event","sourceUrl":"https://example.com","type":"CONCERT","genres":["jazz"]}`
			if tt.mixed {
				body += `,{}`
			}
			app := fiber.New()
			app.Post(tt.path, tt.handler)
			got := decodeResponse[models.ValidateAndAddEventsResponse](mt.T, handlerRequest(mt.T, app, http.MethodPost, tt.path, "application/json", "["+body+"]"), tt.status)
			if tt.mixed {
				if got.Success || len(got.ValidationErrors) != 1 {
					mt.Fatalf("response = %+v", got)
				}
			} else if !got.Success {
				mt.Fatalf("response = %+v", got)
			}
			if tt.path == "/validate" {
				if len(got.ValidatedEvents) != 1 || got.ValidatedEvents[0].Type != "concert" || got.ValidatedEvents[0].Offset != 7200 || got.ValidatedEvents[0].Address.Locality != "Berlin" {
					mt.Fatalf("validated events = %+v", got.ValidatedEvents)
				}
			}
			for _, name := range []string{"find", "find"} {
				if event := mt.GetStartedEvent(); event == nil || event.CommandName != name {
					mt.Fatalf("expected %s command, got %v", name, event)
				}
			}
			if tt.path == "/" {
				event := mt.GetStartedEvent()
				if event == nil || event.CommandName != "update" {
					mt.Fatalf("expected bulk update, got %v", event)
				}
				updates, ok := event.Command.Lookup("updates").Array().Values()
				if ok != nil || len(updates) != 1 {
					mt.Fatalf("unexpected updates: %v, %v", updates, ok)
				}
				update := updates[0].Document()
				replacement := update.Lookup("u").Document()
				if replacement.Lookup("title").StringValue() != "Café" || replacement.Lookup("normalizedTitle").StringValue() != "Cafe" || replacement.Lookup("type").StringValue() != "concert" {
					mt.Fatalf("unexpected replacement: %v", replacement)
				}
				if !update.Lookup("upsert").Boolean() {
					mt.Fatalf("expected upsert: %v", update)
				}
			}
		})
	}
	mt.Run("delete succeeds", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 2}))
		app := fiber.New()
		app.Delete("/", DeleteEvents)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodDelete, "/?sourceUrl=https%3A%2F%2Fexample.com", "", ""), 200)
		if !got.Success || !strings.Contains(got.Message, "deleted 2 events") {
			mt.Fatalf("response = %+v", got)
		}
		filter := deleteFilter(mt, 0)
		if filter.Lookup("sourceUrl").StringValue() != "https://example.com" {
			mt.Fatalf("unexpected event deletion filter: %v", filter)
		}
	})
	mt.Run("distinct succeeds", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "values", Value: bson.A{"Berlin", "Paris", 42}}))
		app := fiber.New()
		app.Get("/:field", GetDistinct)
		got := decodeResponse[models.GetDistinctFieldResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/city", "", ""), 200)
		if !got.Success || len(got.Data) != 2 || got.Data[0] != "Berlin" || got.Data[1] != "Paris" {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("distinct database failure", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mockDBError())
		app := fiber.New()
		app.Get("/:field", GetDistinct)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/city", "", ""), 500)
		if got.Success || got.Message != "failed to query database." || !strings.Contains(got.Error, "database failure") {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("delete database failure", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mockDBError())
		app := fiber.New()
		app.Delete("/", DeleteEvents)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodDelete, "/?sourceUrl=example", "", ""), 500)
		if got.Success || !strings.Contains(got.Error, "database failure") {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("slack validation", func(mt *mtest.T) {
		useMockDatabase(mt)
		app := fiber.New()
		app.Post("/", GetTodaysEventsSlack)
		for _, tt := range []struct {
			contentType, body, text string
		}{
			{"application/json", "{", "Failed to parse request body."},
			{"application/x-www-form-urlencoded", "text=++", "Please provide a city."},
		} {
			got := decodeResponse[map[string]any](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", tt.contentType, tt.body), 400)
			if got["response_type"] != "ephemeral" || got["text"] != tt.text {
				mt.Fatalf("response = %+v", got)
			}
		}
		if event := mt.GetStartedEvent(); event != nil {
			mt.Fatalf("unexpected database command: %v", event)
		}
	})
	mt.Run("slack no events", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mockCursor(mt, "events"))
		app := fiber.New()
		app.Post("/", GetTodaysEventsSlack)
		got := decodeResponse[map[string]any](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/x-www-form-urlencoded", "text=+Berlin+"), 200)
		if got["response_type"] != "ephemeral" || got["text"] != "Sorry, no events tonight for Berlin." {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("slack matching events", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(
			mockCursor(mt, "events", bson.D{{Key: "n", Value: 1}}),
			mockCursor(mt, "events", bson.D{{Key: "title", Value: "Concert"}, {Key: "url", Value: "https://example.com"}, {Key: "location", Value: "Hall"}}),
		)
		app := fiber.New()
		app.Post("/", GetTodaysEventsSlack)
		got := decodeResponse[struct {
			ResponseType string `json:"response_type"`
			Blocks       []struct {
				Type string `json:"type"`
				Text struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"text"`
			} `json:"blocks"`
		}](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/x-www-form-urlencoded", "text=Berlin"), 200)
		if got.ResponseType != "ephemeral" || len(got.Blocks) != 1 || got.Blocks[0].Type != "section" || got.Blocks[0].Text.Type != "mrkdwn" || !strings.Contains(got.Blocks[0].Text.Text, "<https://example.com|Concert> @Hall") {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("slack find error", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mockCursor(mt, "events", bson.D{{Key: "n", Value: 1}}), mockDBError())
		app := fiber.New()
		app.Post("/", GetTodaysEventsSlack)
		got := decodeResponse[map[string]any](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/x-www-form-urlencoded", "text=Berlin"), 200)
		if got["response_type"] != "ephemeral" || got["text"] != "Sorry, something went wrong." {
			mt.Fatalf("response = %+v", got)
		}
	})
}
