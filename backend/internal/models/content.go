package models

import (
	"encoding/json"
	"strings"
	"time"

	"invisible-museum/backend/internal/geo"
)

// Date is a calendar date serialised as YYYY-MM-DD.
type Date struct{ time.Time }

const dateLayout = "2006-01-02"

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Format(dateLayout)) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(dateLayout, strings.TrimSpace(s))
	if err != nil {
		return err
	}
	d.Time = t
	return nil
}

// ParseDate parses YYYY-MM-DD; empty input yields nil.
func ParseDate(s string) (*Date, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil, err
	}
	return &Date{t}, nil
}

// DateFromTime wraps a nullable time as a Date.
func DateFromTime(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	return &Date{*t}
}

// City is a museum city.
type City struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Region      string    `json:"region"`
	CountryCode string    `json:"country_code"`
	CenterLat   float64   `json:"center_lat"`
	CenterLng   float64   `json:"center_lng"`
	BBox        geo.BBox  `json:"bbox"`
	Timezone    string    `json:"timezone"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CityDetail is a city with its topics.
type CityDetail struct {
	City
	Topics []Topic `json:"topics"`
}

// Topic is a museum wing.
type Topic struct {
	ID          string `json:"id"`
	CityID      string `json:"city_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MomentCount int    `json:"moment_count"`
}

// MediaAsset is an image, video, or audio file with its licence.
type MediaAsset struct {
	ID              string       `json:"id"`
	Kind            MediaKind    `json:"kind"`
	StorageKey      string       `json:"storage_key,omitempty"`
	PublicURL       string       `json:"public_url"`
	MimeType        string       `json:"mime_type"`
	Width           *int         `json:"width"`
	Height          *int         `json:"height"`
	DurationSeconds *float64     `json:"duration_seconds"`
	Caption         string       `json:"caption"`
	Credit          string       `json:"credit"`
	License         string       `json:"license"`
	LicenseURL      string       `json:"license_url"`
	SourceURL       string       `json:"source_url"`
	Status          EntityStatus `json:"status"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// PlaceSummary is a place without its moments expanded.
type PlaceSummary struct {
	ID              string       `json:"id"`
	CityID          string       `json:"city_id"`
	Slug            string       `json:"slug"`
	Name            string       `json:"name"`
	Summary         string       `json:"summary"`
	Kind            string       `json:"kind"`
	Latitude        float64      `json:"latitude"`
	Longitude       float64      `json:"longitude"`
	ArrivalRadiusM  float64      `json:"arrival_radius_m"`
	Address         string       `json:"address"`
	Status          EntityStatus `json:"status"`
	PrimaryMomentID *string      `json:"primary_moment_id"`
	MomentCount     int          `json:"moment_count"`
	DistanceM       *float64     `json:"distance_m,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// PlaceDetail is a place with its moments.
type PlaceDetail struct {
	PlaceSummary
	PrimaryMoment *MomentSummary  `json:"primary_moment"`
	Moments       []MomentSummary `json:"moments"`
}

// Source is a citation attached to a moment.
type Source struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Publisher   string `json:"publisher"`
	RetrievedAt *Date  `json:"retrieved_at"`
	License     string `json:"license"`
	Note        string `json:"note"`
}

// MomentSummary is a moment without body, media list, or sources.
type MomentSummary struct {
	ID            string        `json:"id"`
	PlaceID       string        `json:"place_id"`
	Slug          string        `json:"slug"`
	Title         string        `json:"title"`
	Excerpt       string        `json:"excerpt"`
	DateStart     *Date         `json:"date_start"`
	DateEnd       *Date         `json:"date_end"`
	DatePrecision DatePrecision `json:"date_precision"`
	DateDisplay   string        `json:"date_display"`
	HeroMedia     *MediaAsset   `json:"hero_media"`
	HasNarration  bool          `json:"has_narration"`
	Status        EntityStatus  `json:"status"`
	TopicIDs      []string      `json:"topic_ids"`
}

// MomentDetail is the full moment.
type MomentDetail struct {
	MomentSummary
	Body      string       `json:"body"`
	Media     []MediaAsset `json:"media"`
	Sources   []Source     `json:"sources"`
	Topics    []Topic      `json:"topics"`
	Narration *MediaAsset  `json:"narration"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Excerpt returns the first paragraph of body, capped at 240 characters.
func Excerpt(body string) string {
	first := body
	if i := strings.Index(body, "\n"); i >= 0 {
		first = body[:i]
	}
	first = strings.TrimSpace(first)
	if len(first) <= 240 {
		return first
	}
	cut := first[:240]
	if i := strings.LastIndex(cut, " "); i > 120 {
		cut = cut[:i]
	}
	return cut + "…"
}
