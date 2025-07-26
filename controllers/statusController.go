package controllers

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	statusCollection := config.MI.DB.Collection("status")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var statuses []models.ScraperStatus
	filter := bson.M{}
	if name != "" {
		filter = bson.M{
			"name": name,
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

func UpdateScraperStatus(c *fiber.Ctx) error {
	return nil
}
