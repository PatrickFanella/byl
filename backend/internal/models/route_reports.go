package models

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// ReportLocation identifies only the affected part of the frozen itinerary.
type ReportLocation struct {
	Kind       string `json:"kind"`
	StopID     string `json:"stop_id,omitempty"`
	ArrivalID  string `json:"arrival_id,omitempty"`
	FromStopID string `json:"from_stop_id,omitempty"`
	ToStopID   string `json:"to_stop_id,omitempty"`
}

// ReportAlternative retains the submitted dimensions so validation can reject
// malformed coordinates rather than silently truncating extra ordinates.
type ReportAlternative struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

// ReportPoint requires both coordinates, including an explicitly supplied zero.
type ReportPoint struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

// RouteReportInput is a private observation, not reviewed history or route intent.
type RouteReportInput struct {
	TourID      string             `json:"tour_id"`
	RevisionID  string             `json:"revision_id"`
	Mode        TravelMode         `json:"mode"`
	Direction   string             `json:"direction"`
	Affected    ReportLocation     `json:"affected"`
	Reason      string             `json:"reason"`
	Details     string             `json:"details"`
	ObservedOn  *Date              `json:"observed_on"`
	Duration    string             `json:"duration"`
	Better      string             `json:"better"`
	SourceURL   string             `json:"source_url"`
	Annotation  *ReportPoint       `json:"annotation"`
	Alternative *ReportAlternative `json:"alternative"`
}

// Normalize makes unknown direction/duration explicit and retry hashes stable.
func (r *RouteReportInput) Normalize() {
	r.TourID = strings.ToLower(strings.TrimSpace(r.TourID))
	r.RevisionID = strings.ToLower(strings.TrimSpace(r.RevisionID))
	r.Affected.StopID = strings.ToLower(strings.TrimSpace(r.Affected.StopID))
	r.Affected.ArrivalID = strings.ToLower(strings.TrimSpace(r.Affected.ArrivalID))
	r.Affected.FromStopID = strings.ToLower(strings.TrimSpace(r.Affected.FromStopID))
	r.Affected.ToStopID = strings.ToLower(strings.TrimSpace(r.Affected.ToStopID))
	r.Details = strings.TrimSpace(r.Details)
	r.Better = strings.TrimSpace(r.Better)
	r.SourceURL = strings.TrimSpace(r.SourceURL)
	if r.Direction == "" {
		r.Direction = "unknown"
	}
	if r.Duration == "" {
		r.Duration = "unknown"
	}
}

// Validate bounds submitted content; the repository checks frozen references.
func (r RouteReportInput) Validate() Fields {
	f := Fields{}
	required(f, "tour_id", r.TourID)
	required(f, "revision_id", r.RevisionID)
	if !r.Mode.Valid() {
		f["mode"] = "must be walk or bike"
	}
	if r.Direction != "forward" && r.Direction != "reverse" && r.Direction != "unknown" {
		f["direction"] = "must be forward, reverse or unknown"
	}
	if r.Duration != "permanent" && r.Duration != "temporary" && r.Duration != "unknown" {
		f["duration"] = "must be permanent, temporary or unknown"
	}
	switch r.Reason {
	case "blocked", "closed", "unsafe", "access", "surface", "improvement", "other":
	default:
		f["reason"] = "must be blocked, closed, unsafe, access, surface, improvement or other"
	}
	if r.Reason == "improvement" {
		required(f, "better", r.Better)
	}
	if r.Reason == "other" {
		required(f, "details", r.Details)
	}
	if utf8.RuneCountInString(r.Details) > 4000 || utf8.RuneCountInString(r.Better) > 1000 {
		f["details"] = "details allow 4000 characters and better allows 1000"
	}
	if r.SourceURL != "" && (!webURL(r.SourceURL) || len(r.SourceURL) > 2000) {
		f["source_url"] = "must be an HTTP(S) URL without credentials, at most 2000 bytes"
	}
	if r.ObservedOn != nil && r.ObservedOn.IsZero() {
		f["observed_on"] = "must be a calendar date when known"
	}
	a := r.Affected
	switch a.Kind {
	case "stop", "entrance":
		if a.StopID == "" || a.FromStopID != "" || a.ToStopID != "" || (a.Kind == "stop" && a.ArrivalID != "") {
			f["affected"] = "a stop/entrance needs stop_id only, with optional arrival_id for an entrance"
		}
	case "segment":
		if a.FromStopID == "" || a.ToStopID == "" || a.FromStopID == a.ToStopID || a.StopID != "" || a.ArrivalID != "" {
			f["affected"] = "a segment needs distinct from_stop_id and to_stop_id only"
		}
	default:
		f["affected"] = "kind must be stop, entrance or segment"
	}
	if r.Annotation != nil && (r.Annotation.Lat == nil || r.Annotation.Lng == nil || !validReportPoint(*r.Annotation.Lat, *r.Annotation.Lng)) {
		f["annotation"] = "must be a finite WGS84 point"
	}
	if r.Alternative != nil {
		if r.Alternative.Type != "LineString" || len(r.Alternative.Coordinates) < 2 || len(r.Alternative.Coordinates) > 64 {
			f["alternative"] = "must be a LineString with 2 to 64 points"
		}
		for _, p := range r.Alternative.Coordinates {
			if len(p) != 2 || !validReportPoint(p[1], p[0]) {
				f["alternative"] = "must contain finite WGS84 longitude/latitude pairs"
			}
		}
	}
	return f
}

func validReportPoint(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lng) && !math.IsInf(lat, 0) && !math.IsInf(lng, 0) && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

// RouteReport reuses the private proposal disposition lifecycle.
type RouteReport struct {
	ID             string           `json:"id"`
	OwnerAccountID string           `json:"owner_account_id"`
	Submitted      RouteReportInput `json:"submitted"`
	Status         BriefStatus      `json:"status"`
	Reason         string           `json:"reason"`
	Version        int              `json:"version"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}
