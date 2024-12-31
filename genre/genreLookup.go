package genre

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jakopako/event-api/config"
	"go.mongodb.org/mongo-driver/mongo"
)

type GenreCache struct {
	lookupSpotifyGenre bool
	spotifyToken       string
	spotifyTokenExpiry time.Time
	mu                 sync.RWMutex
	eventsColl         *mongo.Collection
	spotifyErrorCount  int // do we need this? if the spotify responds with an error we probably don't want to hammer it with 1000s of requests, so might need to stop querying the api after a certain error count
}

type spotifyTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // minutes
}

var GC *GenreCache

func (gc *GenreCache) renewSpotifyToken() error {
	client := http.Client{}
	if gc.spotifyToken == "" || gc.spotifyTokenExpiry.After(time.Now().UTC()) {
		tokenUrl := "https://accounts.spotify.com/api/token"
		clientId := os.Getenv("SPOTIFY_CLIENT_ID")
		clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
		if clientId == "" || clientSecret == "" {
			return fmt.Errorf("env vars SPOTIFY_CLIENT_ID and/or SPOTIFY_CLIENT_SECRET are empty")
		}

		form := url.Values{}
		form.Add("grant_type", "client_credentials")
		form.Add("client_id", clientId)
		form.Add("client_secret", clientSecret)

		req, _ := http.NewRequest("POST", tokenUrl, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to do token request. %+w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read token response body. %+w", err)
		}

		var tokenResp spotifyTokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return fmt.Errorf("failed to unmarshal token response. %+w", err)
		}

		gc.spotifyToken = tokenResp.AccessToken
		// the following does not seem quite correct
		gc.spotifyTokenExpiry = time.Now().UTC().Add(time.Minute * time.Duration(tokenResp.ExpiresIn-1))
	}
	return nil
}

func (gc *GenreCache) getSpoticyGenres(artist string) ([]string, error) {
	genres := []string{}

	if gc.lookupSpotifyGenre {
		if err := gc.renewSpotifyToken(); err != nil {
			return genres, err
		}

		// find genres
	}

	return genres, nil
}

func InitGenreCache() {
	// this code assumes that the DB has already been initialized

	GC = &GenreCache{
		lookupSpotifyGenre: os.Getenv("LOOKUP_SPOTIFY_GENRE") == "true",
		eventsColl:         config.MI.DB.Collection("events"),
	}
}

func LookupGenres(artist string) ([]string, error) {
	return GC.getSpoticyGenres(artist)
}
