package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestPolyline6RoundTripPreservesDuplicateBreaksAndDirection(t *testing.T) {
	points := [][2]float64{{-88, 41}, {-88.001, 41.000001}, {-88.001, 41.000001}, {-88, 41}}
	if got := DecodePolyline6(EncodePolyline6(points)); !reflect.DeepEqual(got, points) {
		t.Fatalf("polyline round trip %v", got)
	}
}

func TestPathInspectorUsesStrictOriginalStoryLeg(t *testing.T) {
	_, route, _, _ := qualityFixture()
	shape := EncodePolyline6(route.Legs[0].Geometry)
	route.Legs[0].EncodedShape = shape
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/trace_attributes" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["shape_match"] != "edge_walk" || request["encoded_polyline"] != shape || request["costing"] != "bicycle" {
			t.Errorf("wrong attribution request %+v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"shape": shape, "osm_changeset": 123, "edges": []any{map[string]any{"id": 1, "way_id": 11, "node_id": 1, "end_node": map[string]any{"node_id": 3}, "forward": true, "begin_shape_index": 0, "end_shape_index": 2, "travel_mode": "bicycle", "use": "cycleway", "traversability": "both"}}})
	}))
	defer srv.Close()
	got, err := NewValhalla(srv.URL, srv.Client()).(PathInspector).InspectPath(context.Background(), models.TravelModeBike, route)
	if err != nil || got.Failure != "" || calls != 1 || got.Legs[0].Edges[0].BeginNodeID != 1 || got.Legs[0].Edges[0].EndNodeID != 3 || got.Legs[0].OSMChangeset != 123 {
		t.Fatalf("attribution %+v %v calls%d", got, err, calls)
	}
}

func TestPathInspectorRefusalNeverRetriesOrResnaps(t *testing.T) {
	_, route, _, _ := qualityFixture()
	route.Legs = append(route.Legs, route.Legs[0])
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error_code":443,"error":"Use walk_or_snap instead"}`))
	}))
	defer srv.Close()
	got, err := NewValhalla(srv.URL, srv.Client()).(PathInspector).InspectPath(context.Background(), models.TravelModeWalk, route)
	if err != nil || got.Failure != "exact_match_unavailable" || calls != 1 || len(got.Legs) != 0 {
		t.Fatalf("attribution refusal %+v %v calls%d", got, err, calls)
	}
}

func TestPathInspectorMissingDirectionStaysUnknown(t *testing.T) {
	_, route, _, _ := qualityFixture()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"shape": EncodePolyline6(route.Legs[0].Geometry), "osm_changeset": 123, "edges": []any{map[string]any{"id": 1, "way_id": 11, "begin_shape_index": 0, "end_shape_index": 2}}})
	}))
	defer srv.Close()
	got, err := NewValhalla(srv.URL, srv.Client()).(PathInspector).InspectPath(context.Background(), models.TravelModeWalk, route)
	if err != nil || got.Failure != "attribution_incomplete" {
		t.Fatalf("missing direction accepted %+v %v", got, err)
	}
}
