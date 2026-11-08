package models

import (
	"strings"
	"time"
)

// ContributionConsentRequest is separate from navigation and progress. The UTC
// request day bounds replay after expiry without retaining a trip timestamp.
type ContributionConsentRequest struct {
	Version     int        `json:"version"`
	Accepted    bool       `json:"accepted"`
	TourID      string     `json:"tour_id"`
	RevisionID  string     `json:"revision_id"`
	Mode        TravelMode `json:"mode"`
	RequestedOn *Date      `json:"requested_on"`
}

func ContributionDay(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (r *ContributionConsentRequest) Normalize() {
	r.TourID = strings.ToLower(strings.TrimSpace(r.TourID))
	r.RevisionID = strings.ToLower(strings.TrimSpace(r.RevisionID))
}

func (r ContributionConsentRequest) Validate(now time.Time) Fields {
	f := Fields{}
	if r.Version != 1 || !r.Accepted {
		f["accepted"] = "explicit version 1 contribution consent is required"
	}
	required(f, "tour_id", r.TourID)
	required(f, "revision_id", r.RevisionID)
	if !r.Mode.Valid() {
		f["mode"] = "must be walk or bike"
	}
	today := ContributionDay(now)
	if r.RequestedOn == nil || r.RequestedOn.IsZero() || !r.RequestedOn.After(today.AddDate(0, 0, -30)) || r.RequestedOn.After(today) {
		f["requested_on"] = "use a UTC calendar day within the last 30 days; new consent needs today's or yesterday's day"
	}
	return f
}

// ContributionSummary matches the pure detector's minimal, uncalibrated v1
// output. No account/device ID, coordinate, precise timestamp or trace is accepted.
type ContributionSummary struct {
	SchemaVersion             int        `json:"schema_version"`
	DetectorVersion           string     `json:"detector_version"`
	Calibrated                *bool      `json:"calibrated"`
	RevisionID                string     `json:"revision_id"`
	SegmentID                 string     `json:"segment_id"`
	Mode                      TravelMode `json:"mode"`
	Direction                 string     `json:"direction"`
	ObservationPeriod         *Date      `json:"observation_period"`
	QualifiedPassages         int        `json:"qualified_passages"`
	SustainedDepartureRejoins *int       `json:"sustained_departure_rejoins"`
}

func (r ContributionSummary) Validate(now time.Time) Fields {
	f := Fields{}
	if r.SchemaVersion != 1 || r.DetectorVersion != "uncalibrated-v1" || r.Calibrated == nil || *r.Calibrated {
		f["detector_version"] = "only explicitly uncalibrated schema 1 reference summaries are supported"
	}
	required(f, "revision_id", r.RevisionID)
	if strings.TrimSpace(r.SegmentID) == "" || len(r.SegmentID) > 120 {
		f["segment_id"] = "must identify a retained logical segment, at most 120 bytes"
	}
	if !r.Mode.Valid() {
		f["mode"] = "must be walk or bike"
	}
	if r.Direction != "forward" && r.Direction != "reverse" {
		f["direction"] = "must be forward or reverse"
	}
	if r.QualifiedPassages != 1 || r.SustainedDepartureRejoins == nil || *r.SustainedDepartureRejoins < 0 || *r.SustainedDepartureRejoins > 1 {
		f["qualified_passages"] = "send one qualified-passage summary with zero or one sustained departure/rejoin"
	}
	today := ContributionDay(now)
	if r.ObservationPeriod == nil || r.ObservationPeriod.IsZero() || r.ObservationPeriod.After(today) || !r.ObservationPeriod.After(today.AddDate(0, 0, -30)) {
		f["observation_period"] = "use a UTC calendar day within the last 30 days"
	}
	return f
}

type ContributionRequest struct {
	ConsentID string              `json:"consent_id"`
	Summary   ContributionSummary `json:"summary"`
}

type ContributionConsent struct {
	ID             string `json:"id"`
	DeletionHandle string `json:"deletion_handle"`
	ExpiresOn      Date   `json:"expires_on"`
}

type ContributionReceipt struct {
	ID        string `json:"id"`
	ExpiresOn Date   `json:"expires_on"`
}

type ContributionWithdrawal struct {
	ConsentID      string `json:"consent_id"`
	DeletionHandle string `json:"deletion_handle"`
}

func ValidContributionKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
