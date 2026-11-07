// Package llm drafts moments and ranks them for a topic through a language model.
// Two providers exist: the Claude API (official SDK) and any OpenAI-compatible endpoint.
package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrUnavailable is returned when no provider is configured.
var ErrUnavailable = errors.New("llm: no provider configured")

// TopicOption is a topic the model may assign.
type TopicOption struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DraftRequest offers inspected evidence and context labels only. Submitted
// anecdotes and candidate prose have no field in this provider boundary.
type DraftRequest struct {
	PlaceName string
	Topics    []TopicOption
	Evidence  []DraftEvidence
}

// MomentDraft is the model's proposal.
type MomentDraft struct {
	Title          string             `json:"title"`
	Body           string             `json:"body"`
	DateStart      string             `json:"date_start"`
	DateEnd        string             `json:"date_end"`
	DatePrecision  string             `json:"date_precision"`
	DateDisplay    string             `json:"date_display"`
	TopicSlugs     []string           `json:"topic_slugs"`
	Notes          string             `json:"notes"`
	Claims         []DraftClaim       `json:"claims"`
	TitleCitations []DraftCitation    `json:"title_citations"`
	DateCitations  []DraftCitation    `json:"date_citations"`
	Uncertainties  []DraftUncertainty `json:"uncertainties"`
}

// MomentOption is a moment the model may rank.
type MomentOption struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Excerpt     string `json:"excerpt"`
	DateDisplay string `json:"date_display"`
}

// RankRequest asks which moments best fit a tour topic at a place.
type RankRequest struct {
	TopicName        string
	TopicDescription string
	TourTitle        string
	TourSummary      string
	PlaceName        string
	Moments          []MomentOption
}

// RankedMoment is one entry of a ranking.
type RankedMoment struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Ranking orders moments best first.
type Ranking struct {
	Ordered []RankedMoment `json:"ordered"`
}

// Drafter is implemented by every provider.
type Drafter interface {
	DraftMoment(ctx context.Context, req DraftRequest) (MomentDraft, error)
	RankMoments(ctx context.Context, req RankRequest) (Ranking, error)
}

var validPrecisions = map[string]bool{"day": true, "month": true, "year": true, "circa": true}

// Check validates the shape the rest of the system relies on.
func (d MomentDraft) Check() (MomentDraft, error) {
	d.Title = strings.TrimSpace(d.Title)
	d.Body = strings.TrimSpace(d.Body)
	if d.Title == "" {
		return d, errors.New("draft has no title")
	}
	if d.Body == "" {
		return d, errors.New("draft has no body")
	}
	for _, s := range []string{d.DateStart, d.DateEnd} {
		if s == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return d, fmt.Errorf("draft date %q is not YYYY-MM-DD", s)
		}
	}
	if !validPrecisions[d.DatePrecision] {
		return d, fmt.Errorf("draft precision %q is invalid", d.DatePrecision)
	}
	return d, nil
}

// ValidateDraft normalises a draft against the offered topics. It never rejects; it repairs and reports.
func ValidateDraft(d MomentDraft, offered []string) (MomentDraft, []string) {
	allowed := map[string]bool{}
	for _, s := range offered {
		allowed[s] = true
	}
	var kept, dropped []string
	for _, s := range d.TopicSlugs {
		s = strings.TrimSpace(s)
		if allowed[s] {
			kept = append(kept, s)
		} else if s != "" {
			dropped = append(dropped, s)
		}
	}
	d.TopicSlugs = kept
	if !validPrecisions[d.DatePrecision] {
		d.DatePrecision = "circa"
	}
	if d.DateStart != "" {
		if _, err := time.Parse("2006-01-02", d.DateStart); err != nil {
			d.DateStart = ""
		}
	}
	if d.DateEnd != "" {
		if _, err := time.Parse("2006-01-02", d.DateEnd); err != nil {
			d.DateEnd = ""
		}
	}
	if len(dropped) > 0 {
		note := "Dropped topics not offered: " + strings.Join(dropped, ", ")
		if d.Notes != "" {
			d.Notes += " "
		}
		d.Notes += note
	}
	return d, dropped
}

// None is the provider used when LLM_PROVIDER is empty.
type None struct{}

func (None) DraftMoment(context.Context, DraftRequest) (MomentDraft, error) {
	return MomentDraft{}, ErrUnavailable
}

func (None) RankMoments(context.Context, RankRequest) (Ranking, error) {
	return Ranking{}, ErrUnavailable
}
