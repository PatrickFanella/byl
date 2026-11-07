package models

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// BriefStatus is the private intake disposition, never a publication status.
type BriefStatus string

const (
	BriefDraft         BriefStatus = "draft"
	BriefProcessing    BriefStatus = "processing"
	BriefNeedsResearch BriefStatus = "needs_research"
	BriefReview        BriefStatus = "review"
	BriefAccepted      BriefStatus = "accepted"
	BriefDeclined      BriefStatus = "declined"
)

// Valid reports whether the state belongs to the intake lifecycle.
func (s BriefStatus) Valid() bool {
	switch s {
	case BriefDraft, BriefProcessing, BriefNeedsResearch, BriefReview, BriefAccepted, BriefDeclined:
		return true
	}
	return false
}

// CanTransitionTo rejects skipping review or silently reopening a disposition.
func (s BriefStatus) CanTransitionTo(next BriefStatus) bool {
	switch s {
	case BriefDraft:
		return next == BriefProcessing || next == BriefNeedsResearch || next == BriefReview || next == BriefDeclined
	case BriefProcessing:
		return next == BriefNeedsResearch || next == BriefReview
	case BriefNeedsResearch:
		return next == BriefProcessing || next == BriefReview || next == BriefDeclined
	case BriefReview:
		return next == BriefAccepted || next == BriefNeedsResearch || next == BriefDeclined
	case BriefAccepted, BriefDeclined:
		return next == BriefDraft
	}
	return false
}

// RouteBriefInput contains unverified leads supplied by the submitter. These
// fields are not source evidence and must never be copied directly into public text.
type RouteBriefInput struct {
	Topic           string       `json:"topic"`
	Area            string       `json:"area"`
	Notes           string       `json:"notes"`
	PlaceLeads      []string     `json:"place_leads"`
	EventLeads      []string     `json:"event_leads"`
	SourceLinks     []string     `json:"source_links"`
	Modes           []TravelMode `json:"modes"`
	TripShape       string       `json:"trip_shape"`
	DurationMinutes *int         `json:"duration_minutes"`
	StartPreference string       `json:"start_preference"`
	EndPreference   string       `json:"end_preference"`
	PilotID         string       `json:"pilot_id"`
}

// Normalize keeps optional arrays non-null and supplies travel defaults.
func (r *RouteBriefInput) Normalize() {
	r.Topic = strings.TrimSpace(r.Topic)
	r.Area = strings.TrimSpace(r.Area)
	if r.PlaceLeads == nil {
		r.PlaceLeads = []string{}
	}
	if r.EventLeads == nil {
		r.EventLeads = []string{}
	}
	if r.SourceLinks == nil {
		r.SourceLinks = []string{}
	}
	if len(r.Modes) == 0 {
		r.Modes = []TravelMode{TravelModeWalk, TravelModeBike}
	}
	if r.TripShape == "" {
		r.TripShape = "flexible"
	}
}

// Validate permits unresolved names and addresses; only topic and area are required.
func (r RouteBriefInput) Validate() Fields {
	f := Fields{}
	required(f, "topic", r.Topic)
	required(f, "area", r.Area)
	for key, item := range map[string]struct {
		value string
		max   int
	}{
		"topic": {r.Topic, 300}, "area": {r.Area, 500}, "notes": {r.Notes, 10000},
		"start_preference": {r.StartPreference, 500}, "end_preference": {r.EndPreference, 500}, "pilot_id": {r.PilotID, 100},
	} {
		if utf8.RuneCountInString(item.value) > item.max {
			f.set(key, fmt.Sprintf("must be at most %d characters", item.max))
		}
	}
	for key, values := range map[string][]string{"place_leads": r.PlaceLeads, "event_leads": r.EventLeads, "source_links": r.SourceLinks} {
		if len(values) > 20 {
			f.set(key, "must contain at most 20 entries")
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 2000 {
				f.set(key, "each entry must contain 1 to 2000 characters")
			}
			if key == "source_links" {
				u, err := url.Parse(value)
				if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
					f.set(key, "must contain HTTP or HTTPS links without credentials")
				}
			}
		}
	}
	validModes(f, r.Modes)
	if len(r.Modes) > 2 || (len(r.Modes) == 2 && r.Modes[0] == r.Modes[1]) {
		f.set("modes", "choose walk, bike, or both once")
	}
	if r.TripShape != "" && r.TripShape != "loop" && r.TripShape != "point_to_point" && r.TripShape != "out_and_back" && r.TripShape != "flexible" {
		f.set("trip_shape", "must be loop, point_to_point, out_and_back, or flexible")
	}
	if r.DurationMinutes != nil && (*r.DurationMinutes < 15 || *r.DurationMinutes > 1440) {
		f.set("duration_minutes", "must be between 15 and 1440")
	}
	return f
}

// RouteBrief is private to its owner and authorized curators.
type RouteBrief struct {
	ID             string          `json:"id"`
	OwnerAccountID string          `json:"owner_account_id"`
	Submitted      RouteBriefInput `json:"submitted"`
	Status         BriefStatus     `json:"status"`
	Reason         string          `json:"reason"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// UpdateRouteBriefRequest replaces the submitted brief at an expected version.
type UpdateRouteBriefRequest struct {
	Version   int             `json:"version"`
	Submitted RouteBriefInput `json:"submitted"`
}

// BriefStateRequest records a curator decision against the reviewed version.
type BriefStateRequest struct {
	Version int         `json:"version"`
	Status  BriefStatus `json:"status"`
	Reason  string      `json:"reason"`
}

// Validate requires an explicit reason for every lifecycle decision.
func (r BriefStateRequest) Validate() Fields {
	f := Fields{}
	if r.Version < 1 {
		f.set("version", "must be a positive version from the latest brief")
	}
	if !r.Status.Valid() {
		f.set("status", "unknown brief state")
	}
	required(f, "reason", r.Reason)
	if utf8.RuneCountInString(r.Reason) > 2000 {
		f.set("reason", "must be at most 2000 characters")
	}
	return f
}
