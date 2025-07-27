package controllers

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/models"
	"github.com/jakopako/event-api/shared"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/go-playground/validator.v9"
)

// GetScraperStatus func gets scraper status.
// @Description This endpoint returns all scraper statuses matching the search terms.
// @Summary Get scraper status.
// @Tags scraper status
// @Accept json
// @Produce json
// @Param name query string false "scraper name search string"
// @Param page query int false "page number"
// @Param limit query int false "page size"
// @Success 200 {array} models.ScraperStatus
// @Failure 404 {object} string "No scraper status found"
// @Failure 400 {object} string "Bad request"
// @Router /api/status [get]
func GetScraperStatus(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1")) // page is 0 when the parameter is not parsable as int
	if page < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "failed to fetch statuses",
			"error":   "page parameter must be greater than 0",
		})
	}
	limitInt, _ := strconv.Atoi(c.Query("limit", "10"))
	if limitInt < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "failed to fetch statuses",
			"error":   "limit parameter must be greater than 0",
		})
	}
	var limit int64 = int64(limitInt)
	name := c.Query("name", "")

	statusCollection := config.MI.DB.Collection(shared.ScraperStatusCollectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var statuses []models.ScraperStatus
	filter := bson.M{}
	if name != "" {
		filter = bson.M{
			"scraperName": name,
		}
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "name", Value: 1}})
	findOptions.SetSkip((int64(page) - 1) * limit)
	findOptions.SetLimit(limit)

	total, _ := statusCollection.CountDocuments(ctx, filter)

	cursor, err := statusCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "failed to fetch statuses",
			"error":   fmt.Sprintf("statuses not found: %v", err),
		})
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var status models.ScraperStatus
		cursor.Decode(&status)
		statuses = append(statuses, status)
	}

	last := math.Ceil(float64(total) / float64(limit))
	if last < 1 && total > 0 {
		last = 1
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data":      statuses,
		"total":     total,
		"page":      page,
		"last_page": last,
		"limit":     limit,
	})
}

// UpsertScraperStatus inserts or updates the status of a scraper.
// @Description This endpoint inserts or updates the status of a scraper, based on the given name. For now, per scraper only one status is allowed.
// @Summary Update or insert scraper status.
// @Tags scraper status
// @Accept json
// @Produce json
// @Security BasicAuth
// @Param status body models.ScraperStatus true "Scraper status object"
// @Success 200 {object} models.ScraperStatus
// @Failure 400 {object} string "Bad request"
// @Failure 500 {object} string "Internal server error"
// @Router /api/status [post]
func UpsertScraperStatus(c *fiber.Ctx) error {
	statusCollection := config.MI.DB.Collection(shared.ScraperStatusCollectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	validate := validator.New()

	var status models.ScraperStatus
	if err := c.BodyParser(&status); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "failed to parse status",
			"error":   fmt.Sprintf("bad request: %v", err),
		})
	}

	err := validate.Struct(status)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "failed to validate status",
			"error":   fmt.Sprintf("validation error: %v", err),
		})
	}

	if status.LastScrapeEnd.Before(status.LastScrapeStart) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "failed to upsert status",
			"error":   "lastScrapeEnd must be after lastScrapeStart",
		})
	}

	filter := bson.M{"scraperName": status.ScraperName}
	opts := options.Update().SetUpsert(true)
	update := bson.M{
		"$set": status,
	}
	result, err := statusCollection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "failed to upsert status",
			"error":   fmt.Sprintf("internal server error: %v", err),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "status upserted successfully",
		"data":    result,
	})
}

// DeleteScraperStatus deletes the status of a scraper by name.
// @Description This endpoint deletes the status of a scraper by name.
// @Summary Delete scraper status.
// @Tags scraper status
// @Produce json
// @Security BasicAuth
// @Param name path string true "Scraper name"
// @Success 200 {object} string "Status deleted successfully"
// @Failure 400 {object} string "Bad request"
// @Failure 404 {object} string "Status not found"
// @Failure 500 {object} string "Internal server error"
// @Router /api/status/{name} [delete]
func DeleteScraperStatus(c *fiber.Ctx) error {
	statusCollection := config.MI.DB.Collection(shared.ScraperStatusCollectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scraperName, err := url.QueryUnescape(c.Params("name"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "failed to decode scraper name",
			"error":   fmt.Sprintf("invalid scraper name: %v", err),
		})
	}
	if scraperName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "failed to delete status",
			"error":   "scraper name is required",
		})
	}

	filter := bson.M{"scraperName": scraperName}
	result, err := statusCollection.DeleteOne(ctx, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "failed to delete status",
			"error":   fmt.Sprintf("internal server error: %v", err),
		})
	}

	if result.DeletedCount == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "status not found",
			"error":   fmt.Sprintf("no status found for scraper: %s", scraperName),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "status deleted successfully",
		"data":    result,
	})
}
