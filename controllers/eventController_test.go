package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/models"
)

func TestParseTimeRange(t *testing.T) {
	from := "2026-09-03T12:00:00Z"
	to := "2026-09-03T14:00:00+02:00"

	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{name: "from only", from: from},
		{name: "from and to", from: from, to: to},
		{name: "to requires from", to: to, wantErr: true},
		{name: "invalid from", from: "not-a-time", wantErr: true},
		{name: "invalid to", from: from, to: "not-a-time", wantErr: true},
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

func TestEventHandlersRejectLegacyAndRangeParametersTogether(t *testing.T) {
	tests := []struct {
		name    string
		handler fiber.Handler
		url     string
	}{
		{
			name:    "get",
			handler: GetAllEvents,
			url:     "/?date=2026-09-03T12:00:00Z&fromTime=2026-09-03T12:00:00Z",
		},
		{
			name:    "delete",
			handler: DeleteEvents,
			url:     "/?datetime=2026-09-03%2012:00&fromTime=2026-09-03T12:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.All("/", tt.handler)

			response, err := app.Test(httptest.NewRequest("GET", tt.url, nil))
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if response.StatusCode != fiber.StatusBadRequest {
				t.Errorf("status = %d, want %d", response.StatusCode, fiber.StatusBadRequest)
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
	}

	for _, tt := range tests {
		if got := isSupportedDistinctField(tt.field); got != tt.want {
			t.Errorf("isSupportedDistinctField(%q) = %v, want %v", tt.field, got, tt.want)
		}
	}
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
