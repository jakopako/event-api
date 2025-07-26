package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/jakopako/event-api/config"
	_ "github.com/jakopako/event-api/docs"
	"github.com/jakopako/event-api/genre"
	"github.com/jakopako/event-api/geo"
	"github.com/jakopako/event-api/routes"
	_ "github.com/joho/godotenv/autoload"
)

func setupRoutes(app *fiber.App) {
	api := app.Group("/api")
	routes.EventsRoute(api.Group("/events"))
	routes.NotificationsRoute(api.Group("/notifications"))
	routes.StatusRoute(api.Group("/status"))
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

	app.Use(limiter.New(limiter.Config{
		Next: func(c *fiber.Ctx) bool {
			return !strings.HasPrefix(c.Path(), "/api/notifications") && !strings.HasPrefix(c.Path(), "/api/events/validate")
		},
		Max:               20,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
	}))

	app.Use(cache.New(cache.Config{
		Next: func(c *fiber.Ctx) bool {
			return (c.Path() == "/api/events" && (c.Method() == "POST" || c.Method() == "DELETE")) || strings.HasPrefix(c.Path(), "/api/notifications")
		},
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return utils.CopyString(c.OriginalURL())
		},
	}))

	// initialize DB and geoloc cache
	config.ConnectDB()
	geo.InitGeolocCache()
	genre.InitGenreCache()

	setupRoutes(app)

	port := os.Getenv("PORT")
	err := app.Listen(":" + port)

	if err != nil {
		log.Fatalf("Error app failed to start: %v", err)
		panic(err)
	}
}
