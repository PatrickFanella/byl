package models

import (
	"encoding/json"
	"math"
	"net/url"
	"strings"
	"time"

	"invisible-museum/backend/internal/geo"
)

// ArrivalAccess records one mode's sourced eligibility, without accessibility certification.
type ArrivalAccess struct {
	Mode        TravelMode `json:"mode"`
	Status      string     `json:"status"`
	Surface     string     `json:"surface"`
	SourceURL   string     `json:"source_url"`
	SourceTitle string     `json:"source_title"`
	RetrievedAt string     `json:"retrieved_at"`
	Notes       string     `json:"notes"`
}

// ArrivalPoint is a proposed public entrance or viewing location, separate from site coordinates.
type ArrivalPoint struct {
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Latitude  *float64        `json:"latitude"`
	Longitude *float64        `json:"longitude"`
	Access    []ArrivalAccess `json:"access"`
	Notes     string          `json:"notes"`
}

// ArrivalProposalRequest compares the current approved arrival version before proposing a change.
type ArrivalProposalRequest struct {
	Version int          `json:"version"`
	Point   ArrivalPoint `json:"point"`
}

// Validate requires independent walking and biking decisions and bounds public source context.
func (r ArrivalProposalRequest) Validate() Fields {
	f := Fields{}
	if r.Version < 0 {
		f["version"] = "must be the current approved arrival version"
	}
	if strings.TrimSpace(r.Point.Name) == "" || len(r.Point.Name) > 300 {
		f["point.name"] = "requires a name of at most 300 bytes"
	}
	if r.Point.Kind != "entrance" && r.Point.Kind != "viewpoint" {
		f["point.kind"] = "must be entrance or viewpoint"
	}
	if r.Point.Latitude == nil || math.IsNaN(*r.Point.Latitude) || math.IsInf(*r.Point.Latitude, 0) || *r.Point.Latitude < -90 || *r.Point.Latitude > 90 {
		f["point.latitude"] = "requires a latitude"
	}
	if r.Point.Longitude == nil || math.IsNaN(*r.Point.Longitude) || math.IsInf(*r.Point.Longitude, 0) || *r.Point.Longitude < -180 || *r.Point.Longitude > 180 {
		f["point.longitude"] = "requires a longitude"
	}
	modes := map[TravelMode]bool{}
	for _, a := range r.Point.Access {
		if !a.Mode.Valid() || modes[a.Mode] {
			f["point.access"] = "provide walking and biking exactly once"
		}
		modes[a.Mode] = true
		switch a.Status {
		case "unknown", "allowed", "restricted", "closed":
		default:
			f["point.access"] = "status must be unknown, allowed, restricted or closed"
		}
		switch a.Surface {
		case "unknown", "paved", "unpaved", "mixed", "steps", "other":
		default:
			f["point.access"] = "surface must be unknown, paved, unpaved, mixed, steps or other"
		}
		if a.Status != "unknown" || a.Surface != "unknown" || a.SourceURL != "" || a.SourceTitle != "" || a.RetrievedAt != "" {
			u, err := url.Parse(a.SourceURL)
			validURL := err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && (u.Port() == "" || (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80"))
			if !validURL || len(a.SourceURL) > 2000 {
				f["point.access"] = "known access or surface requires an HTTP/HTTPS source without credentials"
			}
			if strings.TrimSpace(a.SourceTitle) == "" || len(a.SourceTitle) > 500 {
				f["point.access"] = "requires a source title of at most 500 bytes"
			}
			d, err := time.Parse("2006-01-02", a.RetrievedAt)
			if err != nil || d.After(time.Now().UTC()) {
				f["point.access"] = "requires a retrieval date YYYY-MM-DD that is not in the future"
			}
		}
		if len(a.Notes) > 2000 {
			f["point.access"] = "access notes must be at most 2000 bytes"
		}
	}
	if len(r.Point.Access) != 2 || !modes[TravelModeWalk] || !modes[TravelModeBike] {
		f["point.access"] = "provide walking and biking exactly once; use unknown when unresolved"
	}
	if len(r.Point.Notes) > 4000 {
		f["point.notes"] = "must be at most 4000 bytes"
	}
	if raw, err := json.Marshal(r); err != nil || len(raw) > 20000 {
		f["point"] = "must be at most 20000 encoded bytes"
	}
	return f
}

// ArrivalProposal retains immutable input and the explicit curator review.
type ArrivalProposal struct {
	ID          string       `json:"id"`
	PlaceID     string       `json:"place_id"`
	BaseVersion int          `json:"base_version"`
	Point       ArrivalPoint `json:"point"`
	Status      string       `json:"status"`
	CreatedBy   string       `json:"created_by"`
	ReviewedBy  *string      `json:"reviewed_by"`
	ReviewNote  string       `json:"review_note"`
	CreatedAt   time.Time    `json:"created_at"`
	ReviewedAt  *time.Time   `json:"reviewed_at"`
}

// ArrivalReviewRequest approves or rejects one immutable proposal at its base version.
type ArrivalReviewRequest struct {
	Version int    `json:"version"`
	Action  string `json:"action"`
	Note    string `json:"note"`
}

// Validate requires an explicit review action and rationale.
func (r ArrivalReviewRequest) Validate() Fields {
	f := Fields{}
	if r.Version < 0 {
		f["version"] = "must be the proposal base version"
	}
	if r.Action != "approve" && r.Action != "reject" {
		f["action"] = "must be approve or reject"
	}
	if strings.TrimSpace(r.Note) == "" || len(r.Note) > 4000 {
		f["note"] = "requires a review note of at most 4000 bytes"
	}
	return f
}

// ApprovedArrival is the public reviewed point. Curator IDs and private review notes are excluded.
type ApprovedArrival struct {
	ID         string       `json:"id"`
	Version    int          `json:"version"`
	Point      ArrivalPoint `json:"point"`
	ReviewedAt time.Time    `json:"reviewed_at"`
}

// ArrivalSelection gives navigation callers a point only for the independently allowed mode.
type ArrivalSelection struct {
	PlaceID         string           `json:"place_id"`
	Mode            TravelMode       `json:"mode"`
	Eligibility     string           `json:"eligibility"`
	NavigationPoint *geo.Point       `json:"navigation_point"`
	Arrival         *ApprovedArrival `json:"arrival"`
}

// SelectArrival never substitutes a site centroid, nearby road or another mode's eligibility.
func (p PlaceSummary) SelectArrival(mode TravelMode) ArrivalSelection {
	s := ArrivalSelection{PlaceID: p.ID, Mode: mode, Eligibility: "unknown", Arrival: p.ArrivalPoint}
	if !mode.Valid() || p.ArrivalPoint == nil {
		return s
	}
	for _, a := range p.ArrivalPoint.Point.Access {
		if a.Mode != mode {
			continue
		}
		s.Eligibility = a.Status
		point := p.ArrivalPoint.Point
		if a.Status == "allowed" && point.Latitude != nil && point.Longitude != nil {
			s.NavigationPoint = &geo.Point{Lat: *point.Latitude, Lng: *point.Longitude}
		}
	}
	return s
}
