package controllers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/models"
	"gopkg.in/go-playground/validator.v9"
)

func TestParseTimeRange(t *testing.T) {
	from := "2026-09-03T12:00:00Z"
	to := "2026-09-03T14:00:00+02:00"

	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
		wantNil bool
	}{
		{name: "no range", wantNil: true},
		{name: "from only", from: from},
		{name: "from and to", from: from, to: to},
		{name: "to requires from", to: to, wantErr: true},
		{name: "invalid from", from: "not-a-time", wantErr: true},
		{name: "invalid to", from: from, to: "not-a-time", wantErr: true},
		{name: "fractional seconds", from: "2026-09-03T12:00:00.123Z"},
		{name: "missing timezone", from: "2026-09-03T12:00:00", wantErr: true},
		{name: "reversed range is parsed", from: from, to: "2026-09-03T11:00:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFrom, gotTo, err := parseTimeRange(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTimeRange() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantNil {
				if gotFrom != nil || gotTo != nil {
					t.Errorf("parseTimeRange() = (%v, %v), want (nil, nil)", gotFrom, gotTo)
				}
				return
			}
			if gotFrom == nil {
				t.Fatal("parseTimeRange() returned nil from time")
			}
			wantFrom, _ := time.Parse(time.RFC3339, tt.from)
			if !gotFrom.Equal(wantFrom) {
				t.Errorf("from = %v, want %v", gotFrom, wantFrom)
			}
			if tt.to == "" && gotTo != nil {
				t.Errorf("to = %v, want nil", gotTo)
			}
			if tt.to != "" {
				wantTo, _ := time.Parse(time.RFC3339, tt.to)
				if gotTo == nil || !gotTo.Equal(wantTo) {
					t.Errorf("to = %v, want %v", gotTo, wantTo)
				}
			}
		})
	}
}

func TestIsSupportedDistinctField(t *testing.T) {
	tests := []struct {
		field string
		want  bool
	}{
		{field: "location", want: true},
		{field: "city", want: true},
		{field: "genres", want: true},
		{field: "type", want: true},
		{field: "country", want: false},
		{field: "", want: false},
		{field: "City", want: false},
		{field: "city.name", want: false},
	}

	for _, tt := range tests {
		if got := isSupportedDistinctField(tt.field); got != tt.want {
			t.Errorf("isSupportedDistinctField(%q) = %v, want %v", tt.field, got, tt.want)
		}
	}
}

func TestGetMarkdownSummary(t *testing.T) {
	date := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		events []models.Event
		want   string
	}{
		{"empty", nil, ""},
		{"multiple events", []models.Event{
			{Title: "First", URL: "https://example.com/1", Location: "Hall", Date: date},
			{Title: "Second", URL: "https://example.com/2", Location: "Club", Date: date.Add(time.Hour)},
		}, "<https://example.com/1|First> @Hall, 2026-09-03 12:00:00 +0000 UTC\n" +
			"<https://example.com/2|Second> @Club, 2026-09-03 13:00:00 +0000 UTC\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getMarkdownSummary(tt.events); got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateAndSanitizeEventsRejectsInvalidEvents(t *testing.T) {
	valid := models.Event{
		Title: "Title", Location: "Venue", City: "City", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		URL: "https://example.com/event", SourceURL: "https://example.com", Type: "concert",
	}
	tests := []struct {
		name  string
		field string
		edit  func(*models.Event)
	}{
		{"missing title", "Title", func(e *models.Event) { e.Title = "" }},
		{"missing location", "Location", func(e *models.Event) { e.Location = "" }},
		{"missing city", "City", func(e *models.Event) { e.City = "" }},
		{"missing date", "Date", func(e *models.Event) { e.Date = time.Time{} }},
		{"missing type", "Type", func(e *models.Event) { e.Type = "" }},
		{"missing URL", "URL", func(e *models.Event) { e.URL = "" }},
		{"invalid URL", "URL", func(e *models.Event) { e.URL = "not-a-url" }},
		{"missing source URL", "SourceURL", func(e *models.Event) { e.SourceURL = "" }},
		{"invalid source URL", "SourceURL", func(e *models.Event) { e.SourceURL = "not-a-url" }},
		{"invalid image URL", "ImageURL", func(e *models.Event) { e.ImageURL = "not-a-url" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			tt.edit(&event)
			input := []models.Event{event}
			events, errs := validateAndSanitizeEvents(context.Background(), &input)
			if len(*events) != 0 || len(*errs) != 1 || !strings.Contains((*errs)[0].Error, tt.field) {
				t.Fatalf("events = %#v, errors = %#v; want one error for %s", *events, *errs, tt.field)
			}
		})
	}
	t.Run("empty input", func(t *testing.T) {
		input := []models.Event{}
		events, errs := validateAndSanitizeEvents(context.Background(), &input)
		if len(*events) != 0 || len(*errs) != 0 {
			t.Fatalf("events = %#v, errors = %#v; want both empty", *events, *errs)
		}
	})
	t.Run("valid event and optional image", func(t *testing.T) {
		for _, imageURL := range []string{"", "https://example.com/image.jpg"} {
			event := valid
			event.ImageURL = imageURL
			if err := validator.New().Struct(event); err != nil {
				t.Errorf("valid event with image URL %q rejected: %v", imageURL, err)
			}
		}
	})
}

func TestGetDistinctRejectsUnsupportedField(t *testing.T) {
	app := fiber.New()
	app.Get("/:field", GetDistinct)

	response, err := app.Test(httptest.NewRequest("GET", "/country", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusBadRequest)
	}

	var body models.GenericResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error != "the field parameter has to be 'location', 'city', 'genres' or 'type'" {
		t.Fatalf("error = %q", body.Error)
	}
}
