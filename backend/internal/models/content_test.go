package models

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExcerptKeepsUnicodeCharactersIntact(t *testing.T) {
	got := Excerpt(strings.Repeat("史", 300))
	if !utf8.ValidString(got) || got != strings.Repeat("史", 240)+"…" {
		t.Fatalf("excerpt corrupted or miscounted Unicode characters: %q", got)
	}
}
