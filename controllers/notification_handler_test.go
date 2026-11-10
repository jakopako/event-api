package controllers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestAddNotificationRequiresActivationURL(t *testing.T) {
	t.Setenv("ACTIVATION_URL", "")
	app := fiber.New()
	app.Get("/", AddNotification)
	got := decodeResponse[models.GenericResponse](t, handlerRequest(t, app, http.MethodGet, "/", "", ""), 500)
	if got.Success || got.Message != "failed to add new notification" || got.Error != "ACTIVATION_URL has to be provided as environment variable" {
		t.Fatalf("response = %+v", got)
	}
}

func TestNotificationHandlersWithMockMongo(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("invalid email", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.Setenv("ACTIVATION_URL", "https://example.com/activate")
		app := fiber.New()
		app.Get("/", AddNotification)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/?email=invalid", "", ""), 400)
		if got.Success || got.Message != "couldn't parse email address" || got.Error == "" || mt.GetStartedEvent() != nil {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("duplicate subscription", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.Setenv("ACTIVATION_URL", "https://example.com/activate")
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 0}))
		app := fiber.New()
		app.Get("/", AddNotification)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/?email=user%40example.com&city=Berlin&radius=10", "", ""), 201)
		if !got.Success || got.Message != "notification already exists in database" {
			mt.Fatalf("response = %+v", got)
		}
		event := mt.GetStartedEvent()
		if event == nil || event.CommandName != "update" {
			mt.Fatalf("expected update command, got %v", event)
		}
	})
	mt.Run("subscription database error", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.Setenv("ACTIVATION_URL", "https://example.com/activate")
		mt.AddMockResponses(mockDBError())
		app := fiber.New()
		app.Get("/", AddNotification)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/?email=user%40example.com", "", ""), 500)
		if got.Success || got.Message != "failed to insert notification" || !strings.Contains(got.Error, "database failure") {
			mt.Fatalf("response = %+v", got)
		}
	})
	for _, tt := range []struct {
		name      string
		responses func(*mtest.T) []bson.D
		status    int
		message   string
	}{
		{"missing or expired", func(mt *mtest.T) []bson.D { return []bson.D{mockCursor(mt, "notifications")} }, 404, "failed to activate notification"},
		{"lookup error", func(mt *mtest.T) []bson.D { return []bson.D{mockDBError()} }, 404, "failed to activate notification"},
		{"already active", func(mt *mtest.T) []bson.D {
			return []bson.D{mockCursor(mt, "notifications", bson.D{{Key: "email", Value: "user@example.com"}, {Key: "token", Value: "token"}, {Key: "active", Value: true}})}
		}, 200, "notification already activated"},
		{"update error", func(mt *mtest.T) []bson.D {
			return []bson.D{mockCursor(mt, "notifications", bson.D{{Key: "email", Value: "user@example.com"}, {Key: "token", Value: "token"}, {Key: "active", Value: false}}), mockDBError()}
		}, 500, "failed to activate notification"},
		{"activated", func(mt *mtest.T) []bson.D {
			return []bson.D{mockCursor(mt, "notifications", bson.D{{Key: "email", Value: "user@example.com"}, {Key: "token", Value: "token"}, {Key: "active", Value: false}}), mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1})}
		}, 200, "successfully activated notification"},
	} {
		mt.Run(tt.name, func(mt *mtest.T) {
			useMockDatabase(mt)
			mt.AddMockResponses(tt.responses(mt)...)
			app := fiber.New()
			app.Get("/", ActivateNotification)
			resp := handlerRequest(mt.T, app, http.MethodGet, "/?email=user%40example.com&token=token", "", "")
			if tt.name == "activated" {
				got := decodeResponse[models.ActivateNotificationResponse](mt.T, resp, tt.status)
				if !got.Success || got.Message != tt.message || !got.Data.Active || got.Data.Email != "user@example.com" {
					mt.Fatalf("response = %+v", got)
				}
			} else {
				got := decodeResponse[models.GenericResponse](mt.T, resp, tt.status)
				if got.Success != (tt.status == 200) || got.Message != tt.message {
					mt.Fatalf("response = %+v", got)
				}
			}
			event := mt.GetStartedEvent()
			if event == nil || event.CommandName != "find" {
				mt.Fatalf("expected find command, got %v", event)
			}
			filter := event.Command.Lookup("filter").Document()
			if filter.Lookup("email").StringValue() != "user@example.com" || filter.Lookup("token").StringValue() != "token" {
				mt.Fatalf("unexpected activation filter: %v", filter)
			}
			cutoff := filter.Lookup("setupDate").Document().Lookup("$gt").Time()
			if time.Since(cutoff) < 23*time.Hour || time.Since(cutoff) > 25*time.Hour {
				mt.Fatalf("unexpected expiry cutoff: %v", cutoff)
			}
		})
	}
	for _, tt := range []struct {
		name     string
		response bson.D
		status   int
	}{
		{"deleted", mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}), 200},
		{"delete error", mockDBError(), 500},
	} {
		mt.Run(tt.name, func(mt *mtest.T) {
			useMockDatabase(mt)
			mt.AddMockResponses(tt.response)
			app := fiber.New()
			app.Get("/", DeleteNotification)
			resp := handlerRequest(mt.T, app, http.MethodGet, "/?email=user%40example.com&token=token", "", "")
			if tt.status == 500 {
				got := decodeResponse[models.GenericResponse](mt.T, resp, tt.status)
				if got.Success || got.Message != "failed to delete notification" || !strings.Contains(got.Error, "database failure") {
					mt.Fatalf("response = %+v", got)
				}
			} else if resp.StatusCode != tt.status {
				mt.Fatalf("status = %d, want %d", resp.StatusCode, tt.status)
			}
			filter := deleteFilter(mt, 1)
			if filter.Lookup("email").StringValue() != "user@example.com" || filter.Lookup("token").StringValue() != "token" {
				mt.Fatalf("unexpected unsubscribe filter: %v", filter)
			}
		})
	}
	for _, tt := range []struct {
		name     string
		response bson.D
		status   int
	}{
		{"cleanup inactive", mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 2}), 200},
		{"cleanup database error", mockDBError(), 500},
	} {
		mt.Run(tt.name, func(mt *mtest.T) {
			useMockDatabase(mt)
			mt.AddMockResponses(tt.response)
			app := fiber.New()
			app.Delete("/", DeleteInactiveNotifictions)
			resp := handlerRequest(mt.T, app, http.MethodDelete, "/", "", "")
			if tt.status == 500 {
				got := decodeResponse[models.GenericResponse](mt.T, resp, tt.status)
				if got.Success || got.Message != "failed to delete notification" || !strings.Contains(got.Error, "database failure") {
					mt.Fatalf("response = %+v", got)
				}
			} else if resp.StatusCode != tt.status {
				mt.Fatalf("status = %d, want %d", resp.StatusCode, tt.status)
			}
			filter := deleteFilter(mt, 0)
			if filter.Lookup("active").Boolean() {
				mt.Fatalf("cleanup must target inactive notifications: %v", filter)
			}
			cutoff := filter.Lookup("setupDate").Document().Lookup("$lt").Time()
			if time.Since(cutoff) < 23*time.Hour || time.Since(cutoff) > 25*time.Hour {
				mt.Fatalf("unexpected expiry cutoff: %v", cutoff)
			}
		})
	}
	mt.Run("send database error", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.AddMockResponses(mockDBError())
		app := fiber.New()
		app.Get("/", SendNotifications)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/", "", ""), 500)
		if got.Success || got.Message != "failed to retreive active notification from database" || !strings.Contains(got.Error, "database failure") {
			mt.Fatalf("response = %+v", got)
		}
	})
	mt.Run("send with no active subscriptions", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.Setenv("QUERY_URL", "https://example.com/events")
		mt.Setenv("UNSUBSCRIBE_URL", "https://example.com/unsubscribe")
		mt.AddMockResponses(mockCursor(mt, "notifications"))
		app := fiber.New()
		app.Get("/", SendNotifications)
		resp := handlerRequest(mt.T, app, http.MethodGet, "/", "", "")
		if resp.StatusCode != 200 {
			mt.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})
	mt.Run("send missing URLs", func(mt *mtest.T) {
		useMockDatabase(mt)
		mt.Setenv("QUERY_URL", "")
		mt.Setenv("UNSUBSCRIBE_URL", "")
		mt.AddMockResponses(mockCursor(mt, "notifications"))
		app := fiber.New()
		app.Get("/", SendNotifications)
		got := decodeResponse[models.GenericResponse](mt.T, handlerRequest(mt.T, app, http.MethodGet, "/", "", ""), 500)
		if got.Success || got.Message != "failed to send emails" || !strings.Contains(got.Error, "QUERY_URL") {
			mt.Fatalf("response = %+v", got)
		}
	})
}
