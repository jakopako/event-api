package routes

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/jakopako/croncert-api/controllers"
)

func ConcertsRoute(route fiber.Router) {
	auth := basicauth.New(basicauth.Config{
		Users: map[string]string{
			os.Getenv("API_POST_USER"): os.Getenv("API_POST_PASSWORD"),
		},
	})
	route.Get("/", controllers.GetAllConcerts)
	route.Post("/", auth, controllers.AddConcert)
}
