package models

import "time"

// IdentityFacts are untrusted structured leads, never publication authority.
type IdentityFacts struct {
	CityID              string        `json:"city_id,omitempty"`
	PlaceName           string        `json:"place_name,omitempty"`
	Aliases             []string      `json:"aliases,omitempty"`
	Address             string        `json:"address,omitempty"`
	HistoricalAddresses []string      `json:"historical_addresses,omitempty"`
	Moved               bool          `json:"moved,omitempty"`
	MomentTitle         string        `json:"moment_title,omitempty"`
	DateStart           string        `json:"date_start,omitempty"`
	DatePrecision       DatePrecision `json:"date_precision,omitempty"`
}

// IdentityMoment contains matching metadata, not narrative to be replaced.
type IdentityMoment struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	DateStart     string        `json:"date_start"`
	DatePrecision DatePrecision `json:"date_precision"`
	SourceURLs    []string      `json:"source_urls"`
}

// IdentityPlace is the bounded local matching catalogue.
type IdentityPlace struct {
	ID, CityID, Name, Address string
	Lat, Lng                  float64
	Status                    EntityStatus
	Moments                   []IdentityMoment
}

// IdentityClaim is a curator-confirmed alias/address claim, retained with its decision.
type IdentityClaim struct {
	PlaceID             string
	Aliases             []string
	HistoricalAddresses []string
}

// ReconciliationProposal describes an inferred match. Persistence rechecks decisions and references.
type ReconciliationProposal struct {
	Item     DiscoveryItem `json:"item"`
	Key      string        `json:"key"`
	PlaceKey string        `json:"place_key"`
	CityID   string        `json:"city_id"`
	PlaceID  string        `json:"place_id"`
	MomentID string        `json:"moment_id"`
	Outcome  string        `json:"outcome"`
	Issues   []string      `json:"issues"`
}

// ReconciliationRecord is a private review result shared by overlapping briefs.
type ReconciliationRecord struct {
	ID                string                 `json:"id"`
	Key               string                 `json:"identity_key"`
	Proposal          ReconciliationProposal `json:"proposal"`
	Assessment        ReconciliationProposal `json:"assessment"`
	PlaceCandidateID  *string                `json:"place_candidate_id"`
	MomentCandidateID *string                `json:"moment_candidate_id"`
	ReviewVersion     int                    `json:"review_version"`
	Decision          *IdentityDecision      `json:"decision"`
	CreatedAt         time.Time              `json:"created_at"`
}

// IdentityDecision records an explicit, versioned curator choice without merging public rows.
type IdentityDecision struct {
	Version             int      `json:"version"`
	Outcome             string   `json:"outcome"`
	PlaceID             string   `json:"place_id"`
	MomentID            string   `json:"moment_id"`
	Aliases             []string `json:"aliases"`
	HistoricalAddresses []string `json:"historical_addresses"`
	Note                string   `json:"note"`
	ReviewerID          string   `json:"reviewer_id"`
}

// ReconciliationRequest lets a curator supplement a frozen discovery lead with sourced identity facts.
type ReconciliationRequest struct {
	Version int           `json:"version"`
	Item    DiscoveryItem `json:"item"`
}

// IdentityDecisionRequest compares the last review version before recording a new decision.
type IdentityDecisionRequest struct {
	Version             int      `json:"version"`
	Outcome             string   `json:"outcome"`
	PlaceID             string   `json:"place_id"`
	MomentID            string   `json:"moment_id"`
	Aliases             []string `json:"aliases"`
	HistoricalAddresses []string `json:"historical_addresses"`
	Note                string   `json:"note"`
}

// ReconciliationReport is checkpointed privately; no source payload enters owner progress.
type ReconciliationReport struct {
	Records []ReconciliationRecord `json:"records"`
}

// ReconciliationSource preserves each brief's original lead and inferred outcome.
type ReconciliationSource struct {
	BriefID  string                 `json:"brief_id"`
	RunID    *string                `json:"run_id"`
	Proposal ReconciliationProposal `json:"proposal"`
}
