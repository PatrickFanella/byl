package httpapi

import (
	"context"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestJourneyHTTPReadinessLegacySummariesAndFrozenRevision(t *testing.T) {
	e := newTestEnv(t)
	curator := e.signIn(t, "journey-curator@example.com", models.AccountRoleCurator)
	base := "/api/v1/admin/tours/" + testdb.SeedTourID
	r := e.get(t, base, curator)
	if r.status != 200 {
		t.Fatal(r.status, string(r.raw))
	}
	pending := r.body["journeys"].(map[string]any)["walk"].(map[string]any)
	if pending["state"] != "pending" || pending["travel_duration_s"] != nil {
		t.Fatal("pending result invented metrics", string(r.raw))
	}
	e.drain(t)
	r = e.get(t, base, curator)
	journey := r.body["journeys"].(map[string]any)["walk"].(map[string]any)
	if journey["state"] != "blocked" || journey["usable"] != false || journey["travel_duration_s"] == nil {
		t.Fatal("straight line became usable", string(r.raw))
	}
	legacy := r.body["routes"].(map[string]any)["walk"].(map[string]any)
	if legacy["status"] != "ready" || legacy["estimated_minutes"] == nil || legacy["duration_s"] != journey["travel_duration_s"] {
		t.Fatal("legacy travel fields changed", string(r.raw))
	}
	if journey["round_trip_duration_s"] != nil || journey["return_plan"].(map[string]any)["status"] != "unplanned" {
		t.Fatal("invented round trip", string(r.raw))
	}
	// Synthetic closing geometry/checkpoint tests transport, not field proof.
	working, err := e.store.GetTour(context.Background(), testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	closing, err := e.store.GetTourRoute(context.Background(), testdb.SeedTourID, models.TravelModeWalk)
	if err != nil {
		t.Fatal(err)
	}
	first, last := closing.Geometry.Coordinates[0], closing.Geometry.Coordinates[len(closing.Geometry.Coordinates)-1]
	closing.Geometry.Coordinates = append(closing.Geometry.Coordinates, first)
	closing.DistanceM += 50
	closing.DurationS += 60
	closing.Legs = append(closing.Legs, models.RouteLeg{FromPosition: len(working.Stops), ToPosition: 1, FromStopID: working.Stops[len(working.Stops)-1].ID, ToStopID: working.Stops[0].ID, Return: true, DistanceM: 50, DurationS: 60, Geometry: [][2]float64{last, first}})
	if err := e.store.UpsertRoute(context.Background(), *closing); err != nil {
		t.Fatal(err)
	}
	e.publishFixture(t, testdb.SeedTourID)
	public := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, "")
	if public.status != 200 {
		t.Fatal(public.status, string(public.raw))
	}
	selected := public.body["journeys"].(map[string]any)["walk"].(map[string]any)
	if selected["state"] != "usable" || selected["usable"] != true || selected["round_trip_duration_s"] != selected["itinerary_duration_s"] || selected["return_plan"].(map[string]any)["duration_s"] != float64(60) {
		t.Fatal("qualifying snapshot missing", string(public.raw))
	}
	for _, word := range []string{"synthetic-http-inspection", "Private synthetic HTTP", "attribution", "inspections"} {
		if strings.Contains(string(public.raw), word) {
			t.Fatal("private checkpoint leaked", word)
		}
	}
	route := e.get(t, "/api/v1/tours/"+testdb.SeedTourID+"/route?mode=walk", "")
	if route.status != 200 || route.body["journey"].(map[string]any)["itinerary_duration_s"] != selected["itinerary_duration_s"] {
		t.Fatal("full route disagrees", string(route.raw))
	}
	// A new working generation must not alter the served immutable explanation.
	_, err = e.store.Pool().Exec(context.Background(), `UPDATE tour_routes SET status='pending',distance_m=999999,duration_s=999999 WHERE tour_id=$1 AND mode='walk'`, testdb.SeedTourID)
	if err != nil {
		t.Fatal(err)
	}
	after := e.get(t, "/api/v1/tours/"+testdb.SeedTourID, "")
	if after.body["journeys"].(map[string]any)["walk"].(map[string]any)["itinerary_duration_s"] != selected["itinerary_duration_s"] {
		t.Fatal("live edits rewrote a retained journey")
	}
}

func TestJourneyHTTPStaleFailedAndMissingResults(t *testing.T) {
	e := newTestEnv(t)
	e.drain(t)
	curator := e.signIn(t, "journey-partial@example.com", models.AccountRoleCurator)
	e.qualifyFixtureRoutes(t, testdb.SeedTourID)
	ctx := context.Background()
	if _, err := e.store.Pool().Exec(ctx, `UPDATE tour_revision_state SET route_version=route_version+1 WHERE tour_id=$1`, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/tours/" + testdb.SeedTourID
	r := e.get(t, base, curator)
	if r.status != 200 || r.body["journeys"].(map[string]any)["walk"].(map[string]any)["state"] != "stale" {
		t.Fatal("stale quality became usable", string(r.raw))
	}
	if _, err := e.store.Pool().Exec(ctx, `DELETE FROM tour_routes WHERE tour_id=$1 AND mode='bike'`, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE tour_routes SET status='failed',geometry=NULL WHERE tour_id=$1 AND mode='walk'`, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	r = e.get(t, base, curator)
	if r.status != 200 {
		t.Fatal(r.status, string(r.raw))
	}
	journeys := r.body["journeys"].(map[string]any)
	if journeys["bike"].(map[string]any)["state"] != "missing" || journeys["walk"].(map[string]any)["state"] != "failed" {
		t.Fatal("partial modes lost", string(r.raw))
	}
	for _, value := range journeys {
		j := value.(map[string]any)
		if j["travel_duration_s"] != nil || j["itinerary_duration_s"] != nil || j["usable"] != false {
			t.Fatal("unavailable mode invented estimates", string(r.raw))
		}
	}
	if _, exists := r.body["routes"].(map[string]any)["bike"]; exists {
		t.Fatal("changed legacy absent mode representation")
	}
}
