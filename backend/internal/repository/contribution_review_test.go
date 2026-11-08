package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"invisible-museum/backend/internal/models"
	"strings"
	"testing"
	"time"
)

// Only synthetic scopes/counts are seeded. This never activates production intake.
func reviewObservation(t *testing.T, s *Store, ctx context.Context, r *models.TourRevision, summary models.ContributionSummary, scope int, observed time.Time, departure int) string {
	t.Helper()
	var consent string
	owner := fmt.Sprintf("%064x", scope)
	created := models.ContributionDay(observed)
	err := s.pool.QueryRow(ctx, `INSERT INTO route_contribution_consents(owner_scope,tour_id,revision_id,mode,grant_key_hash,request_hash,deletion_hash,requested_on,created_on,expires_on) VALUES($1,$2,$3,$4,repeat('a',64),repeat('b',64),repeat('c',64),$5,$5,$5::date+30) ON CONFLICT(owner_scope,grant_key_hash) DO UPDATE SET owner_scope=excluded.owner_scope RETURNING id`, owner, r.TourID, r.ID, summary.Mode, created).Scan(&consent)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO route_contributions(consent_id,owner_scope,segment_id,direction,observation_period,schema_version,detector_version,calibrated,qualified_passages,sustained_departure_rejoins,submission_key_hash,request_hash,expires_on) VALUES($1,$2,$3,$4,$5,1,'uncalibrated-v1',false,1,$6,$7,repeat('d',64),$8)`, consent, owner, summary.SegmentID, summary.Direction, created, departure, fmt.Sprintf("%064x", scope*10000+created.Day()+int(summary.Mode[0])), created.AddDate(0, 0, 30))
	if err != nil {
		t.Fatal(err)
	}
	return consent
}
func reviewFilter(r *models.TourRevision, s models.ContributionSummary, week time.Time) ContributionReviewFilter {
	return ContributionReviewFilter{RevisionID: r.ID, SegmentID: s.SegmentID, Mode: s.Mode, Direction: s.Direction, WindowStart: week, Limit: 1}
}
func TestContributionReviewLiveThresholdWithdrawalAndExpiry(t *testing.T) {
	s, ctx, _, r, _, summary := contributionFixture(t)
	now := time.Now()
	week := ContributionReviewWindow(now)
	var consent string
	for i := 1; i <= 5; i++ {
		d := 0
		if i <= 3 {
			d = 1
		}
		consent = reviewObservation(t, s, ctx, r, summary, i, now, d)
	}
	list, err := s.ListContributionReviews(ctx, ContributionReviewFilter{Limit: 50}, now)
	if err != nil || len(list) != 1 {
		t.Fatalf("flagged tasks: %d %v", len(list), err)
	}
	item := list[0]
	if !item.Flagged || item.DepartureRejoins != 3 || item.QualifiedPassages != 5 || item.ContributorScopes != 5 || item.DepartureScopes != 3 || item.Geometry == nil {
		t.Fatalf("missing sample/reference context: %+v", item)
	}
	raw, _ := json.Marshal(item)
	for _, key := range []string{"owner_scope", "account_id", "consent_id", "deletion_handle", "grant_key"} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("private key exposed: %s", key)
		}
	}
	// Withdrawal deletes one live scope. A stale detail still shows current
	// counts and no flag; no persisted review/task can survive that withdrawal.
	_, err = s.pool.Exec(ctx, `UPDATE route_contribution_consents SET deletion_hash=$2 WHERE id=$1`, consent, contributionHash(strings.Repeat("e", 64)))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WithdrawRouteContributions(ctx, models.ContributionWithdrawal{ConsentID: consent, DeletionHandle: strings.Repeat("e", 64)}, now); err != nil {
		t.Fatal(err)
	}
	detail, err := s.ListContributionReviews(ctx, reviewFilter(r, summary, week), now)
	if err != nil || len(detail) != 1 || detail[0].Flagged || detail[0].QualifiedPassages != 4 {
		t.Fatalf("withdrawal did not clear flag: %+v %v", detail, err)
	}
	list, err = s.ListContributionReviews(ctx, ContributionReviewFilter{}, now)
	if err != nil || len(list) != 0 {
		t.Fatalf("stale task remained: %d %v", len(list), err)
	}
	// Expiry filters before the physical cleanup command runs.
	_, err = s.pool.Exec(ctx, `UPDATE route_contributions SET expires_on=$1`, models.ContributionDay(now))
	if err != nil {
		t.Fatal(err)
	}
	detail, err = s.ListContributionReviews(ctx, reviewFilter(r, summary, week), now)
	if err != nil || detail[0].QualifiedPassages != 0 || detail[0].Flagged {
		t.Fatalf("expired detail: %+v %v", detail, err)
	}
	// Consent expiry independently excludes otherwise-live child observations.
	if _, err := s.pool.Exec(ctx, `UPDATE route_contributions SET expires_on=$1::date+1`, models.ContributionDay(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE route_contribution_consents SET requested_on=$1::date-30,created_on=$1::date-30,expires_on=$1`, models.ContributionDay(now)); err != nil {
		t.Fatal(err)
	}
	detail, err = s.ListContributionReviews(ctx, reviewFilter(r, summary, week), now)
	if err != nil || detail[0].QualifiedPassages != 0 || detail[0].Flagged {
		t.Fatalf("expired consent counted: %+v %v", detail, err)
	}

	// The published pointer, working draft and report store have not been mutated.
	working, err := s.GetTour(ctx, r.TourID, true)
	if err != nil || working.RevisionID == nil || *working.RevisionID != r.ID {
		t.Fatalf("public itinerary changed: %v", err)
	}
}
func TestContributionReviewOneRepeatVisitorMissingAndLowShareNeverFlag(t *testing.T) {
	s, ctx, _, r, _, summary := contributionFixture(t)
	now := time.Now()
	week := ContributionReviewWindow(now).AddDate(0, 0, -7)
	for d := 0; d < 5; d++ {
		reviewObservation(t, s, ctx, r, summary, 1, week.AddDate(0, 0, d), 1)
	}
	detail, err := s.ListContributionReviews(ctx, reviewFilter(r, summary, week), now)
	if err != nil || detail[0].Flagged || detail[0].ContributorScopes != 1 || detail[0].QualifiedPassages != 5 {
		t.Fatalf("repeat visitor: %+v %v", detail, err)
	}
	// Four more scopes supply five passages each, with two more departure
	// scopes. Even with three departure scopes the share remains below 30%.
	// Denominator is only explicit observations, never missing visitors/days.
	for i := 2; i <= 5; i++ {
		for d := 0; d < 5; d++ {
			departure := 0
			if (i == 2 || i == 3) && d == 0 {
				departure = 1
			}
			reviewObservation(t, s, ctx, r, summary, i, week.AddDate(0, 0, d), departure)
		}
	}
	detail, err = s.ListContributionReviews(ctx, reviewFilter(r, summary, week), now)
	if err != nil || detail[0].Flagged || detail[0].DepartureRejoins != 7 || detail[0].QualifiedPassages != 25 || detail[0].ContributorScopes != 5 || detail[0].DepartureScopes != 3 {
		t.Fatalf("low share: %+v %v", detail, err)
	}
	list, err := s.ListContributionReviews(ctx, ContributionReviewFilter{}, now)
	if err != nil || len(list) != 0 {
		t.Fatalf("low sample task: %d %v", len(list), err)
	}
}
func TestContributionReviewSeparatesModeDirectionRevisionAndUTCWeek(t *testing.T) {
	s, ctx, _, r, _, summary := contributionFixture(t)
	now := time.Now()
	week := ContributionReviewWindow(now)
	for i := 1; i <= 4; i++ {
		reviewObservation(t, s, ctx, r, summary, i, now, 1)
	}
	bike := summary
	bike.Mode = models.TravelModeBike
	reviewObservation(t, s, ctx, r, bike, 100, now, 1)
	// Separate week cannot supply the fifth contributor for this week.
	reviewObservation(t, s, ctx, r, summary, 101, week.AddDate(0, 0, -1), 1)
	otherDirection := summary
	if summary.Direction == "forward" {
		otherDirection.Direction = "reverse"
	} else {
		otherDirection.Direction = "forward"
	}
	reviewObservation(t, s, ctx, r, otherDirection, 102, now, 1)
	var otherRevision string
	err := s.pool.QueryRow(ctx, `WITH identity AS (SELECT gen_random_uuid() id) INSERT INTO tour_revisions(id,tour_id,number,draft_version,snapshot)
    SELECT identity.id,r.tour_id,r.number+10,r.draft_version,jsonb_set(jsonb_set(r.snapshot,'{tour,revision_id}',to_jsonb(identity.id::text)),'{tour,revision_number}',to_jsonb(r.number+10)) FROM tour_revisions r,identity WHERE r.id=$1 RETURNING id`, r.ID).Scan(&otherRevision)
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.GetTourRevision(ctx, r.TourID, otherRevision)
	if err != nil {
		t.Fatal(err)
	}
	reviewObservation(t, s, ctx, next, summary, 103, now, 1)
	list, err := s.ListContributionReviews(ctx, ContributionReviewFilter{}, now)
	if err != nil || len(list) != 0 {
		t.Fatalf("dimensions combined: %d %v", len(list), err)
	}
	detail, err := s.ListContributionReviews(ctx, reviewFilter(r, summary, week), now)
	if err != nil || detail[0].ContributorScopes != 4 || detail[0].Flagged {
		t.Fatalf("separate group: %+v %v", detail, err)
	}
	bad := reviewFilter(r, summary, week.AddDate(0, 0, -28))
	if _, err := s.ListContributionReviews(ctx, bad, now); err != ErrInvalidReference {
		t.Fatalf("unbounded old window: %v", err)
	}
	bad = reviewFilter(r, summary, week.AddDate(0, 0, 1))
	if _, err := s.ListContributionReviews(ctx, bad, now); err != ErrInvalidReference {
		t.Fatalf("non-Monday window: %v", err)
	}
}
