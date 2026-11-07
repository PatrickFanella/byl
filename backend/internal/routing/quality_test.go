package routing

import (
	"slices"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
)

func qualityFixture() (*models.TourDetail, Route, *models.PathAttribution, []models.PathInspection) {
	tour := controlledTour(syntheticPath[:3], 0, 2)
	source := tour.Corridor.Intent.Source
	var segments []models.CorridorSegment
	var path []models.CorridorTraversal
	var inspections []models.PathInspection
	var edges []models.PathEdge
	for i, id := range []string{"trail", "connector"} {
		kind, use := "trail", "footway"
		if i == 1 {
			kind, use = "road_connector", "road"
		}
		a, b := syntheticPath[i], syntheticPath[i+1]
		segments = append(segments, models.CorridorSegment{ID: id, Kind: kind, FromNode: []string{"a", "b"}[i], ToNode: []string{"b", "c"}[i], Branches: []string{"branch"}, Modes: models.AllTravelModes, Geometry: models.ReferenceLine{Type: "LineString", Coordinates: [][]float64{{a.Lng, a.Lat}, {b.Lng, b.Lat}}}, Source: &source, Network: &models.CorridorNetwork{Ways: []models.NetworkWay{{ID: int64(i + 11)}}, FromNodeID: int64(i + 1), ToNodeID: int64(i + 2)}, Access: []models.CorridorAccess{{Mode: models.TravelModeWalk, Status: "allowed"}, {Mode: models.TravelModeBike, Status: "allowed"}}})
		path = append(path, models.CorridorTraversal{SegmentID: id})
		for _, purpose := range []string{"network", "access"} {
			inspections = append(inspections, models.PathInspection{SegmentID: id, Purpose: purpose, Mode: models.TravelModeWalk, EvidenceID: "synthetic-inspection-" + id + purpose, URL: "https://example.org/fixture", ContentSHA256: strings.Repeat("a", 64), RetrievedAt: time.Unix(1, 0), Reference: "p1", Quote: "Synthetic inspected evidence."})
		}
		edges = append(edges, models.PathEdge{ID: int64(i + 1), WayID: int64(i + 11), BeginNodeID: int64(i + 1), EndNodeID: int64(i + 2), Forward: true, Use: use, Traversability: "both", TravelMode: "pedestrian", BeginShapeIndex: i, EndShapeIndex: i + 1})
	}
	tour.Corridor.Intent.Segments = segments
	tour.Corridor.Intent.Routes = []models.ModeCorridor{{Mode: models.TravelModeWalk, Branch: "branch", Direction: "fixture order", Path: path, AllowedConnectors: []string{"connector"}}}
	geometry := [][2]float64{{syntheticPath[0].Lng, syntheticPath[0].Lat}, {syntheticPath[1].Lng, syntheticPath[1].Lat}, {syntheticPath[2].Lng, syntheticPath[2].Lat}}
	route := Route{Provider: "valhalla", Legs: []Leg{{Geometry: geometry}}}
	attribution := &models.PathAttribution{Provider: "valhalla", Method: "trace_attributes/edge_walk", Legs: []models.AttributedLeg{{Geometry: geometry, Edges: edges, OSMChangeset: 1}}}
	return tour, route, attribution, inspections
}

func TestPathQualityLawfulConnectorNeedsIdentityConnectivityAndInspection(t *testing.T) {
	tour, route, attribution, inspections := qualityFixture()
	report := AssessPathQuality(tour, models.TravelModeWalk, route, attribution, inspections, "exact-input-hash", time.Unix(10, 0))
	if report.Status != "pass" || len(report.Issues) != 0 || report.InputHash != "exact-input-hash" || len(report.Inspections) != 4 {
		t.Fatalf("lawful fixture %+v", report)
	}
}

func TestPathQualityFindings(t *testing.T) {
	for _, tc := range []struct {
		name, code, status string
		edit               func(*models.TourDetail, *Route, *models.PathAttribution, *[]models.PathInspection)
	}{
		{"parallel road within identical geometry", "unexpected_network_way", "blocked", func(_ *models.TourDetail, _ *Route, a *models.PathAttribution, _ *[]models.PathInspection) {
			a.Legs[0].Edges[0].WayID = 999
			a.Legs[0].Edges[0].Use = "road"
		}},
		{"bridge without junction", "grade_separated_crossing", "blocked", func(_ *models.TourDetail, _ *Route, a *models.PathAttribution, _ *[]models.PathInspection) {
			a.Legs[0].Edges[1].BeginNodeID = 88
			a.Legs[0].Edges[1].Bridge = true
		}},
		{"missing selected connector", "missing_connector", "blocked", func(tour *models.TourDetail, _ *Route, _ *models.PathAttribution, _ *[]models.PathInspection) {
			tour.Corridor.Intent.Routes[0].AllowedConnectors = nil
		}},
		{"ambiguous access", "access_unknown", "needs_research", func(tour *models.TourDetail, _ *Route, _ *models.PathAttribution, _ *[]models.PathInspection) {
			tour.Corridor.Intent.Segments[0].Access[0].Status = "unknown"
		}},
		{"uninspected source URL", "access_not_inspected", "needs_research", func(_ *models.TourDetail, _ *Route, _ *models.PathAttribution, proof *[]models.PathInspection) {
			*proof = (*proof)[:1]
		}},
		{"known closure", "known_closure", "blocked", func(tour *models.TourDetail, _ *Route, _ *models.PathAttribution, _ *[]models.PathInspection) {
			tour.Corridor.Intent.Segments[0].Access[0].Status = "closed"
		}},
		{"mode forbidden by reference", "forbidden_mode", "blocked", func(tour *models.TourDetail, _ *Route, _ *models.PathAttribution, _ *[]models.PathInspection) {
			tour.Corridor.Intent.Segments[0].Modes = []models.TravelMode{models.TravelModeBike}
		}},
		{"wrong provider mode", "provider_mode_forbidden", "blocked", func(_ *models.TourDetail, _ *Route, a *models.PathAttribution, _ *[]models.PathInspection) {
			a.Legs[0].Edges[0].TravelMode = "bicycle"
		}},
		{"development geometry", "development_geometry", "blocked", func(_ *models.TourDetail, r *Route, _ *models.PathAttribution, _ *[]models.PathInspection) {
			r.Provider = "straightline"
		}},
		{"missing provider node", "provider_identity_missing", "needs_research", func(_ *models.TourDetail, _ *Route, a *models.PathAttribution, _ *[]models.PathInspection) {
			a.Legs[0].Edges[0].BeginNodeID = 0
		}},
		{"reference junction absent", "source_junction_not_on_path", "blocked", func(tour *models.TourDetail, _ *Route, _ *models.PathAttribution, _ *[]models.PathInspection) {
			tour.Corridor.Intent.Segments[0].Network.ToNodeID = 99
			tour.Corridor.Intent.Segments[1].Network.FromNodeID = 99
		}},
		{"exact attribution unavailable", "strict_attribution_unavailable", "needs_research", func(_ *models.TourDetail, _ *Route, a *models.PathAttribution, _ *[]models.PathInspection) {
			a.Failure = "exact_match_unavailable"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tour, route, a, proof := qualityFixture()
			tc.edit(tour, &route, a, &proof)
			report := AssessPathQuality(tour, models.TravelModeWalk, route, a, proof, "hash", time.Unix(10, 0))
			if report.Status != tc.status || !slices.ContainsFunc(report.Issues, func(i models.PathQualityIssue) bool { return i.Code == tc.code }) {
				t.Fatalf("wanted %s/%s; got %+v", tc.status, tc.code, report)
			}
		})
	}
}

func TestPathQualityDoesNotHideRoadUseInsideOneWay(t *testing.T) {
	tour, route, attribution, inspections := qualityFixture()
	a, b := route.Legs[0].Geometry[0], route.Legs[0].Geometry[1]
	midpoint := [2]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}
	geometry := [][2]float64{a, midpoint, b, route.Legs[0].Geometry[2]}
	route.Legs[0].Geometry = geometry
	attribution.Legs[0].Geometry = geometry
	first := attribution.Legs[0].Edges[0]
	first.EndNodeID = 99
	road := first
	road.ID = 99
	road.BeginNodeID = 99
	road.EndNodeID = 2
	road.BeginShapeIndex = 1
	road.EndShapeIndex = 2
	road.Use = "road"
	connector := attribution.Legs[0].Edges[1]
	connector.BeginShapeIndex = 2
	connector.EndShapeIndex = 3
	attribution.Legs[0].Edges = []models.PathEdge{first, road, connector}
	report := AssessPathQuality(tour, models.TravelModeWalk, route, attribution, inspections, "hash", time.Unix(10, 0))
	if report.Status != "blocked" || !slices.ContainsFunc(report.Issues, func(i models.PathQualityIssue) bool { return i.Code == "unapproved_road_departure" }) {
		t.Fatalf("mixed-use way hid road departure %+v", report)
	}
}
