package shared

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jakopako/event-api/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestFetchEventsRejectsInvalidQuery(t *testing.T) {
	tests := []struct {
		name  string
		query models.Query
		want  string
	}{
		{"zero page", models.Query{Limit: 1}, "page parameter must be greater than 0"},
		{"negative page", models.Query{Page: -1, Limit: 1}, "page parameter must be greater than 0"},
		{"zero limit", models.Query{Page: 1}, "limit parameter must be greater than 0"},
		{"negative limit", models.Query{Page: 1, Limit: -1}, "limit parameter must be greater than 0"},
		{"negative radius", models.Query{Page: 1, Limit: 1, Radius: -1}, "radius parameter must be greater than or equal to 0"},
		{"validation order", models.Query{Page: -1, Limit: -1, Radius: -1}, "page parameter must be greater than 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, total, last, err := FetchEvents(tt.query)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if events != nil || total != 0 || last != 0 {
				t.Fatalf("got (%v, %d, %d), want (nil, 0, 0)", events, total, last)
			}
		})
	}
	if err := validateEventQuery(models.Query{Page: 1, Limit: 1, Radius: 0}); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
}

func TestBuildEventFilter(t *testing.T) {
	start := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	lat, lon := 52.5, 13.4
	city := bson.M{"$or": []bson.M{{"city": bson.M{"$regex": primitive.Regex{Pattern: "Berlin.*", Options: "i"}}}}}
	cityWithRadius := bson.M{"$or": []bson.M{
		{"city": bson.M{"$regex": primitive.Regex{Pattern: "Berlin.*", Options: "i"}}},
		{"address.geolocation.coordinates": bson.D{{Key: "$geoWithin", Value: bson.D{
			{Key: "$centerSphere", Value: bson.A{[]float64{13.4, 52.5}, float64(10) / 6378.1}},
		}}}},
	}}
	tests := []struct {
		name        string
		query       models.Query
		conditions  []bson.M
		lookup      bool
		lookupError bool
		noMatches   bool
	}{
		{name: "empty"},
		{name: "end without start is ignored", query: models.Query{EndDate: &end}},
		{name: "inclusive start", query: models.Query{StartDate: &start}, conditions: []bson.M{{"date": bson.M{"$gte": &start}}}},
		{name: "exclusive start", query: models.Query{StartDate: &start, ExcludeStartDate: true}, conditions: []bson.M{{"date": bson.M{"$gt": &start}}}},
		{name: "bounded range remains inclusive", query: models.Query{StartDate: &start, EndDate: &end, ExcludeStartDate: true}, conditions: []bson.M{{"date": bson.M{"$gte": &start}}, {"date": bson.M{"$lte": &end}}}},
		{name: "text and genres", query: models.Query{Title: "Café.*", Location: "A+B", Country: "C.D", Type: "(Live)", Genres: []string{"jazz", "rock"}}, conditions: []bson.M{
			{"$or": []bson.M{
				{"title": bson.M{"$regex": primitive.Regex{Pattern: `Café\.\*`, Options: "i"}}},
				{"normalizedTitle": bson.M{"$regex": primitive.Regex{Pattern: `Cafe\.\*`, Options: "i"}}},
			}},
			{"location": bson.M{"$regex": primitive.Regex{Pattern: `A\+B`, Options: "i"}}},
			{"country": bson.M{"$regex": primitive.Regex{Pattern: `C\.D`, Options: "i"}}},
			{"type": bson.M{"$regex": primitive.Regex{Pattern: `\(Live\)`, Options: "i"}}},
			{"genres": bson.M{"$in": []string{"jazz", "rock"}}},
		}},
		{name: "city without radius", query: models.Query{City: "Berlin.*"}, conditions: []bson.M{city}},
		{name: "city radius uses first match and takes precedence over coordinates", query: models.Query{City: "Berlin.*", Country: "DE", Radius: 10, Lat: &lat, Lon: &lon}, lookup: true, conditions: []bson.M{
			{"country": bson.M{"$regex": primitive.Regex{Pattern: "DE", Options: "i"}}}, cityWithRadius,
		}},
		{name: "city lookup error falls back to city", query: models.Query{City: "Berlin.*", Country: "DE", Radius: 10}, lookup: true, lookupError: true, conditions: []bson.M{
			{"country": bson.M{"$regex": primitive.Regex{Pattern: "DE", Options: "i"}}}, city,
		}},
		{name: "city lookup with no matches", query: models.Query{City: "Berlin.*", Country: "DE", Radius: 10}, lookup: true, noMatches: true, conditions: []bson.M{
			{"country": bson.M{"$regex": primitive.Regex{Pattern: "DE", Options: "i"}}}, city,
		}},
		{name: "exact coordinates", query: models.Query{Lat: &lat, Lon: &lon}, conditions: []bson.M{{"address.geolocation.coordinates": bson.A{lon, lat}}}},
		{name: "coordinate radius", query: models.Query{Lat: &lat, Lon: &lon, Radius: 10}, conditions: []bson.M{{"address.geolocation.coordinates": bson.M{"$geoWithin": bson.M{"$centerSphere": bson.A{bson.A{lon, lat}, float64(10) / 6378.1}}}}}},
		{name: "latitude alone ignored", query: models.Query{Lat: &lat}},
		{name: "longitude alone ignored", query: models.Query{Lon: &lon}},
		{name: "radius alone ignored", query: models.Query{Radius: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			lookup := func(city, country string) ([]*models.MongoGeolocation, error) {
				calls++
				if city != "Berlin.*" || country != "DE" {
					t.Errorf("lookup arguments = (%q, %q)", city, country)
				}
				if tt.lookupError {
					return nil, errors.New("lookup failed")
				}
				if tt.noMatches {
					return nil, nil
				}
				return []*models.MongoGeolocation{{Coordinates: []float64{13.4, 52.5}}, {Coordinates: []float64{1, 2}}}, nil
			}
			got := buildEventFilter(tt.query, lookup)
			wantCalls := 0
			if tt.lookup {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Errorf("lookup calls = %d, want %d", calls, wantCalls)
			}
			if len(tt.conditions) == 0 {
				if len(got) != 0 {
					t.Fatalf("filter = %#v, want empty", got)
				}
				return
			}
			conditions, ok := got["$and"].([]bson.M)
			if !ok || len(got) != 1 || len(conditions) != len(tt.conditions) {
				t.Fatalf("filter = %#v, want conditions %#v", got, tt.conditions)
			}
			for _, want := range tt.conditions {
				found := false
				for _, condition := range conditions {
					if reflect.DeepEqual(condition, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing condition %#v in %#v", want, conditions)
				}
			}
		})
	}
}
