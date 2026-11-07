package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
)

func controlledTour(path []Point, stops ...int) *models.TourDetail {
	source := models.CorridorSource{URL: "https://example.org/reference", Title: "Synthetic route", Publisher: "Byl tests", Version: "1", RetrievedOn: "2026-11-07", License: "CC0", LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/"}
	segment := models.CorridorSegment{ID: "part", Kind: "trail", FromNode: "a", ToNode: "z", Branches: []string{"branch"}, Modes: models.AllTravelModes, Geometry: models.ReferenceLine{Type: "LineString"}}
	for _, p := range path {
		segment.Geometry.Coordinates = append(segment.Geometry.Coordinates, []float64{p.Lng, p.Lat})
	}
	if path[0] == path[len(path)-1] {
		segment.ToNode = segment.FromNode
	}
	tour := &models.TourDetail{TourSummary: models.TourSummary{Modes: models.AllTravelModes}, Corridor: &models.ApprovedCorridor{Intent: &models.CorridorIntent{Key: "synthetic", Name: "Synthetic path", Source: source, Segments: []models.CorridorSegment{segment}}}}
	for _, mode := range models.AllTravelModes {
		tour.Corridor.Intent.Routes = append(tour.Corridor.Intent.Routes, models.ModeCorridor{Mode: mode, Branch: "branch", Direction: "selected order", Path: []models.CorridorTraversal{{SegmentID: "part"}}})
	}
	for i, index := range stops {
		point := path[index]
		tour.Stops = append(tour.Stops, models.TourStop{ID: fmt.Sprintf("stop-%d", i+1), Position: i + 1, Place: models.PlaceSummary{Latitude: point.Lat + 0.01, Longitude: point.Lng + 0.01, ArrivalPoint: &models.ApprovedArrival{ID: fmt.Sprintf("arrival-%d", i+1), Version: 1, Point: models.ArrivalPoint{Latitude: &point.Lat, Longitude: &point.Lng, Access: []models.ArrivalAccess{{Mode: models.TravelModeWalk, Status: "allowed"}, {Mode: models.TravelModeBike, Status: "allowed"}}}}}})
	}
	return tour
}

var syntheticPath = []Point{{41, -88}, {41.001, -88}, {41.001, -87.999}, {41.002, -87.999}, {41.002, -87.998}, {41.003, -87.998}, {41.003, -87.997}}

func TestApprovedControlsKeepStoryBoundaries(t *testing.T) {
	tour := controlledTour(syntheticPath, 0, 4, 6)
	for _, index := range []int{1, 3} {
		p := syntheticPath[index]
		tour.Corridor.Intent.Routes[0].ShapingPoints = append(tour.Corridor.Intent.Routes[0].ShapingPoints, models.ShapingPoint{ID: fmt.Sprint(index), SegmentID: "part", TraversalIndex: 0, AfterStopID: "stop-1", BeforeStopID: "stop-2", Latitude: &p.Lat, Longitude: &p.Lng})
	}
	plan, err := BuildPlan(tour, models.TravelModeWalk)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Controlled || len(plan.Legs) != 2 || len(plan.Controls) != 7 || plan.Legs[0].ToID != "stop-2" || plan.Controls[0].Point != syntheticPath[0] {
		t.Fatalf("plan %+v", plan)
	}
	for i, point := range plan.Controls {
		if point.Break != (i == 0 || i == 4 || i == 6) {
			t.Fatalf("control %d became a narrated stop: %+v", i, point)
		}
	}
	assertProviderPlan(t, plan, models.TravelModeWalk)
}

func TestControlledReturnPaths(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprint(closed), func(t *testing.T) {
			path := syntheticPath[:3]
			if closed {
				path = []Point{syntheticPath[0], syntheticPath[1], syntheticPath[2], {Lat: 41, Lng: -87.999}, syntheticPath[0]}
			}
			tour := controlledTour(path, 0, 2)
			if !closed {
				for i := range tour.Corridor.Intent.Routes {
					tour.Corridor.Intent.Routes[i].Path = append(tour.Corridor.Intent.Routes[i].Path, models.CorridorTraversal{SegmentID: "part", Reverse: true})
				}
			}
			plan, err := BuildPlan(tour, models.TravelModeBike)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Legs) != 2 || !plan.Legs[1].Return || plan.Legs[1].From != 2 || plan.Legs[1].To != 1 || plan.Controls[len(plan.Controls)-1].Point != plan.Controls[0].Point {
				t.Fatalf("return %+v", plan)
			}
			route := assertProviderPlan(t, plan, models.TravelModeBike)
			out := distance(path[0], path[1]) + distance(path[1], path[2])
			if !closed && math.Abs(route.DistanceM-2*out) > 1 {
				t.Fatalf("out-and-back lost return distance %.2f want %.2f", route.DistanceM, 2*out)
			}
		})
	}
}

func TestControlledInputFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*models.TourDetail)
		mode models.TravelMode
		want string
	}{
		{"unknown arrival", func(tour *models.TourDetail) { tour.Stops[1].Place.ArrivalPoint = nil }, models.TravelModeWalk, "arrival_unknown"},
		{"restricted bike", func(tour *models.TourDetail) { tour.Stops[1].Place.ArrivalPoint.Point.Access[1].Status = "restricted" }, models.TravelModeBike, "arrival_restricted"},
		{"missing mode", func(tour *models.TourDetail) { tour.Corridor.Intent.Routes = tour.Corridor.Intent.Routes[:1] }, models.TravelModeBike, "corridor_mode_unavailable"},
		{"missing segment", func(tour *models.TourDetail) { tour.Corridor.Intent.Routes[0].Path[0].SegmentID = "missing" }, models.TravelModeWalk, "invalid_corridor_intent"},
		{"unattached end", func(tour *models.TourDetail) { *tour.Stops[2].Place.ArrivalPoint.Point.Latitude += 1 }, models.TravelModeWalk, "corridor_end_not_at_arrival"},
		{"ambiguous repeated stop", func(tour *models.TourDetail) {
			tour.Corridor.Intent.Routes[0].Path = append(tour.Corridor.Intent.Routes[0].Path, models.CorridorTraversal{SegmentID: "part", Reverse: true})
		}, models.TravelModeWalk, "ambiguous_or_missing_stop_occurrence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tour := controlledTour(syntheticPath, 0, 3, 6)
			tc.edit(tour)
			if _, err := BuildPlan(tour, tc.mode); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v want %s", err, tc.want)
			}
		})
	}
	// A bike restriction cannot disable the independently approved walking path.
	tour := controlledTour(syntheticPath, 0, 6)
	tour.Stops[1].Place.ArrivalPoint.Point.Access[1].Status = "closed"
	if _, err := BuildPlan(tour, models.TravelModeWalk); err != nil {
		t.Fatal(err)
	}
	if _, ok := NewStraightLine().(ControlledRouter); ok {
		t.Fatal("development provider must not advertise approved navigation")
	}
}

func TestShapingPointsMustFollowTravelAndStayBetweenStories(t *testing.T) {
	for _, indices := range [][]int{{3, 1}, {0}, {6}} {
		tour := controlledTour(syntheticPath, 0, 6)
		for i, index := range indices {
			p := syntheticPath[index]
			tour.Corridor.Intent.Routes[0].ShapingPoints = append(tour.Corridor.Intent.Routes[0].ShapingPoints, models.ShapingPoint{ID: fmt.Sprint(i), SegmentID: "part", TraversalIndex: 0, AfterStopID: "stop-1", BeforeStopID: "stop-2", Latitude: &p.Lat, Longitude: &p.Lng})
		}
		if _, err := BuildPlan(tour, models.TravelModeWalk); err == nil {
			t.Fatalf("invalid shaping order accepted: %v", indices)
		}
	}
}

func TestReferenceControlLimitRequiresExplicitReview(t *testing.T) {
	var path []Point
	for i := range maxControls + 1 {
		path = append(path, Point{Lat: 41 + float64(i)*0.00001, Lng: -88})
	}
	if _, err := BuildPlan(controlledTour(path, 0, len(path)-1), models.TravelModeWalk); err == nil || !strings.Contains(err.Error(), "corridor_control_limit") {
		t.Fatalf("oversized reference was silently simplified: %v", err)
	}
}

func TestEntrancesWithoutReferenceAllowCurvedNetworkPath(t *testing.T) {
	controls := []Waypoint{{Point: syntheticPath[0], Break: true}, {Point: syntheticPath[6], Break: true}}
	geometry := make([][2]float64, 0, len(syntheticPath))
	d := 0.0
	for i, p := range syntheticPath {
		geometry = append(geometry, [2]float64{p.Lng, p.Lat})
		if i > 0 {
			d += distance(syntheticPath[i-1], p)
		}
	}
	route := Route{DistanceM: d, DurationS: 500, Legs: []Leg{{DistanceM: d, DurationS: 500, Geometry: geometry}}}
	if err := validateControlledRoute(controls, route, false); err != nil {
		t.Fatalf("entrance-only route was forced into an imaginary straight corridor: %v", err)
	}
	if err := validateControlledRoute(controls, route, true); err == nil {
		t.Fatal("reference-constrained route left its control envelope")
	}
}

func assertProviderPlan(t *testing.T, plan Plan, mode models.TravelMode) Route {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Costing   string `json:"costing"`
			Locations []struct {
				Lat, Lon             float64
				Type                 string
				Radius, SearchCutoff int `json:"-"`
			} `json:"locations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		want := "pedestrian"
		if mode == models.TravelModeBike {
			want = "bicycle"
		}
		if req.Costing != want || len(req.Locations) != len(plan.Controls) {
			t.Errorf("wrong request %+v", req)
		}
		var legs []map[string]any
		var points []Point
		total := 0.0
		for i, location := range req.Locations {
			if (location.Type == "break") != plan.Controls[i].Break {
				t.Errorf("break %d", i)
			}
			points = append(points, Point{location.Lat, location.Lon})
			if i > 0 && location.Type == "break" {
				length := 0.0
				for j := 1; j < len(points); j++ {
					length += distance(points[j-1], points[j])
				}
				total += length
				legs = append(legs, map[string]any{"shape": encodePolyline(points), "summary": map[string]any{"length": length / 1000, "time": length / 1.3}})
				points = points[len(points)-1:]
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"trip": map[string]any{"legs": legs, "summary": map[string]any{"length": total / 1000, "time": total / 1.3}}})
	}))
	defer srv.Close()
	route, err := NewValhalla(srv.URL, srv.Client()).(ControlledRouter).RouteControlled(context.Background(), mode, ControlledRequest{Controls: plan.Controls, FollowReference: plan.FollowReference})
	if err != nil {
		t.Fatal(err)
	}
	if len(route.Legs) != len(plan.Legs) {
		t.Fatalf("provider created shaping legs: %+v", route)
	}
	return route
}

func encodePolyline(points []Point) string {
	var out strings.Builder
	var lat, lng int64
	write := func(value int64) {
		value <<= 1
		if value < 0 {
			value = ^value
		}
		for value >= 0x20 {
			out.WriteByte(byte((0x20 | (value & 0x1f)) + 63))
			value >>= 5
		}
		out.WriteByte(byte(value + 63))
	}
	for _, p := range points {
		a, b := int64(math.Round(p.Lat*1e6)), int64(math.Round(p.Lng*1e6))
		write(a - lat)
		write(b - lng)
		lat, lng = a, b
	}
	return out.String()
}

func TestControlledProviderRejectsMissingAndShortcutGeometry(t *testing.T) {
	controls := []Waypoint{{Point: syntheticPath[0], Break: true}, {Point: syntheticPath[1]}, {Point: syntheticPath[2], Break: true}}
	for _, tc := range []struct {
		name     string
		status   int
		response any
	}{
		{"no path", 400, map[string]any{"error": "No path could be found"}},
		{"provider limit", 400, map[string]any{"error": "Exceeded max locations: 10"}},
		{"missing leg", 200, map[string]any{"trip": map[string]any{"legs": []any{}}}},
		{"shortcut", 200, map[string]any{"trip": map[string]any{"summary": map[string]any{"length": 0.15, "time": 100}, "legs": []any{map[string]any{"shape": encodePolyline([]Point{syntheticPath[0], syntheticPath[2]}), "summary": map[string]any{"length": 0.15, "time": 100}}}}}},
		{"missing shape", 200, map[string]any{"trip": map[string]any{"summary": map[string]any{"length": 0.2, "time": 100}, "legs": []any{map[string]any{"summary": map[string]any{"length": 0.2, "time": 100}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			defer srv.Close()
			if _, err := NewValhalla(srv.URL, srv.Client()).(ControlledRouter).RouteControlled(context.Background(), models.TravelModeWalk, ControlledRequest{Controls: controls, FollowReference: true}); err == nil || calls != 1 {
				t.Fatalf("failure must be explicit, without fallback/retry: %v calls%d", err, calls)
			}
		})
	}
}
