package controllers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/models"
	"github.com/jakopako/event-api/shared"
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
// @Param email query string false "email"
// @Success 201 {object} models.GenericResponse
// @Failure 400 {object} models.GenericResponse
// @Failure 500 {object} models.GenericResponse
// @Router /api/notifications/add [get]
func AddNotification(c *fiber.Ctx) error {
	baseAURL := os.Getenv("ACTIVATION_URL")
	baseUURL := os.Getenv("UNSUBSCRIBE_URL")
	if baseAURL == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to add new notification",
			Error:   "ACTIVATION_URL has to be provided as environment variable",
		})
	}
	notificationCollection := config.MI.DB.Collection(shared.NotificationCollectionName)

	// verify email
	email := c.Query("email")
	if _, err := mail.ParseAddress(email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.GenericResponse{
			Success: false,
			Message: "couldn't parse email address",
			Error:   err.Error(),
		})
	}

	// generate token
	token, err := generateRandomString(40)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to generate random token",
			Error:   err.Error(),
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
			Radius:  c.QueryInt("radius"),
			Limit:   10,
			Page:    1,
		},
	}

	update := bson.M{
		"$setOnInsert": n,
	}

	filter := bson.D{{Key: "email", Value: n.Email}, {Key: "query", Value: n.Query}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	opts := options.Update().SetUpsert(true)
	result, err := notificationCollection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to insert notification",
			Error:   err.Error(),
		})
	}

	if result.MatchedCount == 1 {
		return c.Status(fiber.StatusCreated).JSON(models.GenericResponse{
			Success: true,
			Message: "notification already exists in database",
		})
	}

	// send activation email
	uUrl := fmt.Sprintf("%s?email=%s&token=%s", baseUURL, url.QueryEscape(n.Email), url.QueryEscape(n.Token))
	aUrl := fmt.Sprintf("%s?email=%s&token=%s", baseAURL, url.QueryEscape(n.Email), url.QueryEscape(n.Token))
	mTempl := `
Hi,
<br><br>
Click <a href=%s>here</a> to activate your concertcloud.live notification.
<br><br>
If you did not subscribe to any notification on concertcloud.live you can safely ignore this email.
<br><br>
To unsubscribe from this notification in the future click <a href=%s>here</a>.
<br><br>
Your ConcertCloud team
`
	message := fmt.Sprintf(mTempl, aUrl, uUrl)
	if err := sendEmail(n.Email, "notification activation", message); err != nil {
		// TODO: we should delete the notification in the database again
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to send activation email",
			Error:   err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(models.GenericResponse{
		Success: true,
		Message: "successfully added notification to the database",
	})
}

// ActivateNotification func for activating a new notification.
// @Description This endpoint activates a notification that has been added previously if the inactive notification hasn't expired yet (expires after 24h).
// @Summary Activate notification.
// @Tags notifications
// @Produce json
// @Param email query string false "email"
// @Param token query string false "token"
// @Success 200 {object} models.ActivateNotificationResponse
// @Failure 404 {object} models.GenericResponse
// @Failure 500 {object} models.GenericResponse
// @Router /api/notifications/activate [get]
func ActivateNotification(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection(shared.NotificationCollectionName)
	email := c.Query("email")
	token := c.Query("token")

	// check notifications
	now := time.Now().UTC()
	then := now.AddDate(0, 0, -1)
	filter := bson.D{{Key: "email", Value: email}, {Key: "token", Value: token}, {Key: "setupDate", Value: bson.M{"$gt": then}}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	var not models.Notification
	err := notificationCollection.FindOne(ctx, filter).Decode(&not)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to activate notification",
		})
	}

	if not.Active {
		return c.Status(fiber.StatusOK).JSON(models.GenericResponse{
			Success: true,
			Message: "notification already activated",
		})
	}

	update := bson.D{{Key: "$set", Value: bson.D{{Key: "active", Value: true}}}}
	_, err = notificationCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to activate notification",
			Error:   err.Error(),
		})
	}
	not.Active = true
	return c.Status(fiber.StatusOK).JSON(models.ActivateNotificationResponse{
		Data:    not,
		Success: true,
		Message: "successfully activated notification",
	})
}

// DeleteNotification func for deleting an existing notification.
// @Description This endpoint deletes a notification that has been added previously based on the email address and the token.
// @Summary Delete notification.
// @Tags notifications
// @Produce json
// @Param email query string false "email"
// @Param token query string false "token"
// @Success 200
// @Failure 500 {object} models.GenericResponse
// @Router /api/notifications/delete [get]
func DeleteNotification(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection(shared.NotificationCollectionName)
	email := c.Query("email")
	token := c.Query("token")

	filter := bson.D{{Key: "email", Value: email}, {Key: "token", Value: token}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := notificationCollection.DeleteOne(ctx, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to delete notification",
			Error:   err.Error(),
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
// @Success 200
// @Failure 500 {object} models.GenericResponse
// @Router /api/notifications/deleteInactive [delete]
func DeleteInactiveNotifictions(c *fiber.Ctx) error {
	notificationCollection := config.MI.DB.Collection(shared.NotificationCollectionName)

	now := time.Now().UTC()
	then := now.AddDate(0, 0, -1)
	filter := bson.D{{Key: "active", Value: false}, {Key: "setupDate", Value: bson.M{"$lt": then}}}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := notificationCollection.DeleteMany(ctx, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to delete notification",
			Error:   err.Error(),
		})
	}
	return c.SendStatus(fiber.StatusOK)
}

// SendNotifications func for sending all notifications via email.
// @Description This endpoint sends an email for every active notification whose query returns a result.
// @Summary Send notifications.
// @Tags notifications
// @Produce json
// @Security BasicAuth
// @Success 200
// @Failure 500 {object} models.GenericResponse
// @Router /api/notifications/send [get]
func SendNotifications(c *fiber.Ctx) error {
	// fetch active notifications
	notificationCollection := config.MI.DB.Collection(shared.NotificationCollectionName)
	filter := bson.D{{Key: "active", Value: true}}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	cursor, err := notificationCollection.Find(ctx, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to retreive active notification from database",
			Error:   err.Error(),
		})
	}

	var results []models.Notification
	if err = cursor.All(ctx, &results); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to retreive active notification from database",
			Error:   err.Error(),
		})
	}

	baseQURL := os.Getenv("QUERY_URL")
	baseUURL := os.Getenv("UNSUBSCRIBE_URL")
	if baseQURL == "" || baseUURL == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(models.GenericResponse{
			Success: false,
			Message: "failed to send emails",
			Error:   "QUERY_URL and UNSUBSCRIBE_URL have to be provided as environment variables",
		})
	}

	for _, n := range results {
		cursor.Decode(&n)
		// the start date of a notification query is always now, the moment the notification is sent
		now := time.Now().UTC()
		n.Query.StartDate = &now
		_, total, _, err := shared.FetchEvents(n.Query)
		if err != nil {
			log.Errorf("couldn't fetch events for query %v", n.Query)
		}
		if total > 0 {
			// send notification email
			qUrl := fmt.Sprintf("%s?title=%s&city=%s&country=%s&location=%s&radius=%d",
				baseQURL,
				url.QueryEscape(n.Query.Title),
				url.QueryEscape(n.Query.City),
				url.QueryEscape(n.Query.Country),
				url.QueryEscape(n.Query.Location),
				n.Query.Radius)
			uUrl := fmt.Sprintf("%s?token=%s&email=%s", baseUURL, url.QueryEscape(n.Token), url.QueryEscape(n.Email))
			mTempl := `
Hi,
<br><br>
We found a concert for you! Click <a href=%s>here</a> for more information.
<br><br>
To unsubscribe from this notification click <a href=%s>here</a>.
<br><br>
Your ConcertCloud team
`
			message := fmt.Sprintf(mTempl, qUrl, uUrl)
			err = sendEmail(n.Email, "Hurray, a match!", message)
			if err != nil {
				log.Errorf("couldn't send notification email to %s. Error: %v", n.Email, err)
			} else {
				log.Infof("sent notification email to %s", n.Email)
			}
		}
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

func sendEmail(to, subject, message string) error {
	user := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASSWORD")

	from := user
	toList := []string{
		to,
	}

	host := os.Getenv("SMTP_HOST")
	addr := fmt.Sprintf("%s:%s", host, os.Getenv("SMTP_PORT"))

	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n"+
		"%s\r\n", from, to, subject, message))

	auth := smtp.PlainAuth("", user, password, host)

	return smtp.SendMail(addr, auth, from, toList, msg)
}
