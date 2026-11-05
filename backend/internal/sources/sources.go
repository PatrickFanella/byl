// Package sources discovers candidate places, moments, and media from open archives
// and configured local pages. Every adapter implements Adapter; nothing here writes
// to the database or publishes anything.
package sources

import (
	"context"
	"encoding/json"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

// Request describes one discovery run.
type Request struct {
	CityID  string
	Center  geo.Point
	RadiusM float64 // default 2000, max 20000
	Query   string  // free text for text-search adapters
	Limit   int     // default 25, max 100
}

func (r Request) radius() float64 {
	switch {
	case r.RadiusM <= 0:
		return 2000
	case r.RadiusM > 20000:
		return 20000
	}
	return r.RadiusM
}

func (r Request) limit() int {
	switch {
	case r.Limit <= 0:
		return 25
	case r.Limit > 100:
		return 100
	}
	return r.Limit
}

// Found is one discovered item, ready to become a review-queue candidate.
type Found struct {
	Kind       models.CandidateKind
	ExternalID string
	Title      string
	Text       string
	URL        string
	Lat, Lng   *float64
	License    string
	LicenseURL string
	Credit     string
	MediaURL   string
	MimeType   string
	Width      int
	Height     int
	Raw        json.RawMessage
}

// Adapter discovers items near a point or for a query.
type Adapter interface {
	Name() string
	Discover(ctx context.Context, req Request) ([]Found, error)
}

// Info describes an adapter to the admin UI.
type Info struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	SupportsQuery bool   `json:"supports_query"`
	SupportsNear  bool   `json:"supports_near"`
}

func f64(v float64) *float64 { return &v }
