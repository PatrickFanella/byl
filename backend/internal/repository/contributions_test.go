package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

var contributionTestKey = []byte("0123456789abcdef0123456789abcdef")

func contributionFixture(t *testing.T) (*Store, context.Context, string, *models.TourRevision, models.ContributionConsentRequest, models.ContributionSummary) {
	t.Helper()
	s, ctx, actor, original := revisionFixture(t)
	if err := s.AdmitContributionCustody(ctx, contributionTestKey, time.Now().AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}
	intent := corridorInput(t, &original.Snapshot.Tour)
	// Explicit synthetic access supports persistence tests without promoting the
	// portable topology-only fixture into evidence of real-world access.
	for i := range intent.Segments {
		intent.Segments[i].Access = []models.CorridorAccess{{Mode: models.TravelModeWalk, Status: "allowed", Notes: "Synthetic contribution fixture"}, {Mode: models.TravelModeBike, Status: "allowed", Notes: "Synthetic contribution fixture"}}
	}
	bike := intent.Routes[0]
	bike.Mode = models.TravelModeBike
	intent.Routes = append(intent.Routes, bike)
	p, err := s.ProposeCorridor(ctx, original.TourID, actor, models.CorridorProposalRequest{Intent: intent})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReviewCorridor(ctx, original.TourID, p.ID, actor, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic contribution reference fixture"}); err != nil {
		t.Fatal(err)
	}
	readyFixtureRoutes(t, s, ctx, original.TourID)
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.PublishTourRevision(ctx, working.ID, actor, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: working.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	day := models.Date{Time: models.ContributionDay(time.Now())}
	no := false
	zero := 0
	grant := models.ContributionConsentRequest{Version: 1, Accepted: true, TourID: r.TourID, RevisionID: r.ID, Mode: models.TravelModeWalk, RequestedOn: &day}
	summary := models.ContributionSummary{SchemaVersion: 1, DetectorVersion: "uncalibrated-v1", Calibrated: &no, RevisionID: r.ID, Mode: models.TravelModeWalk, Direction: "forward", ObservationPeriod: &day, QualifiedPassages: 1, SustainedDepartureRejoins: &zero}
	for _, seg := range r.Snapshot.Routes[models.TravelModeWalk].Journey.Segments {
		if seg.Access == "allowed" {
			summary.SegmentID = seg.ID
			if seg.Reverse {
				summary.Direction = "reverse"
			}
			break
		}
	}
	if summary.SegmentID == "" {
		t.Fatal("fixture has no allowed retained segment")
	}
	return s, ctx, actor, r, grant, summary
}

func grantContribution(t *testing.T, s *Store, ctx context.Context, actor string, in models.ContributionConsentRequest, key string) *models.ContributionConsent {
	t.Helper()
	c, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, in, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestContributionConsentReplayScopesAndCoarseStorage(t *testing.T) {
	s, ctx, actor, _, in, _ := contributionFixture(t)
	key := strings.Repeat("grant-a", 4)
	c, created, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, in, key, time.Now())
	if err != nil || !created || len(c.DeletionHandle) != 64 {
		t.Fatalf("consent: %v", err)
	}
	retry, created, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, in, key, time.Now().AddDate(0, 0, 3))
	if err != nil || created || *retry != *c {
		t.Fatalf("retained consent replay: %v", err)
	}
	if _, _, err = s.GrantContributionConsent(ctx, contributionTestKey, actor, in, strings.Repeat("stale", 4), time.Now().AddDate(0, 0, 3)); !errors.Is(err, ErrContributionConsent) {
		t.Fatalf("stale new grant: %v", err)
	}
	bike := in
	bike.Mode = models.TravelModeBike
	other := grantContribution(t, s, ctx, actor, bike, strings.Repeat("grant-b", 4))
	var firstScope, otherScope string
	if err = s.pool.QueryRow(ctx, `SELECT owner_scope FROM route_contribution_consents WHERE id=$1`, c.ID).Scan(&firstScope); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT owner_scope FROM route_contribution_consents WHERE id=$1`, other.ID).Scan(&otherScope); err != nil {
		t.Fatal(err)
	}
	if firstScope == otherScope || strings.Contains(firstScope, actor) {
		t.Fatal("owner identifier was reused across modes")
	}
	var forbidden int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name IN ('route_contribution_consents','route_contributions','route_contribution_limits') AND (column_name IN ('account_id','device_id','trip_id','created_at','coordinates','timestamp') OR data_type='timestamp with time zone')`).Scan(&forbidden); err != nil || forbidden != 0 {
		t.Fatalf("unexpected precise/owner columns: %d %v", forbidden, err)
	}
}

func TestContributionReplayDailyDedupWithdrawalAndFreshChoice(t *testing.T) {
	s, ctx, actor, _, grant, summary := contributionFixture(t)
	consent := grantContribution(t, s, ctx, actor, grant, strings.Repeat("grant", 4))
	in := models.ContributionRequest{ConsentID: consent.ID, Summary: summary}
	key := strings.Repeat("summary", 3)
	r, created, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, time.Now())
	if err != nil || !created {
		t.Fatalf("create: %v", err)
	}
	retry, created, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, time.Now())
	if err != nil || created || *r != *retry {
		t.Fatalf("replay: %v", err)
	}
	changed := in
	one := 1
	changed.Summary.SustainedDepartureRejoins = &one
	if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, changed, key, time.Now()); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed key replay: %v", err)
	}
	if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, in, strings.Repeat("another", 3), time.Now()); !errors.Is(err, ErrContributionDuplicate) {
		t.Fatalf("duplicate UTC day: %v", err)
	}
	second := grantContribution(t, s, ctx, actor, grant, strings.Repeat("second", 3))
	in.ConsentID = second.ID
	if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, in, strings.Repeat("third", 4), time.Now()); !errors.Is(err, ErrContributionDuplicate) {
		t.Fatalf("cross-grant duplicate: %v", err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT COALESCE(sum(qualified_passages),0) FROM route_contribution_counts`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("internal counts: %d %v", count, err)
	}
	wrong := models.ContributionWithdrawal{ConsentID: consent.ID, DeletionHandle: strings.Repeat("0", 64)}
	if err = s.WithdrawRouteContributions(ctx, wrong, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong deletion capability: %v", err)
	}
	withdraw := models.ContributionWithdrawal{ConsentID: consent.ID, DeletionHandle: consent.DeletionHandle}
	if err = s.WithdrawRouteContributions(ctx, withdraw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT COALESCE(sum(qualified_passages),0) FROM route_contribution_counts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("count repair: %d %v", count, err)
	}
	if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, time.Now()); !errors.Is(err, ErrContributionConsent) {
		t.Fatalf("revoked sibling consent: %v", err)
	}
	if _, _, err = s.GrantContributionConsent(ctx, contributionTestKey, actor, grant, strings.Repeat("grant", 4), time.Now()); !errors.Is(err, ErrContributionConsent) {
		t.Fatalf("revoked grant replay: %v", err)
	}
	fresh := grantContribution(t, s, ctx, actor, grant, strings.Repeat("fresh", 4))
	in.ConsentID = fresh.ID
	if err = s.WithdrawRouteContributions(ctx, withdraw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, created, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, in, strings.Repeat("fresh-data", 2), time.Now()); err != nil || !created {
		t.Fatalf("old delete replay revoked fresh choice: %v", err)
	}
}

func TestContributionConsentOwnershipReferencesAndPeriod(t *testing.T) {
	s, ctx, actor, _, grant, summary := contributionFixture(t)
	consent := grantContribution(t, s, ctx, actor, grant, strings.Repeat("grant", 4))
	foreign, err := s.UpsertAccount(ctx, models.AuthProviderDev, "foreign-contribution", "foreign-contribution@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	in := models.ContributionRequest{ConsentID: consent.ID, Summary: summary}
	if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, foreign.ID, in, strings.Repeat("key", 8), time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign scope: %v", err)
	}
	for _, change := range []func(*models.ContributionRequest){
		func(i *models.ContributionRequest) { i.Summary.Mode = models.TravelModeBike },
		func(i *models.ContributionRequest) { i.Summary.SegmentID = "not-retained" },
		func(i *models.ContributionRequest) {
			if i.Summary.Direction == "forward" {
				i.Summary.Direction = "reverse"
			} else {
				i.Summary.Direction = "forward"
			}
		},
		func(i *models.ContributionRequest) { i.Summary.RevisionID = "00000000-0000-4000-8000-000000000001" },
	} {
		bad := in
		change(&bad)
		if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, bad, strings.Repeat("invalid", 3), time.Now()); !errors.Is(err, ErrInvalidReference) {
			t.Fatalf("invalid scope accepted: %v", err)
		}
	}
	before := in
	day := models.Date{Time: models.ContributionDay(time.Now()).AddDate(0, 0, -1)}
	before.Summary.ObservationPeriod = &day
	if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, before, strings.Repeat("before", 3), time.Now()); !errors.Is(err, ErrContributionConsent) {
		t.Fatalf("pre-consent day: %v", err)
	}
}

func TestContributionRetentionHidesExpiredCountsAndPhysicallyPurges(t *testing.T) {
	s, ctx, actor, _, grant, summary := contributionFixture(t)
	consent := grantContribution(t, s, ctx, actor, grant, strings.Repeat("grant", 4))
	if _, _, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, models.ContributionRequest{ConsentID: consent.ID, Summary: summary}, strings.Repeat("summary", 3), time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`UPDATE route_contribution_consents SET created_on=created_on-31,requested_on=requested_on-31,expires_on=expires_on-31`,
		`UPDATE route_contributions SET observation_period=observation_period-31,expires_on=expires_on-31`,
		`UPDATE route_contribution_limits SET expires_on=expires_on-31`,
	} {
		if _, err := s.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM route_contribution_counts`).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired rows remained in counts")
	}
	if _, err := s.PurgeRouteContributions(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"route_contribution_consents", "route_contributions", "route_contribution_limits"} {
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("retained expired %s: %v", table, err)
		}
	}
	if _, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, grant, strings.Repeat("grant", 4), time.Now().AddDate(0, 0, 31)); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("expired request resurrected consent: %v", err)
	}
}

func TestContributionConcurrentReplayAndWithdrawalCannotResurrectData(t *testing.T) {
	s, ctx, actor, _, grant, summary := contributionFixture(t)
	consent := grantContribution(t, s, ctx, actor, grant, strings.Repeat("grant", 4))
	in := models.ContributionRequest{ConsentID: consent.ID, Summary: summary}
	key := strings.Repeat("summary", 3)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, time.Now())
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM route_contributions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent duplicate count %d %v", count, err)
	}
	results = make(chan error, 13)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, time.Now())
			if err != nil && !errors.Is(err, ErrContributionConsent) {
				results <- err
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		results <- s.WithdrawRouteContributions(ctx, models.ContributionWithdrawal{ConsentID: consent.ID, DeletionHandle: consent.DeletionHandle}, time.Now())
	}()
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM route_contributions`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("withdrawal retained/resurrected data %d %v", count, err)
	}
}

func TestContributionConsentQuotaDoesNotCountExactReplays(t *testing.T) {
	s, ctx, actor, _, in, _ := contributionFixture(t)
	for i := range 10 {
		grantContribution(t, s, ctx, actor, in, fmt.Sprintf("consent-replay-key-%04d", i))
	}
	if _, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, in, "consent-replay-key-0010", time.Now()); !errors.Is(err, ErrSubmissionLimit) {
		t.Fatalf("missing consent quota: %v", err)
	}
	if _, created, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, in, "consent-replay-key-0000", time.Now()); err != nil || created {
		t.Fatalf("replay quota: %v", err)
	}
}

func TestContributionMigrationRetainsExistingRowsAndHashes(t *testing.T) {
	assertNavigationMigrationPreservesRows(t, 18)
}

func TestContributionSummaryQuotaAndFreeExactReplay(t *testing.T) {
	s, ctx, actor, _, grant, summary := contributionFixture(t)
	consent := grantContribution(t, s, ctx, actor, grant, strings.Repeat("grant", 4))
	today := models.ContributionDay(time.Now())
	limitScope := contributionMAC(contributionTestKey, "route-contribution-limit-v1", actor, "summary", today.Format("2006-01-02"))
	if _, err := s.pool.Exec(ctx, `INSERT INTO route_contribution_limits(scope,count,expires_on) VALUES($1,50,$2)`, limitScope, today.AddDate(0, 0, 2)); err != nil {
		t.Fatal(err)
	}
	in := models.ContributionRequest{ConsentID: consent.ID, Summary: summary}
	key := strings.Repeat("summary", 3)
	if _, _, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, today); !errors.Is(err, ErrSubmissionLimit) {
		t.Fatalf("summary quota not enforced: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE route_contribution_limits SET count=49 WHERE scope=$1`, limitScope); err != nil {
		t.Fatal(err)
	}
	if _, created, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, today); err != nil || !created {
		t.Fatalf("last quota slot: %v", err)
	}
	if _, created, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, today); err != nil || created {
		t.Fatalf("exact replay consumed quota: %v", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count FROM route_contribution_limits WHERE scope=$1`, limitScope).Scan(&count); err != nil || count != 50 {
		t.Fatalf("quota counter: %d %v", count, err)
	}
}

func TestContributionExpiredReplayCannotUseOverlappingConsent(t *testing.T) {
	s, ctx, actor, _, grant, summary := contributionFixture(t)
	start := models.ContributionDay(time.Now())
	a, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, grant, strings.Repeat("grant-a", 4), start)
	if err != nil {
		t.Fatal(err)
	}
	next := models.Date{Time: start.AddDate(0, 0, 1)}
	grant.RequestedOn = &next
	b, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, grant, strings.Repeat("grant-b", 4), next.Time)
	if err != nil {
		t.Fatal(err)
	}
	summary.ObservationPeriod = &next
	key := strings.Repeat("summary", 3)
	in := models.ContributionRequest{ConsentID: a.ID, Summary: summary}
	if _, _, err = s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, next.Time); err != nil {
		t.Fatal(err)
	}
	in.ConsentID = b.ID
	if receipt, _, err := s.CreateRouteContribution(ctx, contributionTestKey, actor, in, key, start.AddDate(0, 0, 30)); !errors.Is(err, ErrConflict) || receipt != nil {
		t.Fatalf("expired receipt replayed through overlapping consent: %v %v", receipt, err)
	}
}

func TestContributionCustodyAdmissionHoldRestoreAndKeyRotation(t *testing.T) {
	s, ctx, actor, _, grant, _ := contributionFixture(t)
	today := models.ContributionDay(time.Now())
	if _, err := s.pool.Exec(ctx, `DELETE FROM route_contribution_custody`); err != nil {
		t.Fatal(err)
	}
	assertAdmitted := func(now time.Time, want bool) {
		t.Helper()
		got, err := s.ContributionsAdmitted(ctx, contributionTestKey, now)
		if err != nil || got != want {
			t.Fatalf("admission: got %v want %v: %v", got, want, err)
		}
	}
	assertAdmitted(today, false)
	if _, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, grant, strings.Repeat("grant", 4), today); !errors.Is(err, ErrContributionsDisabled) {
		t.Fatalf("missing admission accepted: %v", err)
	}
	if err := s.AdmitContributionCustody(ctx, contributionTestKey, today); err != nil {
		t.Fatal(err)
	}
	assertAdmitted(today.AddDate(0, 0, 1), false)
	if err := s.AdmitContributionCustody(ctx, contributionTestKey, today.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	ready := today.AddDate(0, 0, 2)
	assertAdmitted(ready, true)
	if _, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, grant, strings.Repeat("stale", 4), ready); !errors.Is(err, ErrContributionConsent) {
		t.Fatalf("pre-restore request resurrected: %v", err)
	}
	fresh := models.Date{Time: ready}
	grant.RequestedOn = &fresh
	if _, _, err := s.GrantContributionConsent(ctx, contributionTestKey, actor, grant, strings.Repeat("fresh", 4), ready); err != nil {
		t.Fatal(err)
	}
	otherKey := []byte("abcdef0123456789abcdef0123456789")
	if err := s.AdmitContributionCustody(ctx, otherKey, ready); !errors.Is(err, ErrConflict) {
		t.Fatalf("rotation admitted with retained consent: %v", err)
	}
	if _, err := s.PurgeRouteContributions(ctx, ready.AddDate(0, 0, 31)); err != nil {
		t.Fatal(err)
	}
	if err := s.AdmitContributionCustody(ctx, otherKey, ready.AddDate(0, 0, 31)); err != nil {
		t.Fatal(err)
	}
	assertAdmitted(ready.AddDate(0, 0, 33), false)
	if got, err := s.ContributionsAdmitted(ctx, otherKey, ready.AddDate(0, 0, 33)); err != nil || !got {
		t.Fatalf("drained rotation: %v %v", got, err)
	}
}
