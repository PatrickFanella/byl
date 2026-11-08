package models

import (
	"strings"
	"time"
)

// TransitIntent selects one rail connection between adjacent story stops.
// Walking times are explicit curator assumptions; station coordinates are not
// approved entrance geometry or an internal walking route.
type TransitIntent struct {
	Version               int        `json:"version"`
	Provider              string     `json:"provider"`
	Mode                  TravelMode `json:"mode"`
	FromStopID            string     `json:"from_stop_id"`
	ToStopID              string     `json:"to_stop_id"`
	BoardStopID           string     `json:"board_stop_id"`
	AlightStopID          string     `json:"alight_stop_id"`
	WalkToStopSeconds     int        `json:"walk_to_stop_seconds"`
	WalkFromStopSeconds   int        `json:"walk_from_stop_seconds"`
	BoardingBufferSeconds int        `json:"boarding_buffer_seconds"`
	WalkingAssumptions    string     `json:"walking_assumptions"`
}

func (i TransitIntent) Validate() Fields {
	f := Fields{}
	if i.Version != 1 || i.Provider != "cta_gtfs_blue_v1" || i.Mode != TravelModeWalk {
		f["provider"] = "only version 1 CTA Blue Line walking connections are supported"
	}
	if i.FromStopID == "" || i.ToStopID == "" || i.FromStopID == i.ToStopID {
		f["from_stop_id"] = "select two adjacent story stops in forward order"
	}
	for _, s := range []string{i.BoardStopID, i.AlightStopID} {
		if len(s) < 1 || len(s) > 20 || strings.Trim(s, "0123456789") != "" {
			f["board_stop_id"] = "select explicit numeric CTA platform identifiers"
		}
	}
	if i.BoardStopID == i.AlightStopID {
		f["alight_stop_id"] = "boarding and alighting platforms must differ"
	}
	if i.WalkToStopSeconds < 60 || i.WalkToStopSeconds > 3600 || i.WalkFromStopSeconds < 60 || i.WalkFromStopSeconds > 3600 || i.BoardingBufferSeconds < 60 || i.BoardingBufferSeconds > 1800 {
		f["walking_assumptions"] = "provide bounded walking estimates (60–3600 seconds) and a boarding buffer (60–1800 seconds)"
	}
	if strings.TrimSpace(i.WalkingAssumptions) == "" || len(i.WalkingAssumptions) > 2000 {
		f["walking_assumptions"] = "explain the curator walking/time assumptions in at most 2000 bytes"
	}
	return f
}

func (i TransitIntent) MatchesStops(t *TourDetail) bool {
	modeAllowed := false
	for _, mode := range t.Modes {
		modeAllowed = modeAllowed || mode == i.Mode
	}
	if !modeAllowed {
		return false
	}
	for n := 0; n+1 < len(t.Stops); n++ {
		if t.Stops[n].ID == i.FromStopID && t.Stops[n+1].ID == i.ToStopID && t.Stops[n+1].Position == t.Stops[n].Position+1 {
			return true
		}
	}
	return false
}

type TransitPlatform struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ParentID  string  `json:"parent_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type TransitSource struct {
	URL         string    `json:"url"`
	Publisher   string    `json:"publisher"`
	SHA256      string    `json:"sha256"`
	RetrievedAt time.Time `json:"retrieved_at"`
	LicenseURL  string    `json:"license_url"`
	Timezone    string    `json:"timezone"`
}

// ApprovedTransit excludes private reviewer identity and rationale. Only the
// revision snapshot supplies ordinary walking legs; the replaced leg is absent.
type ApprovedTransit struct {
	Version     int64           `json:"version"`
	Intent      TransitIntent   `json:"intent"`
	Board       TransitPlatform `json:"board"`
	Alight      TransitPlatform `json:"alight"`
	Source      TransitSource   `json:"source"`
	ReviewedAt  time.Time       `json:"reviewed_at"`
	WalkingLegs []RouteLeg      `json:"walking_legs,omitempty"`
}

type TransitSelectionRequest struct {
	DraftVersion int64          `json:"draft_version"`
	Intent       *TransitIntent `json:"intent"`
	Remove       bool           `json:"remove"`
	ReviewNote   string         `json:"review_note"`
}
