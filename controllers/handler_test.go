package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func useMockDatabase(mt *mtest.T) {
	// config.MI is shared by handlers, so tests using this helper must not run in parallel.
	previous := config.MI
	config.MI = config.MongoInstance{Client: mt.Client, DB: mt.DB}
	mt.Cleanup(func() { config.MI = previous })
}

func handlerRequest(t *testing.T, app *fiber.App, method, target, contentType, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeResponse[T any](t *testing.T, resp *http.Response, status int) T {
	t.Helper()
	if resp.StatusCode != status {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body = %s", resp.StatusCode, status, body)
	}
	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func mockCursor(mt *mtest.T, collection string, docs ...bson.D) bson.D {
	return mtest.CreateCursorResponse(0, mt.DB.Name()+"."+collection, mtest.FirstBatch, docs...)
}

func mockDBError() bson.D {
	return mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 123, Message: "database failure"})
}

func deleteFilter(mt *mtest.T, limit int32) bson.Raw {
	mt.Helper()
	event := mt.GetStartedEvent()
	if event == nil || event.CommandName != "delete" {
		mt.Fatalf("expected delete command, got %v", event)
	}
	deletes, err := event.Command.Lookup("deletes").Array().Values()
	if err != nil || len(deletes) != 1 {
		mt.Fatalf("unexpected deletes: %v, %v", deletes, err)
	}
	delete := deletes[0].Document()
	if got := delete.Lookup("limit").Int32(); got != limit {
		mt.Fatalf("delete limit = %d, want %d", got, limit)
	}
	return delete.Lookup("q").Document()
}
