package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/croncert-api/controllers"
)

func ConcertsRoute(route fiber.Router) {
	route.Get("/", controllers.GetAllConcerts)
	route.Post("/", controllers.AddConcert)
}
