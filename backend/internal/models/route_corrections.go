package models

import (
	"strings"
	"time"
	"unicode/utf8"
)

// RouteCorrectionComparison is a private read set for one prepared release.
// The investigation remains an unverified lead; Prepared is separately reviewed.
type RouteCorrectionComparison struct {
	Version                  string                            `json:"version"`
	Report                   *RouteReport                      `json:"report"`
	Investigation            *RouteInvestigation               `json:"investigation"`
	Before                   *TourRevision                     `json:"before"`
	Prepared                 *TourSnapshot                     `json:"prepared"`
	PathReports              map[TravelMode]*PathQualityReport `json:"path_reports"`
	PotentiallyAffectedTours []string                          `json:"potentially_affected_tours"`
	Blockers                 []string                          `json:"blockers"`
	ReadyToPublish           bool                              `json:"ready_to_publish"`
}

// ApproveRouteCorrectionRequest approves only the inspected comparison.
type ApproveRouteCorrectionRequest struct {
	Version         string `json:"version"`
	InvestigationID string `json:"investigation_id"`
	Note            string `json:"note"`
	Temporary       bool   `json:"temporary"`
	ReviewOn        *Date  `json:"review_on"`
}

// Validate bounds the decision and requires a future reassessment for detours.
func (r ApproveRouteCorrectionRequest) Validate(now time.Time) Fields {
	f := Fields{}
	if len(r.Version) != 64 {
		f["version"] = "must identify the current comparison"
	}
	if strings.TrimSpace(r.InvestigationID) == "" {
		f["investigation_id"] = "is required"
	}
	if strings.TrimSpace(r.Note) == "" || utf8.RuneCountInString(r.Note) > 2000 {
		f["note"] = "provide a decision reason of at most 2000 characters"
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if r.Temporary {
		if r.ReviewOn == nil || r.ReviewOn.IsZero() || (!now.IsZero() && (!r.ReviewOn.After(today) || r.ReviewOn.After(today.AddDate(1, 0, 0)))) {
			f["review_on"] = "temporary corrections need a future review date within one year"
		}
	} else if r.ReviewOn != nil {
		f["review_on"] = "supply a date only for a temporary correction"
	}
	return f
}

// RouteCorrection is private retained approval provenance. ReviewDue requests
// reassessment; it never removes approved intent or restores a previous route.
type RouteCorrection struct {
	ID                       string    `json:"id"`
	ReportID                 string    `json:"report_id"`
	InvestigationID          string    `json:"investigation_id"`
	TourID                   string    `json:"tour_id"`
	PreviousRevisionID       string    `json:"previous_revision_id"`
	RevisionID               string    `json:"revision_id"`
	ComparisonVersion        string    `json:"comparison_version"`
	CorridorID               string    `json:"corridor_id"`
	ReviewedBy               string    `json:"reviewed_by"`
	Note                     string    `json:"note"`
	Temporary                bool      `json:"temporary"`
	ReviewOn                 *Date     `json:"review_on"`
	ReviewDue                bool      `json:"reassessment_required"`
	PotentiallyAffectedTours []string  `json:"potentially_affected_tours"`
	CreatedAt                time.Time `json:"created_at"`
}

// RouteCorrectionOutcome exposes only the reporter's public release outcome.
type RouteCorrectionOutcome struct {
	RevisionID         string `json:"revision_id"`
	PreviousRevisionID string `json:"previous_revision_id"`
	Temporary          bool   `json:"temporary"`
	ReviewOn           *Date  `json:"review_on"`
	ReviewDue          bool   `json:"reassessment_required"`
}
