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

// GetAllConcerts func gets all concerts.
// @Description Get all concerts.
// @Summary Get all concerts.
// @Tags Concerts
// @Accept json
// @Produce json
// @Param s query string false "search string"
// @Param page query int false "page number"
// @Param limit query int false "page size"
// @Success 200 {array} models.Concert
// @Router /api/concerts [get]
func GetAllConcerts(c *fiber.Ctx) error {
	concertCollection := config.MI.DB.Collection("concerts")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var concerts []models.Concert
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

	if s := c.Query("s"); s != "" {
		filter["$and"] = append(filter["$and"].([]bson.M), bson.M{
			"$or": []bson.M{
				{
					"artist": bson.M{
						"$regex": primitive.Regex{
							Pattern: s,
							Options: "i",
						},
					},
				},
			},
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limitVal, _ := strconv.Atoi(c.Query("limit", "10"))
	var limit int64 = int64(limitVal)

	total, _ := concertCollection.CountDocuments(ctx, filter)

	findOptions.SetSkip((int64(page) - 1) * limit)
	findOptions.SetLimit(limit)

	cursor, err := concertCollection.Find(ctx, filter, findOptions)
	defer cursor.Close(ctx)

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Concerts Not found",
			"error":   err,
		})
	}

	for cursor.Next(ctx) {
		var concert models.Concert
		cursor.Decode(&concert)
		concerts = append(concerts, concert)
	}

	last := math.Ceil(float64(total) / float64(limit))
	if last < 1 && total > 0 {
		last = 1
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data":      concerts,
		"total":     total,
		"page":      page,
		"last_page": last,
		"limit":     limit,
	})
}

// AddConcert func for adding a new concert to the database.
// @Description Add a new concert.
// @Summary Add a new concert.
// @Tags Concerts
// @Accept json
// @Produce json
// @Param message body models.Concert true "Concert Info"
// @Failure 400 {object} string "Failed to parse body"
// @Failure 500 {object} string "Failed to insert concert"
// @Router /api/concerts [post]
func AddConcert(c *fiber.Ctx) error {
	concertCollection := config.MI.DB.Collection("concerts")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	concert := new(models.Concert)

	if err := c.BodyParser(concert); err != nil {
		//log.Println(err)
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Failed to parse body",
			"error":   err,
		})
	}

	validate := validator.New()
	err := validate.Struct(concert)

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
	// an existing concert needs to be updated or a new concert needs to be added.
	filterConcert := models.Concert{
		Artist:   concert.Artist,
		Date:     concert.Date,
		Location: concert.Location,
		Link:     concert.Link,
	}
	result, err := concertCollection.ReplaceOne(ctx, filterConcert, concert, opts)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Failed to insert concert",
			"error":   err,
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data":    result,
		"success": true,
		"message": "Concert inserted successfully",
	})

}
