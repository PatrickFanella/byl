package models

import (
	"strings"
	"testing"
)

func TestHistoryPublicationRequiresEvidenceAndAnIllustration(t *testing.T) {
	body := strings.Repeat("A researched account of the location and its historical context. ", 10)
	body = strings.TrimSpace(body) + "\n\n" + strings.TrimSpace(body) + "\n\n" + strings.TrimSpace(body)
	sources := []Source{{Title: "Local archive", Publisher: "Historical society", URL: "https://archive.example.org/item"}}
	image := MediaAsset{Kind: MediaKindImage, PublicURL: "https://media.example.org/a.jpg", Caption: "The depot in 1910", Credit: "Archive photographer", License: "CC-BY-4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/", SourceURL: "https://archive.example.org/photo"}
	if gaps := HistoryReviewGaps(body, sources, []MediaAsset{image}); len(gaps) != 0 {
		t.Fatalf("complete history rejected: %v", gaps)
	}
	for _, tc := range []struct {
		name    string
		body    string
		sources []Source
		media   []MediaAsset
	}{
		{"short summary", "A station opened.", sources, []MediaAsset{image}},
		{"unreadable block", strings.ReplaceAll(body, "\n\n", " "), sources, []MediaAsset{image}},
		{"no citations", body, nil, []MediaAsset{image}},
		{"picture missing", body, sources, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if gaps := HistoryReviewGaps(tc.body, tc.sources, tc.media); len(gaps) == 0 {
				t.Fatal("incomplete history accepted")
			}
		})
	}
	for _, license := range []string{"", "All rights reserved", "CC-BY-NC-SA-4.0", "CC-BY-ND-4.0"} {
		asset := image
		asset.License = license
		if asset.ReusableImage() {
			t.Fatalf("restricted image accepted: %s", license)
		}
	}
	image.SourceURL = ""
	if image.ReusableImage() {
		t.Fatal("image without provenance accepted")
	}
}
