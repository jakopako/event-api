package geo

import "testing"

func TestIsValidLocalityAddressType(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  bool
	}{
		{"city", true}, {"town", true}, {"village", true}, {"county", true},
		{"", false}, {"City", false}, {"state", false}, {"amenity", false},
	} {
		t.Run(tt.value, func(t *testing.T) {
			if got := isValidLocalityAddressType(tt.value); got != tt.want {
				t.Errorf("isValidLocalityAddressType(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestIsValidAmenityType(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  bool
	}{
		{"arts_centre", true}, {"bar", true}, {"building", true}, {"cafe", true},
		{"community_centre", true}, {"concert_hall", true}, {"events_centre", true},
		{"events_venue", true}, {"mobility_hub", true}, {"museum", true},
		{"music_school", true}, {"music_venue", true}, {"nightclub", true},
		{"place_of_worship", true}, {"pub", true}, {"restaurant", true},
		{"social_centre", true}, {"theatre", true}, {"university", true},
		{"", false}, {"Music_Venue", false}, {"city", false}, {"hospital", false},
	} {
		t.Run(tt.value, func(t *testing.T) {
			if got := isValidAmenityType(tt.value); got != tt.want {
				t.Errorf("isValidAmenityType(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestLookupVenueLocationRequiresLocationAndCity(t *testing.T) {
	for _, tt := range []struct{ location, city string }{
		{"", ""}, {"venue", ""}, {"", "city"},
	} {
		if address, err := LookupVenueLocation(tt.location, tt.city, "", ""); err == nil || address != nil {
			t.Errorf("LookupVenueLocation(%q, %q) = (%v, %v), want an error", tt.location, tt.city, address, err)
		}
	}
}
