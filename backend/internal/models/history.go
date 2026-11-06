package models

import (
	"net/url"
	"strings"
)

const MinHistoryWords = 250

func webURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// ReusableLicense accepts the licenses reviewed for illustrated place histories.
func ReusableLicense(value string) bool {
	license := value == "PD" || value == "CC0-1.0"
	for _, version := range []string{"1.0", "2.0", "2.5", "3.0", "4.0"} {
		license = license || value == "CC-BY-"+version || value == "CC-BY-SA-"+version
	}
	return license
}

func (m MediaAsset) ReusableImage() bool {
	return m.Kind == MediaKindImage && m.PublicURL != "" && ReusableLicense(m.License) &&
		strings.TrimSpace(m.Caption) != "" && strings.TrimSpace(m.Credit) != "" && webURL(m.SourceURL) && webURL(m.LicenseURL)
}

// HistoryReviewGaps describes publication requirements. Drafts may remain incomplete.
func HistoryReviewGaps(body string, sources []Source, media []MediaAsset) []string {
	gaps := []string{}
	if len(strings.Fields(body)) < MinHistoryWords {
		gaps = append(gaps, "history needs at least 250 words of sourced context; research further rather than padding")
	}
	paragraphs := 0
	for _, paragraph := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(paragraph) != "" {
			paragraphs++
		}
	}
	if paragraphs < 3 {
		gaps = append(gaps, "history needs at least three readable paragraphs")
	}
	hasSource := false
	for _, source := range sources {
		if strings.TrimSpace(source.Title) != "" && strings.TrimSpace(source.Publisher) != "" && webURL(source.URL) {
			hasSource = true
		}
	}
	if !hasSource {
		gaps = append(gaps, "history needs a citation with title, publisher, and source URL")
	}
	hasImage := false
	for _, asset := range media {
		hasImage = hasImage || asset.ReusableImage()
	}
	if !hasImage {
		gaps = append(gaps, "attach a picture with a caption, credit, reusable license, license URL, and original source URL")
	}
	return gaps
}
