package shared_test

import (
	"testing"

	"github.com/jakopako/event-api/shared"
)

func TestRemoveDiacritics(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ásgeir", "ásgeir", "asgeir"},
		{"café", "café", "cafe"},
		{"naïve", "naïve", "naive"},
		{"Müller", "Müller", "Muller"},
		{"São Paulo", "São Paulo", "Sao Paulo"},
		{"hello", "hello", "hello"},
		{"résumé", "résumé", "resume"},
		{"Björk", "Björk", "Bjork"},
		{"François", "François", "Francois"},
		{"Åland", "Åland", "Aland"},
		{"empty string", "", ""},
		{"no diacritics", "no diacritics", "no diacritics"},
		{"special chars", "123!@#", "123!@#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shared.RemoveDiacritics(tt.input)
			if result != tt.expected {
				t.Errorf("RemoveDiacritics(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}
