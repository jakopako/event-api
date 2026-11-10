package genre

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/go-test/deep"
	"github.com/jakopako/event-api/models"
)

var allGenres = map[string]bool{
	"elektro":     true,
	"house":       true,
	"tech house":  true,
	"techno":      true,
	"jazz":        true,
	"jazz fusion": true,
	"disco":       true,
	"deep house":  true,
}

func TestExtractGenresFromTextEdgeCases(t *testing.T) {
	gc := GenreCache{allGenres: allGenres}
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"empty", "", []string{}},
		{"no matches", "classical performance", []string{}},
		{"case and hyphens", "DEEP-HOUSE", []string{"deep house"}},
		{"accents", "técHno", []string{"techno"}},
		{"repeated genre", "jazz jazz", []string{"jazz"}},
		{"longest match", "jazz fusion", []string{"jazz fusion"}},
		{"distinct genres", "DISCO techno", []string{"disco", "techno"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gc.extractGenresFromText(tt.text)
			slices.Sort(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractGenresFromText(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestExtractArtistsFromTitleEdgeCases(t *testing.T) {
	gc := GenreCache{}
	tests := []struct {
		name  string
		title string
		want  []string
	}{
		{"empty", "", []string{}},
		{"brackets and separators", "Alpha [UK] & Beta (live), Gamma", []string{"alpha", "beta", "gamma"}},
		{"whitespace and empty names", " , Alpha ,, Beta ", []string{"alpha", "beta"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gc.extractArtistsFromTitle(tt.title); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractArtistsFromTitle(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}

func TestLookupGenresDisabled(t *testing.T) {
	gc := GenreCache{}
	genres, err := gc.lookupGenres(context.Background(), models.Event{Title: "Artist", GenresText: "jazz"})
	if err != nil || genres != nil {
		t.Fatalf("lookupGenres() = (%v, %v), want (nil, nil)", genres, err)
	}
}

func TestLookupGenresFromText(t *testing.T) {
	gc := GenreCache{lookupSpotifyGenre: true, allGenres: allGenres}
	genres, err := gc.lookupGenres(context.Background(), models.Event{Title: "Artist", GenresText: "Jazz Fusion and Disco"})
	slices.Sort(genres)
	if err != nil || !reflect.DeepEqual(genres, []string{"disco", "jazz fusion"}) {
		t.Fatalf("lookupGenres() = (%v, %v), want ([disco jazz fusion], nil)", genres, err)
	}
}

func TestDuplicateGenre1(t *testing.T) {
	gc := GenreCache{
		allGenres: allGenres,
	}

	genresText := "2025 # Elektro # Resident # Show # Tech House # Techno"

	expected := []string{"elektro", "tech house", "techno"}
	result := gc.extractGenresFromText(genresText)
	if diff := deep.Equal(expected, result, deep.FLAG_IGNORE_SLICE_ORDER); diff != nil {
		t.Errorf("%v and %v are not equal. diff: %v", result, expected, diff)
	}

}

func TestDuplicateGenre2(t *testing.T) {
	gc := GenreCache{
		allGenres: allGenres,
	}

	genresText := "This band pays jazz fusion"

	expected := []string{"jazz fusion"}
	result := gc.extractGenresFromText(genresText)
	if diff := deep.Equal(expected, result, deep.FLAG_IGNORE_SLICE_ORDER); diff != nil {
		t.Errorf("%v and %v are not equal. diff: %v", result, expected, diff)
	}

}

func TestDuplicateGenre3(t *testing.T) {
	gc := GenreCache{
		allGenres: allGenres,
	}

	genresText := "This band pays jazz"

	expected := []string{"jazz"}
	result := gc.extractGenresFromText(genresText)
	if diff := deep.Equal(expected, result, deep.FLAG_IGNORE_SLICE_ORDER); diff != nil {
		t.Errorf("%v and %v are not equal. diff: %v", result, expected, diff)
	}

}

func TestDuplicateGenre4(t *testing.T) {
	gc := GenreCache{
		allGenres: allGenres,
	}

	genresText := "This band pays jazz and is cool"

	expected := []string{"jazz"}
	result := gc.extractGenresFromText(genresText)
	if diff := deep.Equal(expected, result, deep.FLAG_IGNORE_SLICE_ORDER); diff != nil {
		t.Errorf("%v and %v are not equal. diff: %v", result, expected, diff)
	}

}

func TestDuplicateGenre5(t *testing.T) {
	gc := GenreCache{
		allGenres: allGenres,
	}

	genresText := "2025 # Deep House # Disco # Diva Energy # Elektro # Queer Icon # Resident # Special # Tech House"

	expected := []string{"deep house", "disco", "elektro", "tech house"}
	result := gc.extractGenresFromText(genresText)
	if diff := deep.Equal(expected, result, deep.FLAG_IGNORE_SLICE_ORDER); diff != nil {
		t.Errorf("%v and %v are not equal. diff: %v", result, expected, diff)
	}

}

func TestExtractArtistsFromTitle(t *testing.T) {
	gc := GenreCache{}
	tests := []struct {
		title    string
		expected []string
	}{
		{
			title: "HEAVYSAURUS (ger)",
			expected: []string{
				"heavysaurus",
			},
		},
		{
			title: "Chaos Blast Meating: HOWLS FROM ABOVE, » DOPELORD, » THRONEHAMMER, » HIDAS, » ACID MAMMOTH, » ENDONOMOS, » TONS",
			expected: []string{
				"chaos blast meating",
				"howls from above",
				"dopelord",
				"thronehammer",
				"hidas",
				"acid mammoth",
				"endonomos",
				"tons",
			},
		},
		{
			title: "Shallow Dissent, Thorn, In Oceans Deep, Pollen",
			expected: []string{
				"shallow dissent",
				"thorn",
				"in oceans deep",
				"pollen",
			},
		},
		{
			title: "Amsterdam BeatClub, feat. The Mocks (60’s Beat) & dj’s!",
			expected: []string{
				"amsterdam beatclub",
				"the mocks",
				"dj’s",
			},
		},
		{
			title: "Caravan Palace",
			expected: []string{
				"caravan palace",
			},
		},
		{
			title: "Biesmans [Running Back / Berlin], Chris Mueller [Electronic Monster / München], Danca [Ritter Butzke Records / München], Rainer Wahnsinn [Electronic Monster / München]",
			expected: []string{
				"biesmans",
				"chris mueller",
				"danca",
				"rainer wahnsinn",
			},
		},
	}

	for _, test := range tests {
		result := gc.extractArtistsFromTitle(test.title)
		if diff := deep.Equal(test.expected, result, deep.FLAG_IGNORE_SLICE_ORDER); diff != nil {
			t.Errorf("%v and %v are not equal. diff: %v", result, test.expected, diff)
		}
	}
}
