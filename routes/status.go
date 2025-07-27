package routes

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/jakopako/event-api/controllers"
)

func StatusRoute(route fiber.Router) {
	// for some reason auth cannot be defined outside this function
	auth := basicauth.New(basicauth.Config{
		Users: map[string]string{
			os.Getenv("API_USER"): os.Getenv("API_PASSWORD"),
		},
	})
	route.Get("/", controllers.GetScraperStatus)
	route.Post("/", auth, controllers.UpsertScraperStatus)
	route.Delete("/:name", auth, controllers.DeleteScraperStatus)
}
