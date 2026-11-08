package models

import "encoding/json"

// PublishPackageRequest approves exactly one previously inspected package.
type PublishPackageRequest struct {
	Version string `json:"version"`
	RunID   string `json:"run_id"`
	TourID  string `json:"tour_id"`
	Note    string `json:"note"`
}

// PackageApproval is private provenance stored in the immutable revision JSON.
// It is deliberately absent from TourSnapshot and all public revision responses.
type PackageApproval struct {
	BriefID    string `json:"brief_id"`
	RunID      string `json:"run_id"`
	Version    string `json:"version"`
	ReviewedBy string `json:"reviewed_by"`
	Note       string `json:"note"`
}

// CuratorReviewPackage is private, including for a brief's ordinary owner.
// Version identifies the full read set; no publication authority is implied.
type CuratorReviewPackage struct {
	SchemaVersion  int                               `json:"schema_version"`
	Version        string                            `json:"version"`
	State          string                            `json:"state"`
	ReadyToPublish bool                              `json:"ready_to_publish"`
	Brief          *RouteBrief                       `json:"brief"`
	Research       *ResearchRun                      `json:"research"`
	Submitted      *RouteBriefInput                  `json:"submitted"`
	Proposal       *ItineraryProposal                `json:"proposal"`
	Tour           *TourDetail                       `json:"tour"`
	Routes         map[TravelMode]*TourRoute         `json:"routes"`
	PathReports    map[TravelMode]*PathQualityReport `json:"path_reports"`
	Reconciliation []ReconciliationRecord            `json:"reconciliation"`
	Entities       []ReviewPackageEntity             `json:"entities"`
	Evidence       []SourceEvidence                  `json:"evidence"`
	Tasks          []ReviewPackageTask               `json:"tasks"`
	VisitorStories []ReviewVisitorStory              `json:"visitor_stories"`
}

// Required distinguishes selected dependencies from contextual research leads.
type ReviewPackageEntity struct {
	Key            string                `json:"key"`
	Kind           string                `json:"kind"`
	ID             string                `json:"id"`
	Required       bool                  `json:"required"`
	Disposition    string                `json:"disposition"`
	Current        json.RawMessage       `json:"current"`
	Candidate      *Candidate            `json:"candidate"`
	Proposed       json.RawMessage       `json:"proposed"`
	ProposalSource string                `json:"proposal_source"`
	Changes        []ReviewPackageChange `json:"changes"`
}

type ReviewPackageChange struct {
	Field  string          `json:"field"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

type ReviewPackageTask struct {
	Code      string `json:"code"`
	EntityKey string `json:"entity_key"`
	Detail    string `json:"detail"`
}

// Visitor draft order is explicit; captured_partial text is never release-ready.
type ReviewVisitorStory struct {
	Position    int      `json:"position"`
	PlaceID     string   `json:"place_id"`
	MomentID    string   `json:"moment_id"`
	CandidateID string   `json:"candidate_id"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	TextSource  string   `json:"text_source"`
	SourceURLs  []string `json:"source_urls"`
	EvidenceIDs []string `json:"evidence_ids"`
}
