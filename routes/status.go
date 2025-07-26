package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jakopako/event-api/controllers"
)

func StatusRoute(route fiber.Router) {
	route.Get("/", controllers.GetScraperStatus)
}
