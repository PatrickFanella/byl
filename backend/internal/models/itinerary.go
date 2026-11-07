package models

import "time"

// ItineraryMaterial is a private captured research input, not a public stop.
// Narrative is an exact retained historical paragraph/claim; no model facts are added.
type ItineraryMaterial struct {
	RecordID      string   `json:"record_id"`
	ReviewVersion int      `json:"review_version"`
	PlaceID       string   `json:"place_id"`
	PlaceName     string   `json:"place_name"`
	CityID        string   `json:"city_id"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	MomentID      string   `json:"moment_id,omitempty"`
	CandidateID   string   `json:"candidate_id,omitempty"`
	MomentTitle   string   `json:"moment_title"`
	Narrative     string   `json:"narrative"`
	EvidenceKind  string   `json:"evidence_kind"`
	ContentSHA256 string   `json:"content_sha256"`
	SourceURLs    []string `json:"source_urls"`
	EvidenceIDs   []string `json:"evidence_ids"`
}

type ItineraryGap struct {
	Code              string  `json:"code"`
	RecordID          string  `json:"record_id,omitempty"`
	PlaceCandidateID  *string `json:"place_candidate_id,omitempty"`
	MomentCandidateID *string `json:"moment_candidate_id,omitempty"`
	Detail            string  `json:"detail"`
}

type ItineraryMaterials struct {
	CapturedAt time.Time           `json:"captured_at"`
	Version    int                 `json:"version"`
	Items      []ItineraryMaterial `json:"items"`
	Gaps       []ItineraryGap      `json:"gaps"`
}

type ItineraryStop struct {
	Material                ItineraryMaterial `json:"material"`
	ThemeReason             string            `json:"theme_reason"`
	WhatToNotice            string            `json:"what_to_notice"`
	Transition              string            `json:"transition"`
	PlannedDwellMinutes     int               `json:"planned_dwell_minutes"`
	PlannedNarrationMinutes int               `json:"planned_narration_minutes"`
}

type ItineraryExclusion struct {
	Material ItineraryMaterial `json:"material"`
	Reason   string            `json:"reason"`
}

// ItineraryProposal records story order separately from non-narrated travel.
// TravelSequence contains zero-based stop indices, including closing/retraced travel.
// It grants no publication, access or computed-distance authority.
type ItineraryProposal struct {
	Version                 int                  `json:"version"`
	Status                  string               `json:"status"`
	CreatedAt               time.Time            `json:"created_at"`
	Title                   string               `json:"title"`
	Introduction            string               `json:"introduction"`
	RequestedShape          string               `json:"requested_shape"`
	TripShape               string               `json:"trip_shape"`
	Modes                   []TravelMode         `json:"modes"`
	TargetDurationMinutes   *int                 `json:"target_duration_minutes"`
	TravelMinutes           *int                 `json:"travel_minutes"`
	TravelState             string               `json:"travel_state"`
	PlannedDwellMinutes     int                  `json:"planned_dwell_minutes"`
	PlannedNarrationMinutes int                  `json:"planned_narration_minutes"`
	Stops                   []ItineraryStop      `json:"stops"`
	TravelSequence          []int                `json:"travel_sequence"`
	Excluded                []ItineraryExclusion `json:"excluded"`
	ResearchGaps            []ItineraryGap       `json:"research_gaps"`
	Assumptions             []string             `json:"assumptions"`
}

type BriefItinerary struct {
	RunID         string             `json:"run_id"`
	SourceVersion int                `json:"source_version"`
	BriefVersion  int                `json:"brief_version"`
	Proposal      *ItineraryProposal `json:"proposal"`
}
