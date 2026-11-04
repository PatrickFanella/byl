package routing

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"invisible-museum/backend/internal/models"
)

var loop = []Point{{41.7959, -88.0092}, {41.7952, -88.0099}, {41.7944, -88.0118}, {41.8083, -88.0111}}

func TestStraightLineRoute(t *testing.T) {
	r, err := NewStraightLine().Route(context.Background(), models.TravelModeWalk, loop)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Legs) != 3 || len(r.Geometry) != 4 {
		t.Fatalf("legs %d geometry %d", len(r.Legs), len(r.Geometry))
	}
	if r.Geometry[0][0] != -88.0092 || r.Geometry[0][1] != 41.7959 {
		t.Fatalf("geometry must be [lng, lat]: %v", r.Geometry[0])
	}
	if r.DistanceM < 1800 || r.DistanceM > 2200 {
		t.Fatalf("distance %.0f m out of expected range", r.DistanceM)
	}
	if want := int(math.Round(r.DistanceM / 1.3)); abs(r.DurationS-want) > 2 {
		t.Fatalf("walk duration %d want ~%d", r.DurationS, want)
	}
	bike, _ := NewStraightLine().Route(context.Background(), models.TravelModeBike, loop)
	if bike.DurationS >= r.DurationS {
		t.Fatal("cycling should be faster than walking")
	}
	if _, err := NewStraightLine().Route(context.Background(), models.TravelModeWalk, loop[:1]); err == nil {
		t.Fatal("one point is not a route")
	}
}

func TestDecodePolyline6(t *testing.T) {
	// Valhalla documentation sample (precision 6).
	pts := DecodePolyline6("_p~iF~ps|U_ulLnnqC_mqNvxq`@")
	if len(pts) != 3 {
		t.Fatalf("expected 3 points, got %d", len(pts))
	}
	// Google sample point (38.5, -120.2) at precision 5 reads as (3.85, -12.02) at precision 6.
	if math.Abs(pts[0][1]-3.85) > 1e-6 || math.Abs(pts[0][0]+12.02) > 1e-6 {
		t.Fatalf("first point %v", pts[0])
	}
}

func TestValhallaClient(t *testing.T) {
	var gotCosting string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotCosting, _ = req["costing"].(string)
		if r.URL.Path != "/route" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"trip": map[string]any{
			"summary": map[string]any{"length": 1.5, "time": 900.0},
			"units":   "kilometers",
			"legs": []map[string]any{
				{"summary": map[string]any{"length": 1.0, "time": 600.0}, "shape": "_p~iF~ps|U_ulLnnqC_mqNvxq`@"},
				{"summary": map[string]any{"length": 0.5, "time": 300.0}, "shape": "_p~iF~ps|U_ulLnnqC"},
			},
		}})
	}))
	defer srv.Close()
	r, err := NewValhalla(srv.URL, srv.Client()).Route(context.Background(), models.TravelModeBike, loop[:3])
	if err != nil {
		t.Fatal(err)
	}
	if gotCosting != "bicycle" {
		t.Fatalf("costing %q", gotCosting)
	}
	if r.DistanceM != 1500 || r.DurationS != 900 || len(r.Legs) != 2 || r.Legs[0].DistanceM != 1000 || len(r.Legs[0].Geometry) != 3 {
		t.Fatalf("route %+v", r)
	}
	if len(r.Geometry) != 5 {
		t.Fatalf("geometry should concatenate legs: %d", len(r.Geometry))
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"No path could be found"}`))
	}))
	defer srv2.Close()
	if _, err := NewValhalla(srv2.URL, srv2.Client()).Route(context.Background(), models.TravelModeWalk, loop); err == nil {
		t.Fatal("expected error from 400")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
