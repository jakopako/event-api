package genre

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jakopako/event-api/config"
	"github.com/jakopako/event-api/models"
	cache "github.com/patrickmn/go-cache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// GenreCache defines what is needed for querying and caching artist's genres
type GenreCache struct {
	memCache *cache.Cache
	// we use an extra collection to make search faster
	// this way we can store all the titles in lowercase and
	// lowercase the input too when searching for a genre. In
	// the events collection we want to keep the title's case
	// so searching we need to use regex every time which is
	// slow. We're doing that currently in shared.FetchEvents
	// but there it doesn't matter to much for now since these
	// are mostly user-triggered queries.
	genresColl         *mongo.Collection
	allGenres          map[string]bool
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
		if strings.EqualFold(artist, a.Name) {
			return a.Genres, nil
		}
	}

	return []string{}, nil
}

func (gc *GenreCache) queryDBGenres(ctx context.Context, artist string) []string {
	// different results for list in non-error case:
	// - nil : we have never queried the genres for that artist
	// - empty list: we have queried the genres in the past and the answer from Spotify was empty
	// - non-empty list: we have queried the genres in the past and the answer was non-empty
	// only in the first case do we want to query Spotify at a later in querySpotifyGenres
	filter := bson.D{{"title", strings.ToLower(artist)}}

	var result models.TitleGenre
	err := gc.genresColl.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil
	}

	return result.Genres
}

func (gc *GenreCache) extractGenresFromText(genresText string) []string {
	genres := map[string]bool{}
	maxTokens := 4
	genresText = strings.ToLower(strings.ReplaceAll(genresText, "-", " "))
	genresText = regexp.MustCompile(`[^a-z0-9 ]+`).ReplaceAllString(genresText, "")
	tokens := strings.Split(genresText, " ")
	for i := 0; i < len(tokens); i++ {
		prevPotGenre := ""
		j := i
		for ; j < i+maxTokens && j < len(tokens); j++ {
			potGenre := strings.Join(tokens[i:j+1], " ")
			if _, found := gc.allGenres[potGenre]; !found {
				if prevPotGenre != "" {
					break
				}
			} else {
				prevPotGenre = potGenre
			}
		}
		if prevPotGenre != "" {
			genres[prevPotGenre] = true
			i = j - 1
		}
	}

	genresList := []string{}
	for g := range genres {
		genresList = append(genresList, g)
	}
	return genresList
}

func (gc *GenreCache) extractArtistsFromTitle(title string) []string {
	// this function is still pretty basic and might not work for all cases
	regex := regexp.MustCompile(`(?i)(?:,|»|:|!|&|and|feat\.|feat|ft|with|vs\.|vs|versus|presenting|presents|performed by|performed|performed live by|performed live|live by|live|live at|live from|live in|live on|live performance|live recording|live version|live vocals|\([^\)]+\)|\[[^\]]+\]|{[^\}]+\}|<.+>)`)
	title = regex.ReplaceAllString(title, ",")
	title = strings.ToLower(title)
	artists := strings.Split(title, ",")
	j := 0
	for i := range artists {
		a := strings.TrimSpace(artists[i])
		if a != "" {
			artists[j] = a
			j++
		}
	}
	return artists[:j]
}

func (gc *GenreCache) writeDBGenres(ctx context.Context, artist string, genres []string) {
	// we ignore errors for now
	_, _ = gc.genresColl.InsertOne(ctx, models.TitleGenre{Title: strings.ToLower(artist), Genres: genres})
}

func (gc *GenreCache) lookupGenres(ctx context.Context, event models.Event) ([]string, error) {
	if gc.lookupSpotifyGenre {
		genres := gc.extractGenresFromText(event.GenresText)
		if len(genres) > 0 {
			return genres, nil
		}

		genresMap := map[string]bool{}
		artists := gc.extractArtistsFromTitle(event.Title)
		for _, a := range artists {
			// check cache
			genresMem, found := gc.memCache.Get(a)
			if found {
				for _, g := range genresMem.([]string) {
					genresMap[g] = true
				}
				continue
			}

			// find genres in own database
			genresA := gc.queryDBGenres(ctx, a)
			if genresA != nil {
				gc.memCache.Set(a, genresA, cache.DefaultExpiration)
				for _, g := range genresA {
					genresMap[g] = true
				}
				continue
			}

			// query spotify
			if err := gc.renewSpotifyToken(); err != nil {
				return nil, err
			}

			genresA, err := gc.querySpotifyGenres(a)
			if err != nil {
				return nil, err
			}

			gc.writeDBGenres(ctx, a, genresA)
			gc.memCache.Set(a, genresA, cache.DefaultExpiration)
			for _, g := range genresA {
				genresMap[g] = true
			}
		}
		for g := range genresMap {
			genres = append(genres, g)
		}
		return genres, nil
	}

	return nil, nil
}

func loadGenresFromFile() map[string]bool {
	allGenres := map[string]bool{}
	file, err := os.Open("genre-data/genres.txt")
	if err != nil {
		// we ignore errors and simply return the empty map
		return allGenres
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allGenres[strings.ReplaceAll(scanner.Text(), "-", " ")] = true
	}

	return allGenres
}

func InitGenreCache() {
	// this code assumes that the DB has already been initialized
	GC = &GenreCache{
		lookupSpotifyGenre: os.Getenv("LOOKUP_SPOTIFY_GENRE") == "true",
		memCache:           cache.New(10*time.Minute, 15*time.Minute),
		genresColl:         config.MI.DB.Collection("genres"),
		allGenres:          loadGenresFromFile(),
	}
}

func LookupGenres(ctx context.Context, event models.Event) ([]string, error) {
	return GC.lookupGenres(ctx, event)
}
