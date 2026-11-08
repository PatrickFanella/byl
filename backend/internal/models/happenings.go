package models

import (
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

// HappeningInput is a private curator draft, never a permanent moment or trip
// mutation. Date-only announcements remain dates; no start time is invented.
type HappeningInput struct {
	PlaceID     string              `json:"place_id"`
	Title       string              `json:"title"`
	Summary     string              `json:"summary"`
	Organizer   string              `json:"organizer"`
	StartsOn    *Date               `json:"starts_on"`
	EndsOn      *Date               `json:"ends_on"`
	State       string              `json:"state"`
	ReviewOn    *Date               `json:"review_on"`
	ExpiresOn   *Date               `json:"expires_on"`
	EvidenceIDs []string            `json:"evidence_ids"`
	Claims      []HappeningCitation `json:"claims"`
}

// Claim citations refer to exact inspected passages, retained privately. The
// reviewer decides whether they support the wording, venue and occurrence.
type HappeningCitation struct {
	Field      string `json:"field"`
	EvidenceID string `json:"evidence_id"`
	Passage    string `json:"passage"`
	Quote      string `json:"quote"`
}

func (in HappeningInput) Validate(now time.Time) Fields {
	f := Fields{}
	if strings.TrimSpace(in.PlaceID) == "" {
		f["place_id"] = "select an existing place"
	}
	if strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 300 || strings.TrimSpace(in.Summary) == "" || utf8.RuneCountInString(in.Summary) > 2000 || strings.TrimSpace(in.Organizer) == "" || utf8.RuneCountInString(in.Organizer) > 300 {
		f["content"] = "provide bounded title, summary and organizer wording for source review"
	}
	if in.State != "scheduled" && in.State != "cancelled" && in.State != "date_conflict" && in.State != "source_unavailable" {
		f["state"] = "use scheduled, cancelled, date_conflict or source_unavailable"
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if in.StartsOn == nil || in.EndsOn == nil || in.StartsOn.IsZero() || in.EndsOn.IsZero() || in.EndsOn.Before(in.StartsOn.Time) || in.EndsOn.After(in.StartsOn.AddDate(0, 0, 31)) || in.StartsOn.After(day.AddDate(0, 0, 180)) || in.EndsOn.Before(day.AddDate(0, 0, -30)) {
		f["occurrence"] = "provide a date range of at most 31 days, starting within 180 days and ending no more than 30 days ago"
	}
	if in.ReviewOn == nil || in.ExpiresOn == nil || in.ReviewOn.IsZero() || in.ExpiresOn.IsZero() || !in.ReviewOn.After(day) || in.ReviewOn.After(day.AddDate(0, 0, 14)) || in.ExpiresOn.Before(in.ReviewOn.Time) || in.EndsOn == nil || in.ExpiresOn.After(in.EndsOn.AddDate(0, 0, 7)) {
		f["review"] = "choose a future review date within 14 days and an expiry no later than seven days after the occurrence ends"
	}
	if len(in.EvidenceIDs) < 1 || len(in.EvidenceIDs) > 5 || len(in.Claims) < 1 || len(in.Claims) > 12 {
		f["evidence"] = "select 1–5 retained evidence records and 1–12 passage citations"
	}
	seen := map[string]bool{}
	for _, id := range in.EvidenceIDs {
		if id == "" || len(id) > 100 || seen[id] {
			f["evidence"] = "evidence identities must be bounded and distinct"
		}
		seen[id] = true
	}
	for _, c := range in.Claims {
		if (c.Field != "title" && c.Field != "summary" && c.Field != "organizer" && c.Field != "occurrence" && c.Field != "place" && c.Field != "status") || !seen[c.EvidenceID] || c.Passage == "" || len(c.Passage) > 100 || strings.TrimSpace(c.Quote) == "" || utf8.RuneCountInString(c.Quote) > 500 {
			f["claims"] = "cite bounded exact passages from selected evidence for public wording, occurrence, place or status"
		}
	}
	return f
}

type HappeningDraft struct {
	ID                  string         `json:"id"`
	DraftID             string         `json:"draft_id"`
	Version             int64          `json:"version"`
	Input               HappeningInput `json:"input"`
	PublishedRevisionID *string        `json:"published_revision_id"`
	CreatedAt           time.Time      `json:"created_at"`
}

type HappeningWriteRequest struct {
	Version int64          `json:"version"`
	Input   HappeningInput `json:"input"`
}

type HappeningReviewPackage struct {
	Version        string              `json:"version"`
	ReadyToPublish bool                `json:"ready_to_publish"`
	Draft          HappeningDraft      `json:"draft"`
	Place          *PlaceSummary       `json:"place"`
	Evidence       []SourceEvidence    `json:"evidence"`
	Tasks          []ReviewPackageTask `json:"tasks"`
	Preview        *Happening          `json:"preview"`
	Current        *Happening          `json:"current"`
	LatestChecks   []SourceEvidence    `json:"latest_checks"`
	ReviewedAction string              `json:"reviewed_action"`
	ReviewNote     string              `json:"review_note"`
	ReviewedBy     string              `json:"reviewed_by"`
	ReviewedAt     *time.Time          `json:"reviewed_at"`
	ReviewDay      string              `json:"review_day"`
}

type HappeningReviewRequest struct {
	Version string `json:"version"`
	Action  string `json:"action"`
	Note    string `json:"note"`
}

type HappeningDecision struct {
	DraftID  string     `json:"draft_id"`
	Action   string     `json:"action"`
	Revision *Happening `json:"revision"`
}

// Happening is the separate public current-notice projection. It contains no
// private passage text, source retrieval failures, reviewer identity or note.
type Happening struct {
	ID            string          `json:"id"`
	RevisionID    string          `json:"revision_id"`
	PlaceID       string          `json:"place_id"`
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	Organizer     string          `json:"organizer"`
	StartsOn      Date            `json:"starts_on"`
	EndsOn        Date            `json:"ends_on"`
	Timezone      string          `json:"timezone"`
	DeclaredState string          `json:"declared_state"`
	State         string          `json:"state"`
	ReviewOn      Date            `json:"review_on"`
	ExpiresOn     Date            `json:"expires_on"`
	LastCheckedAt time.Time       `json:"last_checked_at"`
	SourceHealth  string          `json:"source_health"`
	Sources       []JourneySource `json:"sources"`
	Issues        []string        `json:"issues"`
}

// RefreshStatus is clock-controlled and never writes or changes a story. The
// city date controls occurrence/expiry; evidence freshness uses elapsed time.
func (h *Happening) RefreshStatus(now time.Time, sourceUnavailable bool) {
	zone, err := time.LoadLocation(h.Timezone)
	if err != nil {
		h.State = "needs_review"
		h.Issues = append(h.Issues, "timezone_unavailable")
		return
	}
	local := now.In(zone)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	h.State = h.DeclaredState
	if h.State != "scheduled" {
		return
	}
	switch {
	case sourceUnavailable:
		h.State = "source_unavailable"
	case h.SourceHealth != "" && h.SourceHealth != "unchanged":
		h.State = "needs_review"
	case !day.Before(h.ExpiresOn.Time):
		h.State = "expired"
	case !day.Before(h.ReviewOn.Time), h.LastCheckedAt.IsZero(), now.Before(h.LastCheckedAt), now.Sub(h.LastCheckedAt) >= 7*24*time.Hour:
		h.State = "needs_review"
	case day.Before(h.StartsOn.Time):
		h.State = "upcoming"
	case day.After(h.EndsOn.Time):
		h.State = "ended"
	default:
		h.State = "active"
	}
}
