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

	update := bson.M{
		"$setOnInsert": n,
	}

	filter := bson.D{{"email", n.Email}, {"query", n.Query}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	opts := options.Update().SetUpsert(true)
	result, err := notificationCollection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "failed to insert notification",
			"error":   err.Error(),
		})
	}

	if result.MatchedCount == 1 {
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"data":    result,
			"success": true,
			"message": "notification already exists in database",
		})
	}

	// send activation email
	// publicKey := os.Getenv("MJ_APIKEY_PUBLIC")
	// secretKey := os.Getenv("MJ_APIKEY_PRIVATE")
	// mj := mailjet.NewMailjetClient(publicKey, secretKey)
	// recipientName := strings.Split(n.Email, "@")[0]
	// messagesInfo := []mailjet.InfoMessagesV31{
	// 	{
	// 		From: &mailjet.RecipientV31{
	// 			Email: "activation@concertcloud.live",
	// 			Name:  "ConcertCloud",
	// 		},
	// 		To: &mailjet.RecipientsV31{
	// 			mailjet.RecipientV31{
	// 				Email: n.Email,
	// 				Name:  recipientName,
	// 			},
	// 		},
	// 		Subject:  "Activate your notification",
	// 		TextPart: fmt.Sprintf("Hi,\n\n please activate your notification with the following token %s", token),
	// 		HTMLPart: fmt.Sprintf("Hi,\n\n please activate your notification with the following token %s", token),
	// 	},
	// }
	// messages := mailjet.MessagesV31{Info: messagesInfo}
	// res, err := mj.SendMailV31(&messages)
	// if err != nil {
	// 	return c.Status(500).JSON(fiber.Map{
	// 		"success": false,
	// 		"message": "failed to send activation email",
	// 		"error":   err.Error(),
	// 	})
	// }
	// fmt.Printf("Data: %+v\n", res)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": "res",
		// "data":    res,
		"success": true,
		"message": "successfully added notification to the database",
	})
}

// ActivateNotification func for activating a new notification.
// @Description This endpoint activates a notification that has been added previously if the inactive notification hasn't expired yet (expires after 24h).
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

// DeleteNotification func for deleting an existing notification.
// @Description This endpoint deletes a notification that has been added previously based on the email address and the token.
// @Summary Delete notification.
// @Tags notifications
// @Produce json
// @Param email query string false "email"
// @Param token query string false "token"
// @Failure 500 {object} string "Failed to delete notification"
// @Router /api/notifications/delete [delete]
func DeleteNotifiction(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection("notifications")
	email := c.Query("email")
	token := c.Query("token")

	filter := bson.D{{"email", email}, {"token", token}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := notificationCollection.DeleteOne(ctx, filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "failed to delete notification",
			"error":   err.Error(),
		})
	}
	return c.SendStatus(fiber.StatusOK)

}

// DeleteInactiveNotifictions func for deleting all inactive notifications.
// @Description This endpoint deletes all inactive notification that are older than 24h.
// @Summary Delete inactive notifications.
// @Tags notifications
// @Produce json
// @Security BasicAuth
// @Failure 500 {object} string "Failed to delete notifications"
// @Router /api/notifications/deleteInactive [delete]
func DeleteInactiveNotifictions(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection("notifications")

	filter := bson.D{{"active", false}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := notificationCollection.DeleteMany(ctx, filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "failed to delete notification",
			"error":   err.Error(),
		})
	}
	return c.SendStatus(fiber.StatusOK)
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
