# event-api

A REST API for storing, querying, and managing events (e.g. concerts). Built with [Go](https://go.dev/), [Fiber](https://gofiber.io/), and [MongoDB](https://www.mongodb.com/).

## Features

- **Events** – add, query, validate, and delete events with rich filtering (title, location, city, country, date range, geo-radius, event type)
- **Notifications** – email subscription system: users sign up with a search query and receive periodic emails when matching events appear
- **Scraper status** – endpoints for scrapers to report their run status (items scraped, errors, logs)
- **Slack integration** – slash-command endpoint that returns today's events for a given city
- **Genre lookup** – optionally enriches events with genre tags via the Spotify API
- **Geolocation** – radius-based search using the [Nominatim](https://nominatim.org/) geocoding service
- **Swagger UI** – interactive API docs available at `/api/swagger/`
- **Rate limiting & caching** – built-in sliding-window rate limiter and response cache

## Requirements

- Go 1.24+
- MongoDB
- (optional) Spotify API credentials for genre lookup
- (optional) SMTP server for email notifications

## Configuration

Copy `.env.example` to `.env` and fill in the values:

```bash
cp .env.example .env
```

Key variables:

| Variable | Description |
|---|---|
| `MONGO_URI` | MongoDB connection string |
| `DB` | Database name |
| `PORT` | Port the server listens on |
| `API_USER` / `API_PASSWORD` | Basic-auth credentials for protected endpoints |
| `SMTP_*` | SMTP settings for notification emails |
| `ACTIVATION_URL` | Full URL to the notification activation endpoint |
| `QUERY_URL` | Full URL to the events endpoint (used in notification emails) |
| `UNSUBSCRIBE_URL` | Full URL to the notification deletion endpoint |
| `LOOKUP_SPOTIFY_GENRE` | Set to `true` to enable genre lookup |
| `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` | Spotify API credentials |

## Running locally

```bash
go run .
```

## Running with Docker

```bash
docker build -t event-api .
docker run --env-file .env -p 5000:5000 event-api
```

## API overview

All endpoints are prefixed with `/api`.

### Events – `/api/events`

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/events` | – | Query events (supports `title`, `location`, `city`, `country`, `type`, `date`, `radius`, `page`, `limit`) |
| `POST` | `/api/events` | ✔ | Add new events (JSON array) |
| `POST` | `/api/events/validate` | – | Validate events without persisting them |
| `DELETE` | `/api/events` | ✔ | Delete events by `sourceUrl` or `datetime` |
| `GET` | `/api/events/:field` | – | Get distinct values for `location` or `city` |
| `POST` | `/api/events/today/slack` | – | Today's events formatted for a Slack slash command |

### Notifications – `/api/notifications`

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/notifications/add` | – | Subscribe to event notifications |
| `GET` | `/api/notifications/activate` | – | Activate a pending notification (via email link) |
| `GET` | `/api/notifications/delete` | – | Unsubscribe from notifications |
| `DELETE` | `/api/notifications/deleteInactive` | ✔ | Delete expired inactive notifications |
| `GET` | `/api/notifications/send` | ✔ | Trigger sending of notification emails |

### Scraper status – `/api/status`

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/status` | – | Query scraper statuses |
| `POST` | `/api/status` | ✔ | Insert or update a scraper status |
| `DELETE` | `/api/status/:name` | ✔ | Delete a scraper status by name |

> **Auth** – protected endpoints use HTTP Basic Auth with the `API_USER` / `API_PASSWORD` credentials.

## Interactive docs

Start the server and open `http://localhost:<PORT>/api/swagger/` in your browser for the full Swagger UI.
