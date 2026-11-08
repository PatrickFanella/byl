package repository

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

func TestRouteInvestigationGroupsExactScopeAndRetainsPrivateInputs(t *testing.T) {
	s, ctx, owner, revision := revisionFixture(t)
	makeReport := func(account, details, url string, mode models.TravelMode) *models.RouteReport {
		t.Helper()
		in := reportInput(revision)
		in.Mode, in.Details, in.SourceURL = mode, details, url
		r, _, err := s.CreateRouteReport(ctx, account, in, "")
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	other, err := s.UpsertAccount(ctx, models.AuthProviderDev, "investigation-other", "other@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	third, err := s.UpsertAccount(ctx, models.AuthProviderDev, "investigation-third", "third@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	seed := makeReport(owner, "Observed crossing", "https://operator.example/path#one", models.TravelModeWalk)
	sameOwner := makeReport(owner, "Another account-local report", "", models.TravelModeWalk)
	sharedSource := makeReport(other.ID, "Copied operator notice", "https://OPERATOR.example:443/path#two", models.TravelModeWalk)
	sharedText := makeReport(third.ID, "observed  CROSSING", "", models.TravelModeWalk)
	wrongMode := makeReport(owner, "Other mode", "", models.TravelModeBike)
	related, err := s.RelatedRouteReports(ctx, seed.ID, 100, 0)
	if err != nil || len(related) != 3 {
		t.Fatalf("related reports %+v %v", related, err)
	}
	request := models.RouteInvestigationRequest{Version: 1, RelatedReportIDs: []string{sameOwner.ID, sharedSource.ID, sharedText.ID}}
	type outcome struct {
		investigation *models.RouteInvestigation
		err           error
	}
	completed := make(chan outcome, 2)
	for range 2 {
		go func() {
			i, err := s.QueueRouteInvestigation(ctx, seed.ID, owner, request, "same-action")
			completed <- outcome{i, err}
		}()
	}
	first, second := <-completed, <-completed
	if first.err != nil || second.err != nil || first.investigation.ID != second.investigation.ID {
		t.Fatalf("concurrent retry duplicated an investigation: %v %v", first.err, second.err)
	}
	i := first.investigation
	if len(i.Input.Observations) != 4 || len(i.Input.EvidenceIDs) != 1 || i.Input.Observations[0].Report.ID != seed.ID {
		t.Fatal("missing selected observations or source deduplication")
	}
	for _, o := range i.Input.Observations[1:] {
		if o.DuplicateOf != seed.ID {
			t.Fatalf("duplicate treated as corroboration %+v", o)
		}
	}
	again, err := s.QueueRouteInvestigation(ctx, seed.ID, owner, request, "same-action")
	if err != nil || again.ID != i.ID {
		t.Fatal("idempotent request created another investigation")
	}
	bad := request
	bad.RelatedReportIDs = []string{wrongMode.ID}
	if _, err := s.QueueRouteInvestigation(ctx, seed.ID, owner, bad, "same-action"); !errors.Is(err, ErrConflict) {
		t.Fatalf("key conflict %v", err)
	}
	if _, err := s.QueueRouteInvestigation(ctx, seed.ID, owner, bad, ""); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("cross-mode grouping %v", err)
	}
	bad.RelatedReportIDs = []string{seed.ID}
	if _, err := s.QueueRouteInvestigation(ctx, seed.ID, owner, bad, ""); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("self grouping %v", err)
	}
	bad.RelatedReportIDs = nil
	bad.Version = 99
	if _, err := s.QueueRouteInvestigation(ctx, seed.ID, owner, bad, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version %v", err)
	}
	if _, err := s.GetRouteInvestigation(ctx, wrongMode.ID, i.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("investigation cross-linked to wrong report")
	}
	list, err := s.ListRouteInvestigations(ctx, seed.ID, 50, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	// Source jobs contain only record IDs, never submitted text or source URLs.
	jobs, err := s.ListJobs(ctx, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if strings.Contains(string(j.Payload), "operator.example") || strings.Contains(string(j.Payload), "Observed crossing") {
			t.Fatal("private input copied into job payload")
		}
	}
	// Checkpoint claims cannot be forged or replaced after the first completion.
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET status='running',attempts=2 WHERE id=$1`, i.JobID); err != nil {
		t.Fatal(err)
	}
	result := models.RouteInvestigationResult{Status: "needs_research", StoriesPreserved: true, CheckedAt: time.Now()}
	if err := s.CompleteRouteInvestigation(ctx, i.ID, &models.Job{ID: i.JobID, Attempts: 1}, result); !errors.Is(err, ErrResearchInactive) {
		t.Fatalf("stale worker %v", err)
	}
	if err := s.CompleteRouteInvestigation(ctx, i.ID, &models.Job{ID: i.JobID, Attempts: 2}, result); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`UPDATE route_investigations SET input='{}' WHERE id=$1`, `UPDATE route_investigations SET result='{}' WHERE id=$1`, `DELETE FROM route_investigations WHERE id=$1`} {
		if _, err := s.Pool().Exec(ctx, query, i.ID); err == nil {
			t.Fatal("private checkpoint was mutable")
		}
	}
	if _, err := s.SetRouteReportState(ctx, seed.ID, models.BriefStateRequest{Version: 1, Status: models.BriefDeclined, Reason: "New disposition after investigation"}); err != nil {
		t.Fatal(err)
	}
	again, err = s.QueueRouteInvestigation(ctx, seed.ID, owner, request, "same-action")
	if err != nil || again.ID != i.ID || again.Input.Observations[0].Report.Version != 1 {
		t.Fatal("retry replaced captured report version")
	}
	// Quota/validation failure must leave no partial brief, job or investigation.
	var countsBefore, countsAfter string
	query := `SELECT json_build_array((SELECT count(*) FROM jobs),(SELECT count(*) FROM route_briefs),(SELECT count(*) FROM route_investigations))::text`
	if err := s.Pool().QueryRow(ctx, query).Scan(&countsBefore); err != nil {
		t.Fatal(err)
	}
	tooMany := models.RouteInvestigationRequest{Version: 2, Research: &models.RouteBriefInput{Topic: "Leads", Area: "Here"}}
	for j := 0; j < 20; j++ {
		tooMany.Research.SourceLinks = append(tooMany.Research.SourceLinks, "https://source.example/"+strings.Repeat("x", j+1))
	}
	if _, err := s.QueueRouteInvestigation(ctx, seed.ID, owner, tooMany, ""); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("unbounded sources %v", err)
	}
	if err := s.Pool().QueryRow(ctx, query).Scan(&countsAfter); err != nil || countsBefore != countsAfter {
		t.Fatal("failed queue left partial side effects")
	}
}

func TestInvestigationObservationDuplicateGroupsAreTransitive(t *testing.T) {
	reports := []models.RouteReport{
		{ID: "a", OwnerAccountID: "first", Submitted: models.RouteReportInput{Details: "One", SourceURL: "https://source.example/one"}},
		{ID: "b", OwnerAccountID: "second", Submitted: models.RouteReportInput{Details: "Two", SourceURL: "https://source.example/two"}},
		{ID: "c", OwnerAccountID: "second", Submitted: models.RouteReportInput{Details: "Three", SourceURL: "https://source.example/one#reference"}},
	}
	groups := observationGroups(reports)
	if groups[1].DuplicateOf != "a" || groups[2].DuplicateOf != "a" {
		t.Fatal("bridging duplicate split into independent groups")
	}
	raw, _ := json.Marshal(groups)
	if !strings.Contains(string(raw), "Three") {
		t.Fatal("duplicate grouping discarded original submission")
	}
}
