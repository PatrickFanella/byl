package models

import (
	"strings"
	"testing"
	"time"
)

func happeningDate(value string) *Date { d, _ := ParseDate(value); return d }

func TestHappeningDateStatesDoNotInventOccurrenceTimes(t *testing.T) {
	base := Happening{DeclaredState: "scheduled", Timezone: "America/Chicago", StartsOn: *happeningDate("2026-11-08"), EndsOn: *happeningDate("2026-11-09"), ReviewOn: *happeningDate("2026-11-11"), ExpiresOn: *happeningDate("2026-11-12"), LastCheckedAt: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	for _, example := range []struct {
		name, at, state string
		unavailable     bool
	}{
		{"local day before occurrence", "2026-11-08T03:00:00Z", "upcoming", false},
		{"active date", "2026-11-08T18:00:00Z", "active", false},
		{"last active date", "2026-11-10T03:00:00Z", "active", false},
		{"ended", "2026-11-10T18:00:00Z", "ended", false},
		{"review boundary", "2026-11-11T06:00:00Z", "needs_review", false},
		{"expiry boundary", "2026-11-12T06:00:00Z", "expired", false},
		{"source unavailable", "2026-11-08T18:00:00Z", "source_unavailable", true},
	} {
		t.Run(example.name, func(t *testing.T) {
			h := base
			now, _ := time.Parse(time.RFC3339, example.at)
			h.RefreshStatus(now, example.unavailable)
			if h.State != example.state {
				t.Fatalf("got %s want %s", h.State, example.state)
			}
		})
	}
	for _, declared := range []string{"cancelled", "date_conflict", "source_unavailable"} {
		h := base
		h.DeclaredState = declared
		h.RefreshStatus(time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), false)
		if h.State != declared {
			t.Fatal("notice was silently relabeled as an event")
		}
	}
	h := base
	h.ReviewOn = *happeningDate("2026-11-20")
	h.ExpiresOn = *happeningDate("2026-11-21")
	h.RefreshStatus(h.LastCheckedAt.Add(7*24*time.Hour), false)
	if h.State != "needs_review" {
		t.Fatal("stale source remained current")
	}
	h = base
	h.Timezone = "invalid/timezone"
	h.RefreshStatus(time.Now(), false)
	if h.State != "needs_review" {
		t.Fatal("unknown timezone guessed a date")
	}
}

func TestHappeningDraftRequiresBoundedSourceReviewAndExpiry(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	fixture := func() HappeningInput {
		return HappeningInput{PlaceID: "place", Title: "Synthetic organizer announcement", Summary: "Synthetic source-backed draft", Organizer: "Fixture organizer", StartsOn: happeningDate("2026-11-10"), EndsOn: happeningDate("2026-11-10"), State: "scheduled", ReviewOn: happeningDate("2026-11-09"), ExpiresOn: happeningDate("2026-11-11"), EvidenceIDs: []string{"evidence"}, Claims: []HappeningCitation{{Field: "summary", EvidenceID: "evidence", Passage: "paragraph:1", Quote: "Synthetic source-backed draft"}}}
	}
	if f := fixture().Validate(now); len(f) > 0 {
		t.Fatal(f)
	}
	for name, change := range map[string]func(*HappeningInput){
		"no evidence":          func(in *HappeningInput) { in.EvidenceIDs = nil },
		"no review":            func(in *HappeningInput) { in.ReviewOn = nil },
		"no expiry":            func(in *HappeningInput) { in.ExpiresOn = nil },
		"review too far":       func(in *HappeningInput) { in.ReviewOn = happeningDate("2026-12-01") },
		"reversed dates":       func(in *HappeningInput) { in.EndsOn = happeningDate("2026-11-09") },
		"unbounded occurrence": func(in *HappeningInput) { in.EndsOn = happeningDate("2027-01-01") },
		"unbounded expiry":     func(in *HappeningInput) { in.ExpiresOn = happeningDate("2027-01-01") },
		"unknown state":        func(in *HappeningInput) { in.State = "approved" },
		"uncaptured passage":   func(in *HappeningInput) { in.Claims[0].EvidenceID = "other" },
		"unbounded quotation":  func(in *HappeningInput) { in.Claims[0].Quote = strings.Repeat("x", 501) },
	} {
		t.Run(name, func(t *testing.T) {
			in := fixture()
			change(&in)
			if len(in.Validate(now)) == 0 {
				t.Fatal("unbounded/unreviewable draft accepted")
			}
		})
	}
}
