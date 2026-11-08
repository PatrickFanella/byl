package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

func correctionFixture(t *testing.T) (*Store, context.Context, string, *models.RouteReport, *models.RouteInvestigation) {
	t.Helper()
	s, ctx, author, original := revisionFixture(t)
	input := reportInput(original)
	input.Direction = "forward"
	report, _, err := s.CreateRouteReport(ctx, author, input, "")
	if err != nil {
		t.Fatal(err)
	}
	investigation, err := s.QueueRouteInvestigation(ctx, report.ID, author, models.RouteInvestigationRequest{Version: report.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE jobs SET status='running',attempts=1 WHERE id=$1`, investigation.JobID); err != nil {
		t.Fatal(err)
	}
	result := models.RouteInvestigationResult{Status: "needs_research", Before: original.Snapshot.Routes[input.Mode], StoriesPreserved: true, Issues: []models.PathQualityIssue{{Code: "observations_unverified", Severity: "needs_research", Detail: "Synthetic observation still needs curator review"}}, CheckedAt: time.Now().UTC()}
	if err = s.CompleteRouteInvestigation(ctx, investigation.ID, &models.Job{ID: investigation.JobID, Attempts: 1}, result); err != nil {
		t.Fatal(err)
	}
	corridor, err := s.ProposeCorridor(ctx, original.TourID, author, models.CorridorProposalRequest{Intent: corridorInput(t, &original.Snapshot.Tour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReviewCorridor(ctx, original.TourID, corridor.ID, author, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic separately reviewed corrected intent"}); err != nil {
		t.Fatal(err)
	}
	readyFixtureRoutes(t, s, ctx, original.TourID)
	return s, ctx, author, report, investigation
}

func correctionRequest(t *testing.T, s *Store, ctx context.Context, r *models.RouteReport, i *models.RouteInvestigation) models.ApproveRouteCorrectionRequest {
	t.Helper()
	p, err := s.RouteCorrectionComparison(ctx, r.ID, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !p.ReadyToPublish {
		t.Fatalf("prepared comparison blocked: %+v", p.Blockers)
	}
	return models.ApproveRouteCorrectionRequest{Version: p.Version, InvestigationID: i.ID, Note: "Curator reviewed the prepared path, sources, access and retained routing intent."}
}

func TestRouteCorrectionAtomicReleaseRetainsPreviousTripAndIntent(t *testing.T) {
	s, ctx, author, report, investigation := correctionFixture(t)
	original, err := s.GetTourRevision(ctx, report.Submitted.TourID, report.Submitted.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	oldBytes, _ := json.Marshal(original)
	in := correctionRequest(t, s, ctx, report, investigation)
	c, err := s.ApproveRouteCorrection(ctx, report.ID, author, in)
	if err != nil {
		t.Fatal(err)
	}
	if c.RevisionID == original.ID || c.PreviousRevisionID != original.ID || c.CorridorID == "" {
		t.Fatalf("release %+v", c)
	}
	after, err := s.GetTourRevision(ctx, original.TourID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	retained, _ := json.Marshal(after)
	if string(retained) != string(oldBytes) {
		t.Fatal("old active-trip snapshot changed")
	}
	accepted, err := s.GetRouteReport(ctx, report.ID, author)
	if err != nil || accepted.Status != models.BriefAccepted || accepted.Reason != in.Note {
		t.Fatalf("owner status %+v %v", accepted, err)
	}
	again, err := s.ApproveRouteCorrection(ctx, report.ID, author, in)
	if err != nil || again.ID != c.ID {
		t.Fatalf("exact replay %+v %v", again, err)
	}
	changed := in
	changed.Note = "Different review"
	if _, err = s.ApproveRouteCorrection(ctx, report.ID, author, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed replay %v", err)
	}
	for _, query := range []string{`UPDATE route_corrections SET note='rewritten' WHERE id=$1`, `DELETE FROM route_corrections WHERE id=$1`} {
		if _, err = s.pool.Exec(ctx, query, c.ID); err == nil {
			t.Fatal("retained approval changed")
		}
	}
	if err = s.RecomputeRoutes(ctx, original.TourID); err != nil {
		t.Fatal(err)
	}
	working, err := s.GetTour(ctx, original.TourID, false)
	if err != nil || working.Corridor == nil || working.Corridor.ID != c.CorridorID {
		t.Fatalf("recompute lost intent %+v %v", working, err)
	}
	released, err := s.GetTourRevision(ctx, original.TourID, c.RevisionID)
	if err != nil || released.Snapshot.Tour.Corridor.ID != c.CorridorID || released.Snapshot.Routes[models.TravelModeWalk].Status != models.RouteStatusReady {
		t.Fatal("recompute changed released correction")
	}
	if _, err = s.PublishTourRevision(ctx, working.ID, author, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &c.RevisionID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("pending recomputation silently published %v", err)
	}
}

func TestRouteCorrectionRejectsStaleReadSets(t *testing.T) {
	for _, kind := range []string{"draft", "published revision", "report disposition", "route result"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx, author, report, i := correctionFixture(t)
			in := correctionRequest(t, s, ctx, report, i)
			switch kind {
			case "draft":
				title := "Changed prepared title"
				if _, err := s.UpdateTour(ctx, report.Submitted.TourID, UpdateTourInput{Title: &title}); err != nil {
					t.Fatal(err)
				}
			case "published revision":
				p, err := s.RouteCorrectionComparison(ctx, report.ID, i.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.PublishTourRevision(ctx, p.Prepared.Tour.ID, author, models.PublishRevisionRequest{DraftVersion: p.Prepared.Tour.DraftVersion, CurrentRevisionID: &p.Before.ID}); err != nil {
					t.Fatal(err)
				}
			case "report disposition":
				if _, err := s.SetRouteReportState(ctx, report.ID, models.BriefStateRequest{Version: report.Version, Status: models.BriefNeedsResearch, Reason: "Need current observation"}); err != nil {
					t.Fatal(err)
				}
			case "route result":
				route, err := s.GetTourRoute(ctx, report.Submitted.TourID, models.TravelModeWalk)
				if err != nil {
					t.Fatal(err)
				}
				if err = s.UpsertRoute(ctx, *route); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.ApproveRouteCorrection(ctx, report.ID, author, in); !errors.Is(err, ErrConflict) {
				t.Fatalf("stale %s approved: %v", kind, err)
			}
			var n int
			if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM route_corrections`).Scan(&n); err != nil || n != 0 {
				t.Fatal("stale approval left partial release")
			}
		})
	}
}

func TestRouteCorrectionConcurrentApprovalHasOneRelease(t *testing.T) {
	s, ctx, author, report, i := correctionFixture(t)
	in := correctionRequest(t, s, ctx, report, i)
	type outcome struct {
		c   *models.RouteCorrection
		err error
	}
	done := make(chan outcome, 2)
	for range 2 {
		go func() { c, err := s.ApproveRouteCorrection(ctx, report.ID, author, in); done <- outcome{c, err} }()
	}
	a, b := <-done, <-done
	if a.err != nil && b.err != nil {
		t.Fatalf("no release: %v %v", a.err, b.err)
	}
	for _, o := range []outcome{a, b} {
		if o.err != nil && !errors.Is(o.err, ErrConflict) {
			t.Fatal(o.err)
		}
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM route_corrections WHERE report_id=$1`, report.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("concurrent releases %d %v", n, err)
	}
	// A request whose serializable attempt lost can safely replay the exact body.
	if _, err := s.ApproveRouteCorrection(ctx, report.ID, author, in); err != nil {
		t.Fatal(err)
	}
}

func TestRouteCorrectionTemporaryReviewNeverRestoresOldRoute(t *testing.T) {
	s, ctx, author, report, i := correctionFixture(t)
	in := correctionRequest(t, s, ctx, report, i)
	in.Temporary = true
	in.ReviewOn = &models.Date{Time: time.Now().UTC().AddDate(0, 0, 1)}
	c, err := s.ApproveRouteCorrection(ctx, report.ID, author, in)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReviewDue || c.ReviewOn == nil {
		t.Fatal("future detour already due")
	}
	// Move the database's review date only in this isolated test to exercise expiry.
	if _, err = s.pool.Exec(ctx, `ALTER TABLE route_corrections DISABLE TRIGGER retain_route_correction`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE route_corrections SET review_on=CURRENT_DATE-1 WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `ALTER TABLE route_corrections ENABLE TRIGGER retain_route_correction`); err != nil {
		t.Fatal(err)
	}
	due, err := s.ListRouteCorrections(ctx, true, 20, 0)
	if err != nil || len(due) != 1 || !due[0].ReviewDue {
		t.Fatalf("missing reassessment %+v %v", due, err)
	}
	tour, err := s.GetTour(ctx, c.TourID, false)
	if err != nil || tour.RevisionID == nil || *tour.RevisionID != c.RevisionID || tour.Corridor.ID != c.CorridorID {
		t.Fatal("expiry automatically reopened route")
	}
}

func TestRouteCorrectionPrivateImpactUsesSharedStopIdentity(t *testing.T) {
	s, ctx, author, report, i := correctionFixture(t)
	original, err := s.GetTourRevision(ctx, report.Submitted.TourID, report.Submitted.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	shared := original.Snapshot.Tour.Stops[0].Place.ID
	var otherID string
	if err = s.pool.QueryRow(ctx, `INSERT INTO tours(city_id,slug,title,status,modes) SELECT city_id,'shared-correction','Shared reference','draft',modes FROM tours WHERE id=$1 RETURNING id`, original.TourID).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	stops := []models.StopInput{}
	for _, stop := range original.Snapshot.Tour.Stops {
		stops = append(stops, models.StopInput{PlaceID: stop.Place.ID, FeaturedMomentID: stop.FeaturedMomentID})
	}
	if err = s.ReplaceStops(ctx, otherID, stops); err != nil {
		t.Fatal(err)
	}
	readyFixtureRoutes(t, s, ctx, otherID)
	other, err := s.GetTour(ctx, otherID, false)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := s.PublishTourRevision(ctx, otherID, author, models.PublishRevisionRequest{DraftVersion: other.DraftVersion, CurrentRevisionID: other.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.RouteCorrectionComparison(ctx, report.ID, i.ID)
	if err != nil || len(p.PotentiallyAffectedTours) != 1 || p.PotentiallyAffectedTours[0] != otherID {
		t.Fatalf("shared stop %s impact %+v %v", shared, p, err)
	}
	in := correctionRequest(t, s, ctx, report, i)
	c, err := s.ApproveRouteCorrection(ctx, report.ID, author, in)
	if err != nil || len(c.PotentiallyAffectedTours) != 1 {
		t.Fatalf("retained impact %+v %v", c, err)
	}
	untouched, err := s.GetTour(ctx, otherID, false)
	if err != nil || *untouched.RevisionID != revision.ID {
		t.Fatal("correction automatically edited another tour")
	}
	private, _ := json.Marshal(c)
	if !strings.Contains(string(private), otherID) {
		t.Fatal("impact flag lost")
	}
}

func TestRouteCorrectionLateFailureRollsBackReleaseAndStatus(t *testing.T) {
	s, ctx, author, report, i := correctionFixture(t)
	in := correctionRequest(t, s, ctx, report, i)
	var before int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM tour_revisions`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `CREATE FUNCTION reject_correction_acceptance() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected acceptance failure'; END $$; CREATE TRIGGER reject_correction_acceptance BEFORE UPDATE ON route_reports FOR EACH ROW WHEN (NEW.status='accepted') EXECUTE FUNCTION reject_correction_acceptance()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApproveRouteCorrection(ctx, report.ID, author, in); err == nil {
		t.Fatal("injected late failure did not roll back")
	}
	var after, corrections int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM tour_revisions),(SELECT count(*) FROM route_corrections)`).Scan(&after, &corrections); err != nil || after != before || corrections != 0 {
		t.Fatal("partial correction release")
	}
	tour, err := s.GetTour(ctx, report.Submitted.TourID, false)
	if err != nil || *tour.RevisionID != report.Submitted.RevisionID {
		t.Fatal("failed correction changed released pointer")
	}
	current, err := s.GetRouteReport(ctx, report.ID, author)
	if err != nil || current.Status != report.Status || current.Version != report.Version {
		t.Fatal("failed correction changed private report")
	}
}

func TestRouteCorrectionCannotReleaseWithoutRetainedModeIntent(t *testing.T) {
	s, ctx, author, report, i := correctionFixture(t)
	current, err := s.GetTour(ctx, report.Submitted.TourID, false)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := s.ProposeCorridor(ctx, current.ID, author, models.CorridorProposalRequest{Version: current.Corridor.Version, Remove: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReviewCorridor(ctx, current.ID, proposal.ID, author, models.ArrivalReviewRequest{Version: int(proposal.BaseVersion), Action: "approve", Note: "Synthetic removal to check release guard"}); err != nil {
		t.Fatal(err)
	}
	readyFixtureRoutes(t, s, ctx, current.ID)
	p, err := s.RouteCorrectionComparison(ctx, report.ID, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.ReadyToPublish || !strings.Contains(strings.Join(p.Blockers, " "), "durable_routing_intent_required") {
		t.Fatalf("missing persistence guard %+v", p.Blockers)
	}
}

func TestRouteCorrectionMigrationRetainsExistingRowsAndHashes(t *testing.T) {
	assertNavigationMigrationPreservesRows(t, 17)
}
