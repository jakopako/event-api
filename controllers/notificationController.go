package controllers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/mail"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AddNotification func for adding a new notification to the database.
// @Description Add new notification to the database.
// @Summary Add new notification.
// @Tags notifications
// @Produce json
// @Param title query string false "title search string"
// @Param location query string false "location search string"
// @Param city query string false "city search string"
// @Param country query string false "country search string"
// @Param radius query int false "radius around given city in kilometers"
// @Param date query string false "date search string"
// @Param email query string false "email"
// @Failure 400 {object} string "Failed to parse body"
// @Failure 500 {object} string "Failed to insert notification"
// @Router /api/notifications/add [get]
func AddNotification(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection("notifications")
	// verify date
	date := ""
	if dateString := c.Query("date"); dateString != "" {
		_, err := time.Parse(time.RFC3339, dateString)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "couldn't parse date",
				"error":   err.Error(),
			})
		}
		date = dateString
	}
	// verify email
	email := c.Query("email")
	if _, err := mail.ParseAddress(email); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "couldn't parse email address",
			"error":   err.Error(),
		})
	}
	// generate token
	token, err := generateRandomString(40)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "failed to generate random token",
			"error":   err.Error(),
		})
	}
	n := models.Notification{
		Email:     email,
		Token:     token,
		SetupDate: time.Now().UTC(),
		Active:    false,
		Query: models.Query{
			Title:   c.Query("title"),
			City:    c.Query("city"),
			Country: c.Query("country"),
			Date:    date,
			Radius:  c.QueryInt("radius"),
		},
	}

	// if err := c.BodyParser(n); err != nil {
	// 	//log.Println(err)
	// 	return c.Status(400).JSON(fiber.Map{
	// 		"success": false,
	// 		"message": "failed to parse body",
	// 		"error":   err.Error(),
	// 	})
	// }

	// validate := validator.New()
	// if err := validate.Struct(n); err != nil {
	// 	return c.Status(400).JSON(fiber.Map{
	// 		"success": false,
	// 		"message": "failed to parse body",
	// 		"error":   err.Error(),
	// 	})
	// }

	// token, err := generateRandomString(40)
	// if err != nil {
	// 	return c.Status(500).JSON(fiber.Map{
	// 		"success": false,
	// 		"message": "failed to generate random token",
	// 		"error":   err.Error(),
	// 	})
	// }
	// n.Token = token
	// n.SetupDate = time.Now().UTC()
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
// @Router /api/notifications/activate [get]
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

func DeleteNotifiction(c *fiber.Ctx) error {
	return nil
}

func DeleteInactiveNotifictions(c *fiber.Ctx) error {
	return nil
}

func SendNotifications(c *fiber.Ctx) error {
	return nil
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
