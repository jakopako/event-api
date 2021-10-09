package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/jakopako/croncert-api/config"
	_ "github.com/jakopako/croncert-api/docs"
	"github.com/jakopako/croncert-api/routes"
)

func setupRoutes(app *fiber.App) {
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success":     true,
			"message":     "You are at the root endpoint 😉",
			"github_repo": "https://github.com/MikeFMeyer/catchphrase-go-mongodb-rest-api",
		})
	})

	api := app.Group("/api")
	routes.ConcertsRoute(api.Group("/concerts"))
	routes.SwaggerRoute(api.Group("/swagger"))
}

// https://dev.to/mikefmeyer/build-a-go-rest-api-with-fiber-and-mongodb-44og
// https://dev.to/koddr/build-a-restful-api-on-go-fiber-postgresql-jwt-and-swagger-docs-in-isolated-docker-containers-475j
func main() {
	// if os.Getenv("APP_ENV") != "production" {
	// 	err := godotenv.Load()
	// 	if err != nil {
	// 		log.Fatal("Error loading .env file")
	// 	}
	// }

	app := fiber.New()

	app.Use(cors.New())
	app.Use(logger.New())

	config.ConnectDB()

	setupRoutes(app)

	port := os.Getenv("PORT")
	err := app.Listen(":" + port)

	if err != nil {
		log.Fatal("Error app failed to start")
		panic(err)
	}
}
