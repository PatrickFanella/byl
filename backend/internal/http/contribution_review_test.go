package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestContributionReviewHTTPRoleMinimalCountsReplayAndWithdrawal(t *testing.T) {
	e, _, grant, summary := contributionHTTPFixture(t)
	curator := e.signIn(t, "pattern-review-curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "pattern-review-visitor@example.com", models.AccountRoleVisitor)
	path := "/api/v1/admin/route-contribution-reviews"
	if got := e.get(t, path, ""); got.status != 401 {
		t.Fatalf("anonymous %d", got.status)
	}
	if got := e.get(t, path, visitor); got.status != 403 {
		t.Fatalf("visitor %d", got.status)
	}
	var last models.ContributionConsent
	for i := 0; i < 5; i++ {
		token := e.signIn(t, fmt.Sprintf("pattern-%d@example.com", i), models.AccountRoleVisitor)
		key := fmt.Sprintf("synthetic-grant-key-%d", i)
		got := e.contributionPost(t, "/api/v1/me/route-contributions/consent", token, key, grant)
		if got.status != 201 {
			t.Fatalf("grant: %d %s", got.status, got.raw)
		}
		if err := json.Unmarshal(got.raw, &last); err != nil {
			t.Fatal(err)
		}
		departure := 0
		if i < 3 {
			departure = 1
		}
		summary.SustainedDepartureRejoins = &departure
		body := models.ContributionRequest{ConsentID: last.ID, Summary: summary}
		key = fmt.Sprintf("synthetic-summary-key-%d", i)
		if got := e.contributionPost(t, "/api/v1/me/route-contributions", token, key, body); got.status != 201 {
			t.Fatalf("summary: %d %s", got.status, got.raw)
		}
		if got := e.contributionPost(t, "/api/v1/me/route-contributions", token, key, body); got.status != 200 {
			t.Fatalf("replay: %d %s", got.status, got.raw)
		}
	}
	revision, err := e.store.GetTourRevision(context.Background(), grant.TourID, grant.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	input := models.RouteReportInput{TourID: grant.TourID, RevisionID: grant.RevisionID, Mode: grant.Mode, Reason: "blocked", Details: "Synthetic explicit report, section relationship unverified", Affected: models.ReportLocation{Kind: "segment", FromStopID: revision.Snapshot.Tour.Stops[0].ID, ToStopID: revision.Snapshot.Tour.Stops[1].ID}}
	report := e.do(t, "POST", "/api/v1/me/route-reports", visitor, input)
	if report.status != 201 {
		t.Fatalf("explicit report: %d %s", report.status, report.raw)
	}
	got := e.get(t, path, curator)
	if got.status != 200 || got.header.Get("Cache-Control") != "no-store" || !strings.Contains(got.header.Get("Vary"), "Authorization") {
		t.Fatalf("curator/cache: %d", got.status)
	}
	var data struct {
		Items []models.ContributionReview `json:"items"`
	}
	if err := json.Unmarshal(got.raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 1 || data.Items[0].QualifiedPassages != 5 || data.Items[0].DepartureRejoins != 3 || !data.Items[0].Flagged || len(data.Items[0].Reports) != 1 {
		t.Fatalf("counts/report: %s", got.raw)
	}
	for _, private := range []string{"owner_scope", "owner_account_id", "deletion_handle", "consent_id"} {
		if strings.Contains(string(got.raw), private) {
			t.Fatalf("private field: %s", private)
		}
	}
	if got := e.contributionPost(t, "/api/v1/route-contributions/withdraw", "", "", models.ContributionWithdrawal{ConsentID: last.ID, DeletionHandle: last.DeletionHandle}); got.status != 204 {
		t.Fatalf("withdrawal: %d", got.status)
	}
	values := url.Values{"revision_id": {grant.RevisionID}, "segment_id": {summary.SegmentID}, "mode": {string(grant.Mode)}, "direction": {summary.Direction}, "window_start": {repository.ContributionReviewWindow(time.Now()).Format("2006-01-02")}}
	got = e.get(t, path+"?"+values.Encode(), curator)
	if err := json.Unmarshal(got.raw, &data); err != nil {
		t.Fatal(err)
	}
	if got.status != 200 || len(data.Items) != 1 || data.Items[0].Flagged || data.Items[0].ContributorScopes != 4 {
		t.Fatalf("live withdrawn detail: %d %s", got.status, got.raw)
	}
	for _, query := range []string{"?owner_scope=synthetic", "?limit=1&limit=2"} {
		if got := e.get(t, path+query, curator); got.status != 400 {
			t.Fatalf("unexpected query: %d", got.status)
		}
	}
}
