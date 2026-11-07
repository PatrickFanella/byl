package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"invisible-museum/backend/internal/models"
)

var ErrInvalidDraft = errors.New("llm: invalid evidence-linked draft")

// DraftEvidence identifies an immutable inspected snapshot. Passages are untrusted data.
type DraftEvidence struct {
	ID            string                   `json:"id"`
	URL           string                   `json:"url"`
	Title         string                   `json:"title"`
	Publisher     string                   `json:"publisher"`
	Outcome       string                   `json:"outcome"`
	ContentSHA256 string                   `json:"content_sha256"`
	RetrievedAt   string                   `json:"retrieved_at"`
	Passages      []models.EvidencePassage `json:"passages,omitempty"`
}

type DraftCitation struct {
	EvidenceID string `json:"evidence_id"`
	Passage    string `json:"passage"`
	Quote      string `json:"quote"`
}

type DraftClaim struct {
	Text        string          `json:"text"`
	Kind        string          `json:"kind"`
	Attribution string          `json:"attribution"`
	Citations   []DraftCitation `json:"citations"`
}

type DraftUncertainty struct {
	Kind      string          `json:"kind"`
	Text      string          `json:"text"`
	Citations []DraftCitation `json:"citations"`
}

// ValidateEvidence checks traceability, not whether an excerpt entails a claim.
// Body is exactly the cited claim paragraphs; there is no uncited prose channel.
func ValidateEvidence(d MomentDraft, req DraftRequest) (MomentDraft, error) {
	bad := func() (MomentDraft, error) { return d, ErrInvalidDraft }
	var err error
	if d, err = d.Check(); err != nil {
		return bad()
	}
	if len(req.Evidence) < 1 || len(req.Evidence) > 5 || len(d.Claims) < 1 || len(d.Claims) > 20 || len(d.Uncertainties) > 8 || utf8.RuneCountInString(d.Title) > 160 || utf8.RuneCountInString(d.Body) > 12000 || utf8.RuneCountInString(d.Notes) > 4000 {
		return bad()
	}
	passages := map[string]string{}
	for _, e := range req.Evidence {
		for _, p := range e.Passages {
			passages[e.ID+"/"+p.Reference] = p.Text
		}
	}
	validCitations := func(c []DraftCitation, required bool) bool {
		if (required && len(c) == 0) || len(c) > 6 {
			return false
		}
		for _, ref := range c {
			text, exists := passages[ref.EvidenceID+"/"+ref.Passage]
			if !exists || strings.TrimSpace(ref.Quote) == "" || utf8.RuneCountInString(ref.Quote) > 300 || !strings.Contains(text, ref.Quote) {
				return false
			}
		}
		return true
	}
	if !validCitations(d.TitleCitations, true) || !validCitations(d.DateCitations, d.DateStart != "" || d.DateEnd != "" || d.DateDisplay != "") {
		return bad()
	}
	paragraphs := []string{}
	for _, claim := range d.Claims {
		if strings.TrimSpace(claim.Text) == "" || strings.TrimSpace(claim.Text) != claim.Text || strings.Contains(claim.Text, "\n") || utf8.RuneCountInString(claim.Text) > 2000 || !validCitations(claim.Citations, true) {
			return bad()
		}
		if claim.Kind != "fact" && claim.Kind != "testimony" {
			return bad()
		}
		if claim.Kind == "testimony" && (strings.TrimSpace(claim.Attribution) == "" || !strings.Contains(claim.Text, claim.Attribution)) {
			return bad()
		}
		if utf8.RuneCountInString(claim.Attribution) > 200 {
			return bad()
		}
		paragraphs = append(paragraphs, claim.Text)
	}
	if d.Body != strings.Join(paragraphs, "\n\n") {
		return bad()
	}
	for _, u := range d.Uncertainties {
		if (u.Kind != "conflicting_dates" && u.Kind != "research_gap") || strings.TrimSpace(u.Text) == "" || utf8.RuneCountInString(u.Text) > 1000 || !validCitations(u.Citations, u.Kind == "conflicting_dates") {
			return bad()
		}
		if u.Kind == "conflicting_dates" {
			refs := map[string]bool{}
			for _, c := range u.Citations {
				refs[c.EvidenceID+"/"+c.Passage] = true
			}
			if len(refs) < 2 || d.DateStart != "" || d.DateEnd != "" || d.DateDisplay != "" || d.DatePrecision != "circa" {
				return bad()
			}
		}
	}
	if raw, _ := json.Marshal(d); len(raw) > 32768 {
		return bad()
	}
	if d.DateCitations == nil {
		d.DateCitations = []DraftCitation{}
	}
	if d.Uncertainties == nil {
		d.Uncertainties = []DraftUncertainty{}
	}
	if d.TopicSlugs == nil {
		d.TopicSlugs = []string{}
	}
	return d, nil
}

func decodeDraft(raw string, req DraftRequest) (MomentDraft, error) {
	var d MomentDraft
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return d, ErrInvalidDraft
	}
	for _, key := range MomentDraftSchema()["required"].([]string) {
		value, exists := fields[key]
		if !exists || strings.TrimSpace(string(value)) == "null" {
			return d, ErrInvalidDraft
		}
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return d, fmt.Errorf("%w: malformed response", ErrInvalidDraft)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return d, fmt.Errorf("%w: trailing response", ErrInvalidDraft)
	}
	return ValidateEvidence(d, req)
}
