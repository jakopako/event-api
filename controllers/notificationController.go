package controllers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/go-playground/validator.v9"
)

// AddNotification func for adding a new notification to the database.
// @Description Add new notification to the database.
// @Summary Add new notification.
// @Tags notifications
// @Accept json
// @Produce json
// @Param message body models.Notification true "Notification Info"
// @Failure 400 {object} string "Failed to parse body"
// @Failure 500 {object} string "Failed to insert notification"
// @Router /api/notification/add [post]
func AddNotification(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection("notifications")
	n := new(models.Notification)
	if err := c.BodyParser(n); err != nil {
		//log.Println(err)
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "failed to parse body",
			"error":   err.Error(),
		})
	}

	validate := validator.New()
	if err := validate.Struct(n); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "failed to parse body",
			"error":   err.Error(),
		})
	}

	token, err := generateRandomString(40)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "failed to generate random token",
			"error":   err.Error(),
		})
	}
	n.Token = token
	n.SetupDate = time.Now().UTC()
	filter := bson.D{{"email", n.Email}, {"query", n.Query}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	opts := options.Replace().SetUpsert(true)
	result, err := notificationCollection.ReplaceOne(ctx, filter, n, opts)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "failed to insert notification",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data":    result,
		"success": true,
		"message": "notification inserted successfully",
	})
}

// ActivateNotification func for activating a new notification.
// @Description This endpoint activates a notification that has been added previously.
// @Summary Activate notification.
// @Tags notifications
// @Produce json
// @Param email query string false "email"
// @Param token query string false "token"
// @Failure 400 {object} string "Failed to activate notification"
// @Failure 500 {object} string "Failed to activate notification"
// @Router /api/notification/activate [get]
func ActivateNotification(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection("notifications")
	email := c.Query("email")
	token := c.Query("token")
	update := bson.D{{"$set", bson.D{{"active", true}}}}

	now := time.Now().UTC()
	then := now.AddDate(0, 0, -1)
	filter := bson.D{{"email", email}, {"token", token}, {"setupDate", bson.M{"$gt": then}}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := notificationCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "failed to activate notification",
			"error":   err.Error(),
		})
	}
	return c.SendStatus(fiber.StatusOK)
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
