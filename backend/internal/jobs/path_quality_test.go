package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

// This provider implements the real worker contract over synthetic identities.
// It is not a claim about any actual trail, access permission or live provider.
type qualifiedJobRouter struct{ controlledJobRouter }

func (r *qualifiedJobRouter) RouteControlled(ctx context.Context, mode models.TravelMode, request routing.ControlledRequest) (routing.Route, error) {
	route, err := r.controlledJobRouter.RouteControlled(ctx, mode, request)
	route.Provider = "valhalla"
	return route, err
}
func (r *qualifiedJobRouter) InspectPath(_ context.Context, mode models.TravelMode, route routing.Route) (*models.PathAttribution, error) {
	a := &models.PathAttribution{Provider: "valhalla", Method: "trace_attributes/edge_walk", Legs: []models.AttributedLeg{}}
	for i, leg := range route.Legs {
		use, travel := "footway", "pedestrian"
		if mode == models.TravelModeBike {
			use, travel = "cycleway", "bicycle"
		}
		a.Legs = append(a.Legs, models.AttributedLeg{Geometry: leg.Geometry, OSMChangeset: 1, Edges: []models.PathEdge{{ID: int64(i + 1), WayID: int64(i + 11), BeginNodeID: int64(i + 1), EndNodeID: int64(i + 2), Forward: true, Use: use, TravelMode: travel, Traversability: "both", BeginShapeIndex: 0, EndShapeIndex: len(leg.Geometry) - 1}}})
	}
	return a, nil
}

func TestInspectedPathWorkerRequiresExplicitReleaseAndRetainsClosureFinding(t *testing.T) {
	w, s, ctx, e := evidenceSetup(t)
	const quote = "Synthetic ways 11, 12 and 13 connect nodes 1, 2, 3 and 4. Walking and cycling are allowed."
	w.Evidence = evidenceFunc(func(context.Context, string, string) (models.EvidenceResult, error) {
		result := evidenceResult()
		result.RetrievedAt = time.Now().UTC()
		result.Passages = []models.EvidencePassage{{Reference: "paragraph:1", Text: quote}}
		return result, nil
	})
	drain(t, w, ctx)
	approveJobArrivals(t, s, ctx, "allowed")
	tour, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	original := *tour.RevisionID
	author, err := s.UpsertAccount(ctx, models.AuthProviderDev, "quality-reviewer", "quality@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	source := models.CorridorSource{URL: "https://example.org/synthetic-network", Title: "Synthetic network and access", Publisher: "Fixture", Version: "1", RetrievedOn: "2026-11-07", License: "CC0", LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/"}
	intent := &models.CorridorIntent{Key: "synthetic-quality", Name: "Synthetic qualification fixture", Source: source}
	refs := []models.CorridorEvidenceRef{}
	path := []models.CorridorTraversal{}
	for i := 1; i < len(tour.Stops); i++ {
		a, b := tour.Stops[i-1].Place.ArrivalPoint.Point, tour.Stops[i].Place.ArrivalPoint.Point
		id := fmt.Sprintf("segment-%d", i)
		intent.Segments = append(intent.Segments, models.CorridorSegment{ID: id, Kind: "trail", FromNode: fmt.Sprint(i), ToNode: fmt.Sprint(i + 1), Branches: []string{"fixture"}, Modes: models.AllTravelModes, Geometry: models.ReferenceLine{Type: "LineString", Coordinates: [][]float64{{*a.Longitude, *a.Latitude}, {*b.Longitude, *b.Latitude}}}, Network: &models.CorridorNetwork{Ways: []models.NetworkWay{{ID: int64(i + 10)}}, FromNodeID: int64(i), ToNodeID: int64(i + 1)}, Access: []models.CorridorAccess{{Mode: models.TravelModeWalk, Status: "allowed"}, {Mode: models.TravelModeBike, Status: "allowed"}}})
		path = append(path, models.CorridorTraversal{SegmentID: id})
		refs = append(refs, models.CorridorEvidenceRef{SegmentID: id, Purpose: "network", EvidenceID: e.ID, Reference: "paragraph:1", Quote: quote})
		for _, mode := range models.AllTravelModes {
			refs = append(refs, models.CorridorEvidenceRef{SegmentID: id, Purpose: "access", Mode: mode, EvidenceID: e.ID, Reference: "paragraph:1", Quote: quote})
		}
	}
	for _, mode := range models.AllTravelModes {
		intent.Routes = append(intent.Routes, models.ModeCorridor{Mode: mode, Branch: "fixture", Direction: "synthetic story order", Path: path, AllowedConnectors: []string{}, ShapingPoints: []models.ShapingPoint{}})
	}
	bad := slices.Clone(refs)
	bad[0].Quote = "This text was never inspected."
	if _, err := s.ProposeCorridor(ctx, tour.ID, author.ID, models.CorridorProposalRequest{Intent: intent, QualityEvidence: bad}); !errors.Is(err, repository.ErrInvalidReference) {
		t.Fatalf("invented quote accepted: %v", err)
	}
	p, err := s.ProposeCorridor(ctx, tour.ID, author.ID, models.CorridorProposalRequest{Intent: intent, QualityEvidence: refs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCorridor(ctx, tour.ID, p.ID, author.ID, models.ArrivalReviewRequest{Action: "approve", Note: "Synthetic inspected source review"}); err != nil {
		t.Fatal(err)
	}
	router := &qualifiedJobRouter{controlledJobRouter{Router: routing.NewStraightLine()}}
	w.router = router
	drain(t, w, ctx)
	working, err := s.GetTour(ctx, tour.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if *working.RevisionID != original {
		t.Fatal("qualification auto-published")
	}
	for _, mode := range models.AllTravelModes {
		r, err := s.GetPathQualityReport(ctx, tour.ID, mode, "")
		if err != nil || r.Status != "pass" || !r.InputCurrent || len(r.Inspections) != 9 {
			t.Fatalf("worker checkpoint %+v %v", r, err)
		}
	}
	released, err := s.PublishTourRevision(ctx, tour.ID, author.ID, models.PublishRevisionRequest{DraftVersion: working.DraftVersion, CurrentRevisionID: &original})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(released)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), quote) || strings.Contains(string(raw), e.ID) || !strings.Contains(string(raw), "approved_navigation") {
		t.Fatal("private source leaked or frozen approved navigation missing")
	}
	frozen := string(raw)
	intent.Segments[0].Access[0].Status = "closed"
	p, err = s.ProposeCorridor(ctx, tour.ID, author.ID, models.CorridorProposalRequest{Version: 1, Intent: intent, QualityEvidence: refs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCorridor(ctx, tour.ID, p.ID, author.ID, models.ArrivalReviewRequest{Version: 1, Action: "approve", Note: "Synthetic closure"}); err != nil {
		t.Fatal(err)
	}
	calls := len(router.calls)
	drain(t, w, ctx)
	if len(router.calls) != calls+1 {
		t.Fatal("provider was called for the known closed walking path")
	}
	report, err := s.GetPathQualityReport(ctx, tour.ID, models.TravelModeWalk, "")
	if err != nil || report.Status != "blocked" || !slices.ContainsFunc(report.Issues, func(i models.PathQualityIssue) bool { return i.Code == "known_closure" && i.SegmentID == "segment-1" }) {
		t.Fatalf("closure checkpoint %+v %v", report, err)
	}
	old, err := s.GetTourRevision(ctx, tour.ID, released.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(old)
	if err != nil || string(raw) != frozen {
		t.Fatal("closure rewrote published history")
	}
}
