package models

import (
	"encoding/json"
	"time"
)

// Candidate is an entry in the unified review queue.
type Candidate struct {
	ID                   string          `json:"id"`
	Kind                 CandidateKind   `json:"kind"`
	Origin               CandidateOrigin `json:"origin"`
	SourceName           *string         `json:"source_name"`
	ExternalID           *string         `json:"external_id"`
	SubmittedByAccountID *string         `json:"submitted_by_account_id"`
	TargetPlaceID        *string         `json:"target_place_id"`
	TargetMomentID       *string         `json:"target_moment_id"`
	Payload              json.RawMessage `json:"payload"`
	Status               CandidateStatus `json:"status"`
	ReviewerAccountID    *string         `json:"reviewer_account_id"`
	ReviewNote           string          `json:"review_note"`
	ApprovedEntityID     *string         `json:"approved_entity_id"`
	ReviewedAt           *time.Time      `json:"reviewed_at"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// Job is a background job row.
type Job struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
	Status      JobStatus       `json:"status"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	RunAfter    time.Time       `json:"run_after"`
	LastError   string          `json:"last_error"`
	StartedAt   *time.Time      `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Job kinds handled by the worker.
const (
	JobKindComputeRoute      = "compute_route"
	JobKindGenerateNarration = "generate_narration"
	JobKindImportSource      = "import_source"
	JobKindDraftCandidate    = "draft_candidate"
	JobKindFetchMedia        = "fetch_media"
	JobKindResearchBrief     = "research_brief"
	JobKindFetchEvidence     = "fetch_evidence"
)

// ResearchBriefPayload references durable private checkpoints rather than source text.
type ResearchBriefPayload struct {
	RunID string `json:"run_id"`
}

// ImportSourcePayload asks a source adapter to fill the review queue.
type ImportSourcePayload struct {
	Source  string   `json:"source"`
	CityID  string   `json:"city_id"`
	PlaceID *string  `json:"place_id,omitempty"`
	Lat     *float64 `json:"lat,omitempty"`
	Lng     *float64 `json:"lng,omitempty"`
	RadiusM float64  `json:"radius_m,omitempty"`
	Query   string   `json:"query,omitempty"`
	Limit   int      `json:"limit,omitempty"`
}

// DraftCandidatePayload binds inspected evidence to a captured pending candidate.
type DraftCandidatePayload struct {
	CandidateID   string   `json:"candidate_id"`
	EvidenceIDs   []string `json:"evidence_ids"`
	PayloadSHA256 string   `json:"payload_sha256"`
}

// DraftCandidateRequest selects immutable inspected snapshots, not submitted prose.
type DraftCandidateRequest struct {
	EvidenceIDs []string `json:"evidence_ids"`
}

// FetchMediaPayload downloads an approved media candidate's file into blob storage.
type FetchMediaPayload struct {
	CandidateID string `json:"candidate_id"`
	AssetID     string `json:"asset_id"`
}

// ComputeRoutePayload is the payload of a compute_route job.
type ComputeRoutePayload struct {
	RouteVersion int64      `json:"route_version"`
	TourID       string     `json:"tour_id"`
	Mode         TravelMode `json:"mode"`
}

// GenerateNarrationPayload is the payload of a generate_narration job.
type GenerateNarrationPayload struct {
	MomentID string `json:"moment_id"`
}

// Stats are the admin dashboard counts.
type Stats struct {
	PendingCandidates map[string]int `json:"pending_candidates"`
	Jobs              map[string]int `json:"jobs"`
	Published         struct {
		Places  int `json:"places"`
		Moments int `json:"moments"`
		Tours   int `json:"tours"`
	} `json:"published"`
	UserRoutes int `json:"user_routes"`
	Accounts   int `json:"accounts"`
}

// ImportRequest triggers a source import from the admin API.
type ImportRequest struct {
	Source  string  `json:"source"`
	City    string  `json:"city"`
	Place   string  `json:"place"`
	Near    string  `json:"near"`
	RadiusM float64 `json:"radius_m"`
	Query   string  `json:"query"`
	Limit   int     `json:"limit"`
}
