package routes

import (
	"github.com/gofiber/fiber/v2"

	swagger "github.com/arsmn/fiber-swagger/v2"
)

func SwaggerRoute(route fiber.Router) {
	route.Get("*", swagger.Handler)
}
