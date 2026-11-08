package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

// Synthetic mapping output: a shorter crossing, with optional prohibited steps.
type investigationRouter struct {
	routing.Router
	controls []routing.ControlledRequest
	points   [][]routing.Point
	steps    bool
	fail     bool
}

func (r *investigationRouter) Route(ctx context.Context, mode models.TravelMode, points []routing.Point) (routing.Route, error) {
	r.points = append(r.points, slices.Clone(points))
	if r.fail {
		return routing.Route{}, errors.New("private-provider-message")
	}
	path, err := r.Router.Route(ctx, mode, points)
	for j := range path.Legs {
		path.Legs[j].DistanceM /= 2
		path.Legs[j].DurationS /= 2
	}
	return path, err
}

func (r *investigationRouter) RouteControlled(ctx context.Context, mode models.TravelMode, request routing.ControlledRequest) (routing.Route, error) {
	r.controls = append(r.controls, request)
	points := []routing.Point{request.Controls[0].Point, request.Controls[len(request.Controls)-1].Point}
	return r.Route(ctx, mode, points)
}

func (r *investigationRouter) InspectPath(_ context.Context, mode models.TravelMode, path routing.Route) (*models.PathAttribution, error) {
	travel, use := "pedestrian", "footway"
	if mode == models.TravelModeBike {
		travel = "bicycle"
	}
	if r.steps {
		use = "steps"
	}
	a := &models.PathAttribution{Provider: "valhalla", Method: "trace_attributes/edge_walk"}
	for j, leg := range path.Legs {
		a.Legs = append(a.Legs, models.AttributedLeg{Geometry: leg.Geometry, OSMChangeset: 1, Edges: []models.PathEdge{{ID: int64(j + 1), WayID: int64(j + 11), BeginNodeID: int64(j + 1), EndNodeID: int64(j + 2), Forward: true, Use: use, TravelMode: travel, Traversability: "both", EndShapeIndex: len(leg.Geometry) - 1}}})
	}
	return a, nil
}

func investigationFixture(t *testing.T, entrance bool, mode models.TravelMode) (*Worker, *repository.Store, context.Context, *models.RouteReport, *models.TourRevision, *investigationRouter) {
	t.Helper()
	w, s, ctx := newWorker(t, routing.NewStraightLine(), narration.None{})
	drain(t, w, ctx)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "investigation", "investigation@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	working, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	// Persist a synthetic qualified baseline for investigation tests. Provider
	// qualification itself is exercised separately in path_quality_test.go.
	for _, m := range working.Modes {
		route, err := s.GetTourRoute(ctx, working.ID, m)
		if err != nil {
			t.Fatal(err)
		}
		_, digest, err := s.TourRouteInputs(ctx, working.ID, m, working.RouteVersion)
		if err != nil {
			t.Fatal(err)
		}
		route.Provider = "valhalla"
		quality := models.PathQualityReport{PathQualitySummary: models.PathQualitySummary{Version: 1, Status: "pass", Issues: []models.PathQualityIssue{}, CheckedAt: time.Now()}, InputHash: digest, Mode: m, Provider: "valhalla", RouteSHA256: route.RoutingFingerprint(), Inspections: []models.PathInspection{{EvidenceID: "synthetic-persistence-checkpoint"}}, Attribution: &models.PathAttribution{Provider: "valhalla", Method: "trace_attributes/edge_walk", Legs: []models.AttributedLeg{{}}}}
		quality.Attribution.Legs = make([]models.AttributedLeg, len(route.Legs))
		if err := s.UpsertRouteQualified(ctx, *route, working.RouteVersion, digest, quality); err != nil {
			t.Fatal(err)
		}
	}
	working, err = s.GetTour(ctx, working.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := s.PublishTourRevision(ctx, working.ID, a.ID, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: working.RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	in := models.RouteReportInput{TourID: revision.TourID, RevisionID: revision.ID, Mode: mode, Direction: "forward", Reason: "improvement", Better: "A synthetic crossing lead, not an access assertion.", SourceURL: "https://operator.example/crossing"}
	stops := revision.Snapshot.Tour.Stops
	in.Affected = models.ReportLocation{Kind: "segment", FromStopID: stops[0].ID, ToStopID: stops[1].ID}
	path := revision.Snapshot.Routes[mode].Legs[0].Geometry
	in.Alternative = &models.ReportAlternative{Type: "LineString", Coordinates: [][]float64{{path[0][0], path[0][1]}, {(path[0][0]+path[len(path)-1][0])/2 + 0.0001, (path[0][1] + path[len(path)-1][1]) / 2}, {path[len(path)-1][0], path[len(path)-1][1]}}}
	if entrance {
		in.Affected = models.ReportLocation{Kind: "entrance", StopID: stops[1].ID}
		lat, lng := stops[1].Place.Latitude+0.0001, stops[1].Place.Longitude
		in.Annotation, in.Alternative = &models.ReportPoint{Lat: &lat, Lng: &lng}, nil
	}
	r, _, err := s.CreateRouteReport(ctx, a.ID, in, "")
	if err != nil {
		t.Fatal(err)
	}
	router := &investigationRouter{Router: routing.NewStraightLine()}
	w.router = router
	w.Evidence = evidenceFunc(func(context.Context, string, string) (models.EvidenceResult, error) { return evidenceResult(), nil })
	return w, s, ctx, r, revision, router
}

func investigate(t *testing.T, w *Worker, s *repository.Store, ctx context.Context, report *models.RouteReport, research *models.RouteBriefInput) *models.RouteInvestigation {
	t.Helper()
	i, err := s.QueueRouteInvestigation(ctx, report.ID, report.OwnerAccountID, models.RouteInvestigationRequest{Version: report.Version, Research: research}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	drain(t, w, ctx)
	i, err = s.GetRouteInvestigation(ctx, report.ID, i.ID)
	if err != nil || i.Result == nil {
		t.Fatalf("missing result %+v %v", i, err)
	}
	return i
}

func hasInvestigationIssue(i *models.RouteInvestigation, code string) bool {
	return slices.ContainsFunc(i.Result.Issues, func(issue models.PathQualityIssue) bool { return issue.Code == code })
}

func TestRouteInvestigationCrossingRetainsStoriesAndFrozenBefore(t *testing.T) {
	w, s, ctx, report, revision, router := investigationFixture(t, false, models.TravelModeWalk)
	before, _ := json.Marshal(revision)
	// A concurrent working edit must never substitute today's story or geometry.
	if _, err := s.Pool().Exec(ctx, `UPDATE moments SET body='Changed live narration' WHERE id=$1`, testdb.SeedMomentBurlingtonID); err != nil {
		t.Fatal(err)
	}
	i := investigate(t, w, s, ctx, report, nil)
	r := i.Result
	if r.After == nil || r.DistanceChangeM == nil || *r.DistanceChangeM >= 0 || r.DurationChangeS == nil || *r.DurationChangeS >= 0 || len(router.controls) != 1 || !router.controls[0].FollowReference || len(router.controls[0].Controls) != 3 {
		t.Fatalf("crossing comparison %+v", r)
	}
	if len(r.AffectedStopIDs) != 2 || !r.StoriesPreserved || r.IndependentCorroboration || r.Status != "needs_research" || !hasInvestigationIssue(i, "report_controls_unreviewed") || len(r.Evidence) != 1 || r.Evidence[0].Result == nil {
		t.Fatalf("review/provenance %+v", r)
	}
	baselineJSON, _ := json.Marshal(revision.Snapshot.Routes[report.Submitted.Mode])
	retainedJSON, _ := json.Marshal(r.Before)
	if string(baselineJSON) != string(retainedJSON) {
		t.Fatalf("retained baseline changed\nwant %s\ngot %s", baselineJSON, retainedJSON)
	}
	if !reflect.DeepEqual(r.After.Legs[1:], r.Before.Legs[1:]) {
		t.Fatal("unaffected legs changed")
	}
	retained, err := s.GetTourRevision(ctx, revision.TourID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(retained)
	if string(before) != string(after) {
		t.Fatal("investigation rewrote retained stories/narration")
	}
	working, err := s.GetTour(ctx, revision.TourID, false)
	if err != nil || *working.RevisionID != revision.ID {
		t.Fatal("investigation published an alternative")
	}
	jobs, err := s.ListJobs(ctx, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Kind == models.JobKindGenerateNarration || j.Kind == models.JobKindDraftCandidate {
			t.Fatal("path-only investigation changed story workflow")
		}
	}
	// A restart/replayed job reads the immutable result without another provider call.
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET status='queued',run_after=now() WHERE id=$1`, i.JobID); err != nil {
		t.Fatal(err)
	}
	drain(t, w, ctx)
	if len(router.controls) != 1 {
		t.Fatal("completed checkpoint recomputed")
	}
}

func TestRouteInvestigationMissingEntranceComputesBothAdjacentLegs(t *testing.T) {
	w, s, ctx, report, revision, router := investigationFixture(t, true, models.TravelModeWalk)
	i := investigate(t, w, s, ctx, report, nil)
	if i.Result.After == nil || len(router.points) != 2 || len(router.controls) != 0 || len(i.Result.AffectedStopIDs) != 3 {
		t.Fatal("missing adjacent entrance comparison")
	}
	want := routing.Point{Lat: *report.Submitted.Annotation.Lat, Lng: *report.Submitted.Annotation.Lng}
	if router.points[0][1] != want || router.points[1][0] != want {
		t.Fatal("proposed arrival not computed consistently")
	}
	if len(i.Result.Arrivals) != len(revision.Snapshot.Tour.Stops) || i.Result.Arrivals[1].Arrival != nil {
		t.Fatal("unreviewed entrance was promoted")
	}
}

func TestRouteInvestigationIllegalShortcutRemainsBlocked(t *testing.T) {
	w, s, ctx, report, _, router := investigationFixture(t, false, models.TravelModeBike)
	router.steps = true
	i := investigate(t, w, s, ctx, report, nil)
	if i.Result.After == nil || i.Result.Quality == nil || i.Result.Quality.Status != "blocked" || !hasInvestigationIssue(i, "cycling_steps") || i.Result.Status != "needs_research" {
		t.Fatalf("shortcut %+v", i.Result)
	}
	if got, err := s.GetRouteReport(ctx, report.ID, ""); err != nil || got.Status != models.BriefReview || got.Version != 1 {
		t.Fatal("worker auto-disposed report")
	}
}

func TestRouteInvestigationUnavailableNoticeAndNewClaimsUseSourcing(t *testing.T) {
	w, s, ctx, report, _, _ := investigationFixture(t, false, models.TravelModeWalk)
	w.Evidence = evidenceFunc(func(context.Context, string, string) (models.EvidenceResult, error) {
		return models.EvidenceResult{Outcome: "inaccessible", HTTPStatus: 404}, nil
	})
	research := &models.RouteBriefInput{Topic: "An unverified new site", Area: "Synthetic area", PlaceLeads: []string{"A reported site"}, EventLeads: []string{"An unsupported historical claim"}, SourceLinks: []string{"https://operator.example/crossing#notice"}}
	i := investigate(t, w, s, ctx, report, research)
	if !hasInvestigationIssue(i, "source_lookup_failed") || !hasInvestigationIssue(i, "historical_research_required") || len(i.Input.EvidenceIDs) != 1 {
		t.Fatalf("source gap %+v", i.Result)
	}
	b, err := s.GetRouteBrief(ctx, i.Input.BriefID, "")
	if err != nil || b.Status != models.BriefDraft || b.Submitted.PlaceLeads[0] != "A reported site" || b.Submitted.EventLeads[0] != "An unsupported historical claim" {
		t.Fatalf("sourcing brief %+v %v", b, err)
	}
	if i.Result.Evidence[0].BriefID != b.ID || !i.Result.Evidence[0].Result.Untrusted || i.Result.Evidence[0].Result.MediaReuse != "unresolved" {
		t.Fatal("source workflow provenance lost")
	}
}

func TestRouteInvestigationPartialResearchAndFailedRouterLeaveReviewIssue(t *testing.T) {
	w, s, ctx, report, _, router := investigationFixture(t, false, models.TravelModeWalk)
	i, err := s.QueueRouteInvestigation(ctx, report.ID, report.OwnerAccountID, models.RouteInvestigationRequest{Version: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a source job still running elsewhere. Retain a partial checkpoint
	// after bounded investigation attempts, without inventing a source outcome.
	if _, err := s.Pool().Exec(ctx, `UPDATE jobs SET run_after=now()+interval '1 hour' WHERE id<>$1 AND status='queued'`, i.JobID); err != nil {
		t.Fatal(err)
	}
	w.backoff = func(int) time.Duration { return 0 }
	router.fail = true
	drain(t, w, ctx)
	i, err = s.GetRouteInvestigation(ctx, report.ID, i.ID)
	if err != nil || i.Result == nil || i.Result.After != nil || i.Result.DistanceChangeM != nil || !hasInvestigationIssue(i, "source_lookup_pending") || !hasInvestigationIssue(i, "alternative_unavailable") {
		t.Fatalf("partial %+v %v", i, err)
	}
	raw, _ := json.Marshal(i.Result)
	if strings.Contains(string(raw), "private-provider-message") {
		t.Fatal("provider error leaked")
	}
	j, err := s.GetJob(ctx, i.JobID)
	if err != nil || j.Status != models.JobStatusDone || j.Attempts != 3 {
		t.Fatalf("bounded partial job %+v %v", j, err)
	}
}

type investigationResultRouter struct{ result routing.Route }

func (r investigationResultRouter) Route(context.Context, models.TravelMode, []routing.Point) (routing.Route, error) {
	return r.result, nil
}

func TestInvestigationAlternativeSeparatesOutwardAndReturnLegs(t *testing.T) {
	tour := &models.TourDetail{Stops: []models.TourStop{{ID: "a", Position: 1}, {ID: "b", Position: 2}}}
	a, b := [2]float64{-88, 41}, [2]float64{-88.001, 41}
	before := models.TourRoute{Status: models.RouteStatusReady, Provider: "straightline", Legs: []models.RouteLeg{
		{FromPosition: 1, ToPosition: 2, DistanceM: 100, DurationS: 80, Geometry: [][2]float64{a, b}},
		{FromPosition: 2, ToPosition: 1, Return: true, DistanceM: 100, DurationS: 80, Geometry: [][2]float64{b, a}},
	}}
	for _, tc := range []struct {
		direction string
		index     int
	}{{"forward", 0}, {"reverse", 1}, {"unknown", 0}} {
		t.Run(tc.direction, func(t *testing.T) {
			router := &investigationRouter{Router: routing.NewStraightLine()}
			w := &Worker{router: router}
			in := models.RouteReportInput{Mode: models.TravelModeWalk, Direction: tc.direction, Affected: models.ReportLocation{Kind: "segment", FromStopID: "a", ToStopID: "b"}, Alternative: &models.ReportAlternative{Type: "LineString", Coordinates: [][]float64{{a[0], a[1]}, {-88.0003, 41.0001}, {-88.0007, 41.0001}, {b[0], b[1]}}}}
			after, _, err := w.investigationAlternative(context.Background(), before, tour, in)
			if err != nil || after == nil || len(router.controls) != 1 {
				t.Fatalf("directional path %v", err)
			}
			if !reflect.DeepEqual(after.Legs[1-tc.index], before.Legs[1-tc.index]) || after.Legs[tc.index].NavigationSource != "unreviewed_report_proposal" || after.Legs[tc.index].Return != before.Legs[tc.index].Return {
				t.Fatal("report changed the wrong directional leg")
			}
			controls := router.controls[0].Controls
			wantFirst := a
			wantVia := -88.0003
			if tc.index == 1 {
				wantFirst = b
				wantVia = -88.0007
			}
			if len(controls) != 4 || controls[0].Lng != wantFirst[0] || controls[1].Lng != wantVia || !controls[0].Break || controls[1].Break || !controls[len(controls)-1].Break {
				t.Fatal("directional shaping or story breaks changed")
			}
		})
	}
	// A provider without shaping support cannot silently drop the submitted path.
	w := &Worker{router: routing.NewStraightLine()}
	in := models.RouteReportInput{Mode: models.TravelModeWalk, Direction: "forward", Affected: models.ReportLocation{Kind: "segment", FromStopID: "a", ToStopID: "b"}, Alternative: &models.ReportAlternative{Coordinates: [][]float64{{a[0], a[1]}, {b[0], b[1]}}}}
	if after, _, err := w.investigationAlternative(context.Background(), before, tour, in); err == nil || after != nil {
		t.Fatal("unsupported controls silently ignored")
	}
	// A displaced or malformed provider result cannot become a comparison.
	in.Alternative = nil
	for _, leg := range []routing.Leg{
		{Geometry: [][2]float64{{0, 0}, {0, 1}}, DistanceM: 1, DurationS: 1},
		{Geometry: [][2]float64{a, b}, DistanceM: -1, DurationS: 1},
		{Geometry: [][2]float64{a, b}, DistanceM: 1, DurationS: -1},
		{Geometry: [][2]float64{a}, DistanceM: 1, DurationS: 1},
	} {
		w.router = investigationResultRouter{routing.Route{Provider: "fixture", Legs: []routing.Leg{leg}}}
		if after, _, err := w.investigationAlternative(context.Background(), before, tour, in); err == nil || after != nil {
			t.Fatal("invalid provider result accepted")
		}
	}
}
