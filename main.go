package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/jakopako/event-api/config"
	_ "github.com/jakopako/event-api/docs"
	"github.com/jakopako/event-api/geo"
	"github.com/jakopako/event-api/routes"
	_ "github.com/joho/godotenv/autoload"
)

func setupRoutes(app *fiber.App) {
	api := app.Group("/api")
	routes.EventsRoute(api.Group("/events"))
	routes.SwaggerRoute(api.Group("/swagger"))
}

// https://dev.to/mikefmeyer/build-a-go-rest-api-with-fiber-and-mongodb-44og
// https://dev.to/koddr/build-a-restful-api-on-go-fiber-postgresql-jwt-and-swagger-docs-in-isolated-docker-containers-475j
func main() {
	app := fiber.New()

	app.Use(cors.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} ${latency} ${method} ${url}\n",
	}))
	app.Use(cache.New(cache.Config{
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/api/events" && (c.Method() == "POST" || c.Method() == "DELETE")
		},
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return utils.CopyString(c.OriginalURL())
		},
	}))

	// initialize DB and geoloc cache
	config.ConnectDB()
	geo.InitGeolocCache()

	setupRoutes(app)

	port := os.Getenv("PORT")
	err := app.Listen(":" + port)

	if err != nil {
		log.Fatalf("Error app failed to start: %v", err)
		panic(err)
	}
}
