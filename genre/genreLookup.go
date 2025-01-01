package genre

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jakopako/event-api/models"
	"github.com/jakopako/event-api/shared"
)

// GenreCache defines what is needed for querying and caching artist's genres
// Contrary to the geolocation cache an in-memory cache probably doesn't make
// sense for the artist genres since there'd be a looot
type GenreCache struct {
	lookupSpotifyGenre bool
	spotifyToken       string
	spotifyTokenExpiry time.Time
	spotifyErrorCount  int // do we need this? if the spotify responds with an error we probably don't want to hammer it with 1000s of requests, so might need to stop querying the api after a certain error count
}

type spotifyTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // minutes
}

type spotifyArtistsResponse struct {
	Artists struct {
		Href     string `json:"href"`
		Limit    int    `json:"limit"`
		Next     string `json:"next"`
		Offset   int    `json:"offset"`
		Previous any    `json:"previous"`
		Total    int    `json:"total"`
		Items    []struct {
			ExternalUrls struct {
				Spotify string `json:"spotify"`
			} `json:"external_urls"`
			Followers struct {
				Href  any `json:"href"`
				Total int `json:"total"`
			} `json:"followers"`
			Genres []string `json:"genres"`
			Href   string   `json:"href"`
			ID     string   `json:"id"`
			Images []struct {
				URL    string `json:"url"`
				Height int    `json:"height"`
				Width  int    `json:"width"`
			} `json:"images"`
			Name       string `json:"name"`
			Popularity int    `json:"popularity"`
			Type       string `json:"type"`
			URI        string `json:"uri"`
		} `json:"items"`
	} `json:"artists"`
}

var GC *GenreCache

func (gc *GenreCache) renewSpotifyToken() error {
	client := http.Client{}
	if gc.spotifyToken == "" || gc.spotifyTokenExpiry.Before(time.Now().UTC()) {
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

		req, _ := http.NewRequest(http.MethodPost, tokenUrl, strings.NewReader(form.Encode()))
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
		gc.spotifyTokenExpiry = time.Now().UTC().Add(time.Second * time.Duration(tokenResp.ExpiresIn-1))
	}
	return nil
}

func (gc *GenreCache) querySpotifyGenres(artist string) ([]string, error) {
	client := http.Client{}
	requestUrl := fmt.Sprintf("https://api.spotify.com/v1/search?q=%s&type=artist", url.QueryEscape(strings.ToLower(artist)))
	bearer := "Bearer " + gc.spotifyToken
	req, _ := http.NewRequest(http.MethodGet, requestUrl, nil)
	req.Header.Add("Authorization", bearer)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do artist search spotify request. %+w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read artist search response body. %+w", err)
	}

	var sar spotifyArtistsResponse
	if err := json.Unmarshal(body, &sar); err != nil {
		return nil, fmt.Errorf("failed to unmarshal artist search response. %+w", err)
	}

	for _, a := range sar.Artists.Items {
		if strings.EqualFold(a.Name, artist) {
			return a.Genres, nil
		}
	}

	return []string{}, nil
}

func (gc *GenreCache) queryDBGenres(artist string) ([]string, error) {
	// different results for list in non-error case:
	// - nil : we have never queried the genres for that artist
	// - empty list: we have queried the genres in the past and the answer from Spotify was empty
	// - non-empty list: we have queried the genres in the past and the answer was non-empty
	// only in the first case do we want to query Spotify at a later in querySpotifyGenres
	events, _, _, err := shared.FetchEvents(models.Query{Title: artist, Page: 1, Limit: 10})
	if err != nil {
		return nil, err
	}

	for _, e := range events {
		if e.Genres != nil {
			return e.Genres, nil
		}
	}

	return nil, nil
}

func (gc *GenreCache) lookupGenres(artist string) ([]string, error) {
	if gc.lookupSpotifyGenre {
		// find genres in own database
		genres, err := gc.queryDBGenres(artist)
		if err != nil {
			return nil, err
		}
		if genres != nil {
			return genres, nil
		}

		// query spotify
		if err := gc.renewSpotifyToken(); err != nil {
			return nil, err
		}

		return gc.querySpotifyGenres(artist)
	}

	return nil, nil
}

func InitGenreCache() {
	// this code assumes that the DB has already been initialized

	GC = &GenreCache{
		lookupSpotifyGenre: os.Getenv("LOOKUP_SPOTIFY_GENRE") == "true",
	}
}

func LookupGenres(artist string) ([]string, error) {
	return GC.lookupGenres(artist)
}
