package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestGetScraperStatusBadPagination(t *testing.T) {
	app := fiber.New()
	app.Get("/", GetScraperStatus)
	for _, tt := range []struct {
		query, errorText string
	}{
		{"?page=0", "page parameter must be greater than 0"},
		{"?page=bad", "page parameter must be greater than 0"},
		{"?limit=-1", "limit parameter must be greater than 0"},
		{"?limit=bad", "limit parameter must be greater than 0"},
	} {
		t.Run(tt.query, func(t *testing.T) {
			got := decodeResponse[models.GenericResponse](t, handlerRequest(t, app, http.MethodGet, "/"+tt.query, "", ""), 400)
			if got.Success || got.Message != "failed to fetch statuses" || got.Error != tt.errorText {
				t.Fatalf("response = %+v", got)
			}
		})
	}
}

func TestStatusHandlersWithMockMongo(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("upsert input validation", func(mt *mtest.T) {
		useMockDatabase(mt)
		app := fiber.New()
		app.Post("/", UpsertScraperStatus)
		tests := []struct {
			name, body, message, errorText string
		}{
			{"malformed", "{", "failed to parse status", ""},
			{"missing fields", "{}", "failed to validate status", "ScraperName"},
			{"negative items", `{"scraperName":"Scraper","nrItems":-1,"nrErrors":0,"lastScrapeStart":"2026-01-01T10:00:00Z","lastScrapeEnd":"2026-01-01T11:00:00Z"}`, "failed to validate status", "NrItems"},
			{"negative errors", `{"scraperName":"Scraper","nrItems":0,"nrErrors":-1,"lastScrapeStart":"2026-01-01T10:00:00Z","lastScrapeEnd":"2026-01-01T11:00:00Z"}`, "failed to validate status", "NrErrors"},
			{"reversed times", `{"scraperName":"Scraper","nrItems":0,"nrErrors":0,"lastScrapeStart":"2026-01-01T11:00:00Z","lastScrapeEnd":"2026-01-01T10:00:00Z"}`, "failed to upsert status", "lastScrapeEnd must be after lastScrapeStart"},
		}
		for _, tt := range tests {
			mt.Run(tt.name, func(sub *mtest.T) {
				got := decodeResponse[models.GenericResponse](sub.T, handlerRequest(sub.T, app, http.MethodPost, "/", "application/json", tt.body), 400)
				if got.Success || got.Message != tt.message || got.Error == "" || !strings.Contains(got.Error, tt.errorText) {
					sub.Fatalf("response = %+v", got)
				}
				if event := mt.GetStartedEvent(); event != nil {
					sub.Fatalf("unexpected database command: %v", event)
				}
			})
		}
	})
	mt.Run("upsert success with zero counters and equal times", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		app := fiber.New()
		app.Post("/", UpsertScraperStatus)
		body := `{"scraperName":"Scraper","nrItems":0,"nrErrors":0,"lastScrapeStart":"2026-01-01T10:00:00Z","lastScrapeEnd":"2026-01-01T10:00:00Z"}`
		got := decodeResponse[models.UpsertScraperStatusResponse](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/json", body), 200)
		if !got.Success || got.Data.ScraperName != "Scraper" || got.Data.NrItems == nil || *got.Data.NrItems != 0 || got.Data.NrErrors == nil || *got.Data.NrErrors != 0 {
			mt.Fatalf("response = %+v", got)
		}
		if event := mt.GetStartedEvent(); event == nil || event.CommandName != "update" {
			mt.Fatalf("expected update command, got %v", event)
		}
	})
	mt.Run("upsert database failure", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mockDBError())
		app := fiber.New()
		app.Post("/", UpsertScraperStatus)
		body := `{"scraperName":"Scraper","nrItems":0,"nrErrors":0,"lastScrapeStart":"2026-01-01T10:00:00Z","lastScrapeEnd":"2026-01-01T11:00:00Z"}`
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodPost, "/", "application/json", body), 500)
		if got.Success || got.Message != "failed to upsert status" || !strings.Contains(got.Error, "database failure") {
			mt.Fatalf("response = %+v", got)
		}
	})
	for _, includeLogs := range []bool{false, true} {
		name := "logs excluded"
		target := "/?name=Scraper&page=2&limit=1"
		if includeLogs {
			name = "logs included"
			target += "&returnScraperLogs=true"
		}
		mt.Run("list database failure", func(mt *mtest.T) {
			useMockDatabase(mt)
			mt.AddMockResponses(mockCursor(mt, "status"), mockDBError())
			app := fiber.New()
			app.Get("/", GetScraperStatus)
			got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/", "", ""), 404)
			if got.Success || got.Message != "failed to fetch statuses" || !strings.Contains(got.Error, "database failure") {
				mt.Fatalf("response = %+v", got)
			}
		})
		mt.Run(name, func(mt *mtest.T) {
			useMockDatabase(mt)
			mt.AddMockResponses(mockCursor(mt, "status", bson.D{{Key: "n", Value: 3}}), mockCursor(mt, "status", bson.D{{Key: "scraperName", Value: "Scraper"}}))
			app := fiber.New()
			app.Get("/", GetScraperStatus)
			got := decodeResponse[models.GetScraperStatusResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, target, "", ""), 200)
			if got.Total != 3 || got.LastPage != 3 || got.Page != 2 || got.Limit != 1 || len(got.Data) != 1 || got.Data[0].ScraperName != "Scraper" {
				mt.Fatalf("response = %+v", got)
			}
			mt.GetStartedEvent()
			find := mt.GetStartedEvent()
			if find == nil || find.CommandName != "find" {
				mt.Fatalf("unexpected find: %v", find)
			}
			if find.Command.Lookup("filter").Document().Lookup("scraperName").StringValue() != "Scraper" {
				mt.Fatalf("unexpected filter: %v", find.Command)
			}
			projection := find.Command.Lookup("projection")
			if includeLogs && projection.Type != 0 || !includeLogs && projection.Type == 0 {
				mt.Fatalf("unexpected projection: %v", projection)
			}
			if !includeLogs && projection.Document().Lookup("scraperLogs").Int32() != 0 {
				mt.Fatalf("unexpected projection: %v", projection)
			}
		})
	}
	for _, tt := range []struct {
		name     string
		response bson.D
		status   int
		message  string
	}{
		{"deleted", mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}), 200, "status deleted successfully"},
		{"not found", mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}), 404, "status not found"},
		{"database error", mockDBError(), 500, "failed to delete status"},
	} {
		mt.Run(tt.name, func(mt *mtest.T) {
			useMockDatabase(mt)
			mt.AddMockResponses(tt.response)
			app := fiber.New()
			app.Delete("/:name", DeleteScraperStatus)
			got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodDelete, "/My%20Scraper", "", ""), tt.status)
			if got.Success != (tt.status == 200) || got.Message != tt.message {
				mt.Fatalf("response = %+v", got)
			}
			filter := deleteFilter(mt, 1)
			if filter.Lookup("scraperName").StringValue() != "My Scraper" {
				mt.Fatalf("unexpected scraper filter: %v", filter)
			}
		})
	}
}
