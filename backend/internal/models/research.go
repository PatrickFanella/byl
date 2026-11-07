package models

import (
	"encoding/json"
	"time"
)

// ResearchStages is the ordered private research workflow.
var ResearchStages = []string{"research", "reconciliation", "drafting", "assembly"}

// ResearchStage reports progress without returning source content or provider details.
type ResearchStage struct {
	Stage        string            `json:"stage"`
	Status       string            `json:"status"`
	Attempts     int               `json:"attempts"`
	Reason       string            `json:"reason"`
	CandidateIDs []string          `json:"candidate_ids"`
	Output       json.RawMessage   `json:"-"`
	Outcomes     []ResearchAttempt `json:"outcomes"`
}

// ResearchAttempt retains sanitized failure and recovery evidence after a retry succeeds.
type ResearchAttempt struct {
	Attempt int    `json:"attempt"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

// ResearchRun is a resumable private workflow, with a frozen input snapshot.
type ResearchRun struct {
	ID             string          `json:"id"`
	BriefID        string          `json:"brief_id"`
	JobID          string          `json:"job_id"`
	SourceVersion  int             `json:"source_version"`
	BriefVersion   int             `json:"brief_version"`
	Status         string          `json:"status"`
	Reason         string          `json:"reason"`
	RequestsUsed   int             `json:"requests_used"`
	TokensReserved int             `json:"tokens_reserved"`
	RequestLimit   int             `json:"request_limit"`
	TokenLimit     int             `json:"token_limit"`
	DeadlineAt     time.Time       `json:"deadline_at"`
	CreatedAt      time.Time       `json:"created_at"`
	Stages         []ResearchStage `json:"stages"`
	Submitted      RouteBriefInput `json:"-"`
}

// ResearchActionRequest protects start, resume and cancellation from stale edits.
type ResearchActionRequest struct {
	Version int    `json:"version"`
	RunID   string `json:"run_id,omitempty"`
}

// ResearchOutput is persisted atomically with any proposed review candidates.
// It never creates or changes public entities or overwrites existing candidates.
type ResearchOutput struct {
	Data            json.RawMessage
	Candidates      []Candidate
	NeedsResearch   string
	Reconciliations []ReconciliationProposal
}

// ResearchReasonMessage maps safe reason codes to user-facing descriptions.
func ResearchReasonMessage(code string) string {
	switch code {
	case "processor_unavailable":
		return "Route research is not configured yet."
	case "source_unavailable":
		return "A research source is unavailable."
	case "provider_unavailable":
		return "A research provider is unavailable."
	case "provider_failure":
		return "A research provider could not complete this stage."
	case "insufficient_evidence":
		return "More source evidence is needed."
	case "invalid_output":
		return "The research result needs review before it can be used."
	case "request_budget":
		return "The research request limit was reached."
	case "model_budget":
		return "The research model-use limit was reached."
	case "retry_budget":
		return "The stage retry limit was reached."
	case "elapsed_budget":
		return "The research time limit was reached."
	case "cancelled":
		return "A curator cancelled this research run."
	case "brief_changed":
		return "The brief changed; review it before starting another run."
	case "worker_retry_exhausted":
		return "The worker could not finish within its retry limit."
	}
	return ""
}
