package routes

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/jakopako/event-api/controllers"
)

func EventsRoute(route fiber.Router) {
	auth := basicauth.New(basicauth.Config{
		Users: map[string]string{
			os.Getenv("API_USER"): os.Getenv("API_PASSWORD"),
		},
	})
	route.Get("/", controllers.GetAllEvents)
	route.Post("/", auth, controllers.AddEvents)
	route.Delete("/", auth, controllers.DeleteEvents)
	route.Get("/:field", controllers.GetDistinct)
	route.Post("/today/slack", controllers.GetTodaysEventsSlack)
}

func NotificationsRoute(route fiber.Router) {
	route.Post("/add", controllers.AddNotification)
	route.Get("/activate", controllers.ActivateNotification)
}
