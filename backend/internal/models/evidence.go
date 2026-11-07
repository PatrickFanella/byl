package models

import (
	"time"
	"unicode/utf8"
)

// EvidencePassage is a bounded, untrusted excerpt with a stable paragraph reference.
type EvidencePassage struct {
	Reference string `json:"reference"`
	Text      string `json:"text"`
}

// EvidenceResult records what was actually inspected, separately from media rights.
// Page access is permission neither to redistribute its text nor to reuse its images.
type EvidenceResult struct {
	Outcome       string            `json:"outcome"`
	FinalURL      string            `json:"final_url"`
	Publisher     string            `json:"publisher"`
	Title         string            `json:"title"`
	RetrievedAt   time.Time         `json:"retrieved_at"`
	HTTPStatus    int               `json:"http_status"`
	ContentType   string            `json:"content_type"`
	ContentSHA256 string            `json:"content_sha256"`
	Passages      []EvidencePassage `json:"passages"`
	Truncated     bool              `json:"truncated"`
	Untrusted     bool              `json:"untrusted"`
	MediaReuse    string            `json:"media_reuse"`
}

// SourceEvidence is a private retrieval snapshot. SourceKey identifies the
// normalized source URL; ID identifies this particular retrieval request.
type SourceEvidence struct {
	ID           string          `json:"id"`
	BriefID      string          `json:"brief_id"`
	BriefVersion int             `json:"brief_version"`
	JobID        string          `json:"job_id"`
	SourceKey    string          `json:"source_key"`
	URL          string          `json:"url"`
	Reference    string          `json:"reference"`
	SourceKind   string          `json:"source_kind"`
	Result       *EvidenceResult `json:"result"`
	CreatedAt    time.Time       `json:"created_at"`
}

// EvidenceRequest asks a curator-only job to inspect a page or catalogue record.
type EvidenceRequest struct {
	Version    int    `json:"version"`
	URL        string `json:"url"`
	SourceKind string `json:"source_kind"`
}

// FetchEvidencePayload keeps source content and potentially sensitive URLs out of jobs/logs.
type FetchEvidencePayload struct {
	EvidenceID string `json:"evidence_id"`
}

// Valid rejects oversized or mislabeled results before an immutable checkpoint.
func (r EvidenceResult) Valid() bool {
	switch r.Outcome {
	case "inspected_page", "catalog_only", "search_snippet", "blocked", "timeout", "unavailable", "inaccessible", "too_large", "unsupported_format", "unsupported_encoding", "no_readable_text", "worker_failed":
	default:
		return false
	}
	if len(r.FinalURL) > 2000 || utf8.RuneCountInString(r.Title) > 500 || len(r.Publisher) > 255 || len(r.ContentType) > 100 || len(r.Passages) > 100 {
		return false
	}
	if r.Outcome != "inspected_page" && r.Outcome != "catalog_only" && len(r.Passages) > 0 {
		return false
	}
	size := 0
	for _, p := range r.Passages {
		size += utf8.RuneCountInString(p.Text)
		if len(p.Reference) > 100 {
			return false
		}
	}
	return size <= 20000
}
