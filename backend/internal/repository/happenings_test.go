package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

type happeningFixture struct {
	s                   *Store
	ctx                 context.Context
	actor, brief, place string
	now                 time.Time
	input               models.HappeningInput
	proof               *models.SourceEvidence
}

func TestHappeningMigrationRetainsExistingRowsAndRouteHashes(t *testing.T) {
	assertNavigationMigrationPreservesRows(t, 20)
}

func newHappeningFixture(t *testing.T) *happeningFixture {
	t.Helper()
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "happening-curator", "happening@example.com", "", true)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Synthetic current notice", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	tour, err := s.GetTour(ctx, "downtown-memory-loop", false)
	if err != nil {
		t.Fatal(err)
	}
	f := &happeningFixture{s: s, ctx: ctx, actor: a.ID, brief: b.ID, place: tour.Stops[0].Place.ID, now: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	raw, err := os.ReadFile("testdata/happenings/announcement.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.proof = f.evidence(t, "inspected_page", string(raw), "initial")
	date := func(value string) *models.Date {
		d, err := models.ParseDate(value)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	f.input = models.HappeningInput{PlaceID: f.place, Title: "Synthetic park history talk", Summary: "A curator will discuss local history.", Organizer: "Fixture Parks", StartsOn: date("2026-11-10"), EndsOn: date("2026-11-10"), State: "scheduled", ReviewOn: date("2026-11-11"), ExpiresOn: date("2026-11-12"), EvidenceIDs: []string{f.proof.ID}, Claims: f.claims(f.proof.ID, "Fixture Parks invites visitors to the Synthetic park history talk at Main Street Station on 2026-11-10.")}
	return f
}

func (f *happeningFixture) claims(id, quote string) []models.HappeningCitation {
	out := []models.HappeningCitation{}
	for _, field := range []string{"title", "summary", "organizer", "occurrence", "place"} {
		out = append(out, models.HappeningCitation{Field: field, EvidenceID: id, Passage: "paragraph:1", Quote: quote})
	}
	return out
}

func (f *happeningFixture) evidence(t *testing.T, outcome, text, key string) *models.SourceEvidence {
	t.Helper()
	e, err := f.s.QueueSourceEvidence(f.ctx, f.brief, models.EvidenceRequest{Version: 1, URL: "https://organizer.example/test-announcement", SourceKind: "page"}, "page", strings.Repeat("d", 64), key)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(text))
	result := models.EvidenceResult{Outcome: outcome, FinalURL: e.URL, Title: "Synthetic organizer announcement", Publisher: "Fixture Parks", RetrievedAt: f.now, HTTPStatus: 200, ContentType: "text/html", ContentSHA256: hex.EncodeToString(hash[:]), Passages: []models.EvidencePassage{}, Untrusted: true, MediaReuse: "unresolved"}
	if outcome == "inspected_page" {
		result.Passages = []models.EvidencePassage{{Reference: "paragraph:1", Text: text}}
	} else {
		result.HTTPStatus = 503
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	// An explicitly synthetic retained retrieval, like the existing package tests;
	// safe network fetching and source parsing are qualified separately.
	if _, err := f.s.pool.Exec(f.ctx, `UPDATE source_evidence SET result=$2 WHERE id=$1`, e.ID, raw); err != nil {
		t.Fatal(err)
	}
	e, err = f.s.GetSourceEvidence(f.ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (f *happeningFixture) draft(t *testing.T) *models.HappeningDraft {
	t.Helper()
	d, err := f.s.SaveHappeningDraft(f.ctx, "", f.actor, "happening-create-key", models.HappeningWriteRequest{Input: f.input}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *happeningFixture) approve(t *testing.T, id string) *models.HappeningDecision {
	t.Helper()
	p, err := f.s.GetHappeningReviewPackage(f.ctx, id, f.now)
	if err != nil || !p.ReadyToPublish {
		t.Fatalf("review package %+v %v", p, err)
	}
	r, err := f.s.ReviewHappening(f.ctx, id, f.actor, models.HappeningReviewRequest{Version: p.Version, Action: "approve", Note: "Synthetic exact-read-set source review"}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestHappeningReviewPublishesSeparateImmutableNoticeAndRetainsRetries(t *testing.T) {
	f := newHappeningFixture(t)
	history := func() string {
		var hash string
		if err := f.s.pool.QueryRow(f.ctx, `SELECT md5(COALESCE(string_agg(row_to_json(m)::text,'' ORDER BY id),'')) FROM moments m`).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		return hash
	}
	before := history()
	d := f.draft(t)
	public, err := f.s.PublicHappenings(f.ctx, f.place, 20, 0, f.now)
	if err != nil || len(public) != 0 {
		t.Fatal("draft leaked", public, err)
	}
	again, err := f.s.SaveHappeningDraft(f.ctx, "", f.actor, "happening-create-key", models.HappeningWriteRequest{Input: f.input}, f.now)
	if err != nil || again.DraftID != d.DraftID {
		t.Fatal("create retry changed draft", err)
	}
	p, err := f.s.GetHappeningReviewPackage(f.ctx, d.ID, f.now)
	if err != nil || !p.ReadyToPublish {
		t.Fatalf("package %+v %v", p, err)
	}
	in := models.HappeningReviewRequest{Version: p.Version, Action: "approve", Note: "Synthetic exact-read-set source review"}
	r, err := f.s.ReviewHappening(f.ctx, d.ID, f.actor, in, f.now)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.ReviewHappening(f.ctx, d.ID, f.actor, in, f.now)
	if err != nil || replayed.Revision.RevisionID != r.Revision.RevisionID {
		t.Fatal("approval replay changed publication", err)
	}
	public, err = f.s.PublicHappenings(f.ctx, f.place, 20, 0, f.now)
	if err != nil || len(public) != 1 || public[0].State != "upcoming" || public[0].RevisionID != r.Revision.RevisionID {
		t.Fatal(public, err)
	}
	raw, _ := json.Marshal(public)
	if strings.Contains(string(raw), "paragraph:1") || strings.Contains(string(raw), "reviewed_by") || strings.Contains(string(raw), "private_package") {
		t.Fatal("public response leaked private proof")
	}
	for _, q := range []string{`UPDATE happening_drafts SET content='{}' WHERE id=$1`, `UPDATE happening_reviews SET note='changed' WHERE draft_id=$1`} {
		if _, err := f.s.pool.Exec(f.ctx, q, d.DraftID); err == nil {
			t.Fatal("review history mutable")
		}
	}
	if _, err := f.s.pool.Exec(f.ctx, `UPDATE happening_revisions SET public_snapshot='{}' WHERE id=$1`, r.Revision.RevisionID); err == nil {
		t.Fatal("public snapshot mutable")
	}
	public, err = f.s.PublicHappenings(f.ctx, f.place, 20, 0, time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC))
	if err != nil || public[0].State != "expired" {
		t.Fatal(public, err)
	}
	if history() != before {
		t.Fatal("expiry/publication rewrote historical moments")
	}
}

func TestHappeningChangedEvidenceOrDraftFencesApprovalAndUnavailabilityIsVisible(t *testing.T) {
	f := newHappeningFixture(t)
	d := f.draft(t)
	p, err := f.s.GetHappeningReviewPackage(f.ctx, d.ID, f.now)
	if err != nil {
		t.Fatal(err)
	}
	latest := f.evidence(t, "inspected_page", "A revised synthetic announcement has conflicting dates.", "changed")
	if _, err := f.s.ReviewHappening(f.ctx, d.ID, f.actor, models.HappeningReviewRequest{Version: p.Version, Action: "approve", Note: "Old read set"}, f.now); !errors.Is(err, ErrConflict) {
		t.Fatal("changed evidence approved", err)
	}
	p, err = f.s.GetHappeningReviewPackage(f.ctx, d.ID, f.now)
	if err != nil || p.ReadyToPublish || p.Preview.SourceHealth != "changed" {
		t.Fatal("changed source was silently current", p, err)
	}
	// Selecting the fresh capture first must not conceal an older cited version
	// of the same URL later in the explicit read set.
	input := f.input
	input.EvidenceIDs = []string{latest.ID, f.proof.ID}
	if _, err := f.s.SaveHappeningDraft(f.ctx, d.ID, f.actor, "mixed-source-versions-key", models.HappeningWriteRequest{Version: d.Version, Input: input}, f.now); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.GetHappeningReviewPackage(f.ctx, d.ID, f.now)
	if err != nil || p.ReadyToPublish || p.Preview.SourceHealth != "changed" {
		t.Fatal("older cited version was hidden by fresh evidence ordering", p, err)
	}
	// A second fixture publishes before its next source attempt fails.
	f = newHappeningFixture(t)
	d = f.draft(t)
	f.approve(t, d.ID)
	f.evidence(t, "unavailable", "", "unavailable")
	public, err := f.s.PublicHappenings(f.ctx, f.place, 20, 0, f.now)
	if err != nil || len(public) != 1 || public[0].State != "source_unavailable" {
		t.Fatal(public, err)
	}
}

func TestHappeningCancellationAndRejectionPreservePriorRelease(t *testing.T) {
	f := newHappeningFixture(t)
	d := f.draft(t)
	original := f.approve(t, d.ID)
	quote := "Fixture Parks cancelled the Synthetic park history talk at Main Street Station on 2026-11-10."
	e := f.evidence(t, "inspected_page", quote, "cancelled")
	input := f.input
	input.State = "cancelled"
	input.Summary = "The organizer cancelled this synthetic occurrence."
	input.EvidenceIDs = []string{e.ID}
	input.Claims = f.claims(e.ID, quote)
	input.Claims = append(input.Claims, models.HappeningCitation{Field: "status", EvidenceID: e.ID, Passage: "paragraph:1", Quote: quote})
	next, err := f.s.SaveHappeningDraft(f.ctx, d.ID, f.actor, "cancelled-draft-key", models.HappeningWriteRequest{Version: d.Version, Input: input}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	public, err := f.s.PublicHappenings(f.ctx, f.place, 20, 0, f.now)
	if err != nil || public[0].RevisionID != original.Revision.RevisionID {
		t.Fatal("draft changed published wording", err)
	}
	p, err := f.s.GetHappeningReviewPackage(f.ctx, d.ID, f.now)
	if err != nil || p.Draft.DraftID != next.DraftID || !p.ReadyToPublish {
		t.Fatal(p, err)
	}
	if _, err := f.s.ReviewHappening(f.ctx, d.ID, f.actor, models.HappeningReviewRequest{Version: p.Version, Action: "reject", Note: "Synthetic rejected proposal"}, f.now); err != nil {
		t.Fatal(err)
	}
	public, err = f.s.PublicHappenings(f.ctx, f.place, 20, 0, f.now)
	if err != nil || public[0].RevisionID != original.Revision.RevisionID {
		t.Fatal("rejection rewrote release", err)
	}
	if _, err := f.s.SaveHappeningDraft(f.ctx, d.ID, f.actor, "fresh-cancel-review-key", models.HappeningWriteRequest{Version: next.Version, Input: input}, f.now); err != nil {
		t.Fatal(err)
	}
	cancelled := f.approve(t, d.ID)
	if cancelled.Revision.State != "cancelled" {
		t.Fatal(cancelled)
	}
	var count int
	if err := f.s.pool.QueryRow(f.ctx, `SELECT count(*) FROM happening_revisions WHERE happening_id=$1`, d.ID).Scan(&count); err != nil || count != 2 {
		t.Fatal("publication history lost", count, err)
	}
}

func TestHappeningUncitedUserProseAndStaleSourceCannotPublish(t *testing.T) {
	for _, mode := range []string{"uncited", "stale", "catalog"} {
		t.Run(mode, func(t *testing.T) {
			f := newHappeningFixture(t)
			switch mode {
			case "uncited":
				f.input.Claims[0].Quote = "Direct user prose never inspected"
			case "stale":
				f.now = f.now.Add(8 * 24 * time.Hour)
				date, _ := models.ParseDate(f.now.AddDate(0, 0, 1).Format("2006-01-02"))
				f.input.ReviewOn = date
				f.input.ExpiresOn = date
			case "catalog":
				e := f.evidence(t, "catalog_only", "", "catalog")
				f.input.EvidenceIDs = []string{e.ID}
				for i := range f.input.Claims {
					f.input.Claims[i].EvidenceID = e.ID
				}
			}
			d := f.draft(t)
			p, err := f.s.GetHappeningReviewPackage(f.ctx, d.ID, f.now)
			if err != nil || p.ReadyToPublish {
				t.Fatal("unsupported draft is publishable", p, err)
			}
			if _, err := f.s.ReviewHappening(f.ctx, d.ID, f.actor, models.HappeningReviewRequest{Version: p.Version, Action: "approve", Note: fmt.Sprintf("Attempt %s", mode)}, f.now); !errors.Is(err, ErrConflict) {
				t.Fatal("unsupported announcement published", err)
			}
		})
	}
}
