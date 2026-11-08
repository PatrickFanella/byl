package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/auth"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

var contributionHTTPKey = []byte("0123456789abcdef0123456789abcdef")

func admitContributionHTTPFixture(t *testing.T, e *testEnv) {
	t.Helper()
	if err := e.store.AdmitContributionCustody(context.Background(), contributionHTTPKey, time.Now().AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}
}

func (e *testEnv) contributionPost(t *testing.T, path, token, key string, payload any) resp {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Body.Close() }()
	raw, err = io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	out := resp{status: r.StatusCode, raw: raw, header: r.Header}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.body)
	}
	return out
}

func contributionHTTPFixture(t *testing.T) (*testEnv, string, models.ContributionConsentRequest, models.ContributionSummary) {
	t.Helper()
	e := newTestEnvWith(t, func(d *Deps) { d.Contributions = true; d.ContributionKey = contributionHTTPKey })
	admitContributionHTTPFixture(t, e)
	e.drain(t)
	curator := e.signIn(t, "contribution-curator@example.com", models.AccountRoleCurator)
	tour := e.get(t, "/api/v1/admin/tours/"+testdb.SeedTourID, curator).body
	e.approveCorridorFixtureArrivals(t, curator, tour)
	intent := corridorRequest(t, tour, 0)
	// The topology fixture is not access evidence. Add an explicit synthetic
	// assertion solely for these owned database/API persistence tests.
	for _, raw := range intent["intent"].(map[string]any)["segments"].([]any) {
		raw.(map[string]any)["access"] = []any{map[string]any{"mode": "walk", "status": "allowed", "notes": "Synthetic contribution fixture"}, map[string]any{"mode": "bike", "status": "allowed", "notes": "Synthetic contribution fixture"}}
	}
	proposal := e.do(t, "POST", "/api/v1/admin/tours/"+testdb.SeedTourID+"/corridors", curator, intent)
	if proposal.status != 201 {
		t.Fatalf("reference fixture: %d %s", proposal.status, proposal.raw)
	}
	review := e.do(t, "POST", "/api/v1/admin/tours/"+testdb.SeedTourID+"/corridors/"+proposal.body["id"].(string)+"/review", curator, map[string]any{"version": 0, "action": "approve", "note": "Synthetic contribution reference fixture"})
	if review.status != 200 {
		t.Fatalf("reference review fixture: %d %s", review.status, review.raw)
	}
	e.drain(t)
	e.publishFixture(t, testdb.SeedTourID)
	ctx := context.Background()
	working, err := e.store.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.store.GetTourRevision(ctx, working.ID, *working.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	day := models.Date{Time: models.ContributionDay(time.Now())}
	no := false
	zero := 0
	grant := models.ContributionConsentRequest{Version: 1, Accepted: true, TourID: r.TourID, RevisionID: r.ID, Mode: models.TravelModeWalk, RequestedOn: &day}
	summary := models.ContributionSummary{SchemaVersion: 1, DetectorVersion: "uncalibrated-v1", Calibrated: &no, RevisionID: r.ID, Mode: models.TravelModeWalk, Direction: "forward", ObservationPeriod: &day, QualifiedPassages: 1, SustainedDepartureRejoins: &zero}
	for _, segment := range r.Snapshot.Routes[models.TravelModeWalk].Journey.Segments {
		if segment.Access == "allowed" {
			summary.SegmentID = segment.ID
			if segment.Reverse {
				summary.Direction = "reverse"
			}
			break
		}
	}
	if summary.SegmentID == "" {
		t.Fatal("fixture has no retained eligible segment")
	}
	owner := e.signIn(t, "contribution-owner@example.com", "")
	return e, owner, grant, summary
}

func TestContributionHTTPDefaultsOffWithoutChangingNavigation(t *testing.T) {
	e := newTestEnv(t)
	capability := e.get(t, "/api/v1/route-contribution-intake", "")
	if capability.status != 200 || capability.body["enabled"] != false || capability.header.Get("Cache-Control") != "no-store" {
		t.Fatal("default gate/cache")
	}
	owner := e.signIn(t, "contribution-off@example.com", "")
	for _, path := range []string{"/api/v1/me/route-contributions/consent", "/api/v1/me/route-contributions"} {
		if r := e.contributionPost(t, path, owner, strings.Repeat("k", 20), map[string]any{}); r.status != 403 || r.body["error"].(map[string]any)["code"] != "contributions_disabled" {
			t.Fatalf("disabled gate: %d %s", r.status, r.raw)
		}
		if r := e.contributionPost(t, path, "", strings.Repeat("k", 20), map[string]any{}); r.status != 401 {
			t.Fatalf("anonymous: %d", r.status)
		}
	}
	var count int
	if err := e.store.Pool().QueryRow(context.Background(), `SELECT count(*) FROM route_contribution_consents`).Scan(&count); err != nil || count != 0 {
		t.Fatal("navigation or gate created consent")
	}
	if r := e.get(t, "/api/v1/cities", ""); r.status != 200 {
		t.Fatal("collection gate blocked browsing")
	}
}

func TestContributionHTTPRestoreClosesIntakeDespiteEnabledConfiguration(t *testing.T) {
	e := newTestEnvWith(t, func(d *Deps) { d.Contributions = true; d.ContributionKey = contributionHTTPKey })
	owner := e.signIn(t, "restored-contribution-owner@example.com", "")
	assertClosed := func() {
		t.Helper()
		if r := e.get(t, "/api/v1/route-contribution-intake", ""); r.status != 200 || r.body["enabled"] != false {
			t.Fatalf("unadmitted intake opened: %d %s", r.status, r.raw)
		}
		if r := e.contributionPost(t, "/api/v1/me/route-contributions/consent", owner, strings.Repeat("key", 8), map[string]any{}); r.status != 403 {
			t.Fatalf("unadmitted collection accepted: %d", r.status)
		}
	}
	assertClosed()
	if err := e.store.AdmitContributionCustody(context.Background(), contributionHTTPKey, time.Now()); err != nil {
		t.Fatal(err)
	}
	assertClosed()
	if _, err := e.store.Pool().Exec(context.Background(), `UPDATE route_contribution_custody SET admitted_on=admitted_on-2`); err != nil {
		t.Fatal(err)
	}
	if r := e.get(t, "/api/v1/route-contribution-intake", ""); r.status != 200 || r.body["enabled"] != true {
		t.Fatalf("ready admission stayed closed: %d %s", r.status, r.raw)
	}
	if _, err := e.store.Pool().Exec(context.Background(), `DELETE FROM route_contribution_custody`); err != nil {
		t.Fatal(err)
	}
	assertClosed()
	if r := e.get(t, "/api/v1/cities", ""); r.status != 200 {
		t.Fatal("restore gate blocked browsing")
	}
}

func TestContributionHTTPConsentReplayErasureAndOwnerBoundary(t *testing.T) {
	e, owner, grant, summary := contributionHTTPFixture(t)
	key := strings.Repeat("grant", 4)
	c := e.contributionPost(t, "/api/v1/me/route-contributions/consent", owner, key, grant)
	if c.status != 201 || len(c.body) != 3 {
		t.Fatalf("grant: %d %s", c.status, c.raw)
	}
	if r := e.contributionPost(t, "/api/v1/me/route-contributions/consent", owner, key, grant); r.status != 200 || string(r.raw) != string(c.raw) {
		t.Fatalf("grant replay: %d %s", r.status, r.raw)
	}
	var consent models.ContributionConsent
	if err := json.Unmarshal(c.raw, &consent); err != nil {
		t.Fatal(err)
	}
	in := models.ContributionRequest{ConsentID: consent.ID, Summary: summary}
	foreign := e.signIn(t, "contribution-foreign@example.com", "")
	if r := e.contributionPost(t, "/api/v1/me/route-contributions", foreign, strings.Repeat("summary", 3), in); r.status != 404 {
		t.Fatalf("foreign: %d %s", r.status, r.raw)
	}
	saved := e.contributionPost(t, "/api/v1/me/route-contributions", owner, strings.Repeat("summary", 3), in)
	if saved.status != 201 || len(saved.body) != 2 || saved.header.Get("Cache-Control") != "no-store" || !strings.Contains(saved.header.Get("Vary"), "Authorization") {
		t.Fatalf("private receipt: %d %s", saved.status, saved.raw)
	}
	if r := e.contributionPost(t, "/api/v1/me/route-contributions", owner, strings.Repeat("summary", 3), in); r.status != 200 || r.body["id"] != saved.body["id"] {
		t.Fatalf("summary replay: %d", r.status)
	}
	if r := e.contributionPost(t, "/api/v1/me/route-contributions", owner, strings.Repeat("different", 3), in); r.status != 409 || r.body["error"].(map[string]any)["code"] != "already_contributed" {
		t.Fatalf("day dedup: %d %s", r.status, r.raw)
	}
	wrong := models.ContributionWithdrawal{ConsentID: consent.ID, DeletionHandle: strings.Repeat("0", 64)}
	if r := e.contributionPost(t, "/api/v1/route-contributions/withdraw", "", "", wrong); r.status != 404 {
		t.Fatalf("wrong delete capability: %d", r.status)
	}
	// Keep capability erasure available while the collection gate is closed and
	// after sign-out. It authenticates the random scoped capability, not a URL token.
	resolver := auth.NewSessions(e.store, time.Hour)
	offServer := httptest.NewServer(NewRouter(Deps{Store: e.store, Sessions: resolver}))
	t.Cleanup(offServer.Close)
	off := *e
	off.srv = offServer
	withdraw := models.ContributionWithdrawal{ConsentID: consent.ID, DeletionHandle: consent.DeletionHandle}
	for range 2 {
		if r := off.contributionPost(t, "/api/v1/route-contributions/withdraw", "", "", withdraw); r.status != 204 {
			t.Fatalf("closed/signed-out erasure: %d %s", r.status, r.raw)
		}
	}
	if r := e.contributionPost(t, "/api/v1/me/route-contributions", owner, strings.Repeat("summary", 3), in); r.status != 403 || r.body["error"].(map[string]any)["code"] != "consent_required" {
		t.Fatalf("withdrawn upload: %d %s", r.status, r.raw)
	}
	var count int
	if err := e.store.Pool().QueryRow(context.Background(), `SELECT count(*) FROM route_contributions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("erasure retained contributions")
	}
}

func TestContributionHTTPRejectsRawPreciseOrUnexpectedFieldsWithoutLoggingPayload(t *testing.T) {
	var logs bytes.Buffer
	e := newTestEnvWith(t, func(d *Deps) {
		d.Contributions = true
		d.ContributionKey = contributionHTTPKey
		d.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	})
	admitContributionHTTPFixture(t, e)
	owner := e.signIn(t, "contribution-redaction@example.com", "")
	const marker = "PRIVATE-CONTRIBUTION-TRACE-MARKER"
	for _, payload := range []any{
		map[string]any{"consent_id": marker, "trace": []any{map[string]any{"lat": 41.8, "lng": -88.0, "timestamp": marker}}},
		map[string]any{"consent_id": marker, "summary": map[string]any{"observation_period": "2026-11-08T00:00:00Z"}},
		map[string]any{"consent_id": marker, "summary": map[string]any{"timestamp": marker}},
		map[string]any{"consent_id": marker, "summary": map[string]any{"account_id": marker}},
		map[string]any{"consent_id": marker, "unexpected_" + marker: true},
		map[string]any{"consent_id": strings.Repeat(marker, 300)},
	} {
		r := e.contributionPost(t, "/api/v1/me/route-contributions", owner, strings.Repeat("key", 8), payload)
		if r.status != 400 {
			t.Fatalf("raw/precise input accepted: %d", r.status)
		}
		if strings.Contains(string(r.raw), marker) {
			t.Fatal("rejected content reflected in response")
		}
	}
	if r := e.contributionPost(t, "/api/v1/me/route-contributions?deletion_handle="+marker, owner, strings.Repeat("key", 8), map[string]any{}); r.status != 400 || strings.Contains(string(r.raw), marker) {
		t.Fatal("query capability accepted/reflected")
	}
	if strings.Contains(logs.String(), marker) || strings.Contains(logs.String(), owner) {
		t.Fatal("private payload or bearer token entered application logs")
	}
	var count int
	if err := e.store.Pool().QueryRow(context.Background(), `SELECT count(*) FROM route_contributions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected data retained")
	}
}

func TestContributionHTTPExplicitConsentAndReplayHeaderAreRequired(t *testing.T) {
	e, owner, grant, summary := contributionHTTPFixture(t)
	missingChoice := grant
	missingChoice.Accepted = false
	if r := e.contributionPost(t, "/api/v1/me/route-contributions/consent", owner, strings.Repeat("key", 8), missingChoice); r.status != 422 {
		t.Fatalf("implicit consent: %d", r.status)
	}
	for _, key := range []string{"", "short", "contains spaces in key"} {
		if r := e.contributionPost(t, "/api/v1/me/route-contributions/consent", owner, key, grant); r.status != 422 {
			t.Fatalf("missing/invalid retry key: %d", r.status)
		}
	}
	if r := e.contributionPost(t, "/api/v1/me/route-contributions", owner, strings.Repeat("key", 8), map[string]any{}); r.status != 422 {
		t.Fatalf("missing summary: %d", r.status)
	}
	summary.SustainedDepartureRejoins = nil
	if r := e.contributionPost(t, "/api/v1/me/route-contributions", owner, strings.Repeat("key", 8), models.ContributionRequest{Summary: summary}); r.status != 422 {
		t.Fatalf("missing deviation counter became zero: %d", r.status)
	}
}
