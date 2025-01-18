package genre

import (
	"testing"

	"github.com/go-test/deep"
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
