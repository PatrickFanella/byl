package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestRegionalCuratedTourAPIAndLegacyReads(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	curator := e.signIn(t, "regional@example.com", models.AccountRoleCurator)
	city, err := e.store.CreateCity(ctx, models.CreateCityRequest{Name: "Lisle", Region: "Illinois", CenterLat: 41.8, CenterLng: -88.1, BBoxMinLat: 41.7, BBoxMaxLat: 41.9, BBoxMinLng: -88.2, BBoxMaxLng: -88})
	if err != nil {
		t.Fatal(err)
	}
	place, err := e.store.CreatePlace(ctx, models.CreatePlaceRequest{CityID: city.ID, Name: "Fixture depot", Latitude: 41.8, Longitude: -88.1})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetPlaceStatus(ctx, place.ID, models.EntityStatusPublished); err != nil {
		t.Fatal(err)
	}
	r := e.do(t, http.MethodPost, "/api/v1/admin/topics", curator, map[string]any{"city_id": testdb.SeedCityID, "city_ids": []string{city.ID}, "name": "Regional rail"})
	if r.status != 201 || len(r.body["city_ids"].([]any)) != 2 {
		t.Fatalf("shared topic: %d %s", r.status, r.raw)
	}
	topicID := r.body["id"].(string)
	r = e.do(t, http.MethodPost, "/api/v1/admin/tours", curator, map[string]any{"city_id": testdb.SeedCityID, "title": "Regional corridor", "topic_id": topicID, "modes": []string{"walk"}, "stops": []map[string]any{{"place_id": testdb.SeedPlaceStationID}, {"place_id": place.ID}}})
	if r.status != 201 || len(r.body["city_ids"].([]any)) != 2 {
		t.Fatalf("cross-city create: %d %s", r.status, r.raw)
	}
	id := r.body["id"].(string)
	slug := r.body["slug"].(string)
	if got := e.get(t, "/api/v1/tours/"+slug, ""); got.status != 404 {
		t.Fatal("draft spanning tour became public")
	}
	if got := e.do(t, http.MethodPost, "/api/v1/admin/tours/"+id+"/publish", curator, nil); got.status != 409 {
		t.Fatal("cross-city tour bypassed pending route check")
	}
	e.drain(t)
	if got := e.do(t, http.MethodPost, "/api/v1/admin/tours/"+id+"/publish", curator, nil); got.status != 200 {
		t.Fatalf("publish: %d %s", got.status, got.raw)
	}
	for _, cityRef := range []string{"downers-grove", city.Slug} {
		got := e.get(t, "/api/v1/cities/"+cityRef+"/tours?topic="+topicID, "")
		if got.status != 200 || len(items(t, got)) != 1 || items(t, got)[0].(map[string]any)["id"] != id {
			t.Fatalf("city discovery: %d %s", got.status, got.raw)
		}
	}
	public := e.get(t, "/api/v1/tours/"+slug, "")
	var legacy struct {
		CityID string `json:"city_id"`
		Slug   string `json:"slug"`
		Stops  []struct {
			Place struct {
				CityID string `json:"city_id"`
				ID     string `json:"id"`
			} `json:"place"`
		} `json:"stops"`
	}
	if err := json.Unmarshal(public.raw, &legacy); err != nil || legacy.CityID != testdb.SeedCityID || legacy.Slug != slug || len(legacy.Stops) != 2 || legacy.Stops[1].Place.CityID != city.ID || legacy.Stops[1].Place.ID != place.ID {
		t.Fatalf("legacy fields lost: %+v %v", legacy, err)
	}
	for _, tc := range []struct {
		visibility string
		direct     int
	}{{"unlisted", 200}, {"private", 404}} {
		if got := e.do(t, http.MethodPatch, "/api/v1/admin/tours/"+id, curator, map[string]any{"visibility": tc.visibility}); got.status != 200 {
			t.Fatalf("visibility patch: %s", got.raw)
		}
		for _, cityRef := range []string{"downers-grove", city.Slug} {
			got := e.get(t, "/api/v1/cities/"+cityRef+"/tours?topic="+topicID, "")
			if got.status != 200 || len(items(t, got)) != 0 {
				t.Fatalf("hidden tour leaked: %s", got.raw)
			}
		}
		if got := e.get(t, "/api/v1/tours/"+slug, ""); got.status != tc.direct {
			t.Fatalf("direct visibility: %d %s", got.status, got.raw)
		}
	}
	if got := e.get(t, "/api/v1/admin/tours/"+id, curator); got.status != 200 {
		t.Fatal("curator lost private tour access")
	}
	if got := e.do(t, http.MethodPatch, "/api/v1/admin/topics/"+topicID, curator, map[string]any{"city_ids": []string{}}); got.status != 422 {
		t.Fatalf("used topic coverage removed: %d %s", got.status, got.raw)
	}
}

func TestRegionalTopicAPIPatchSemanticsAndInvalidReferences(t *testing.T) {
	e := newTestEnv(t)
	curator := e.signIn(t, "regional@example.com", models.AccountRoleCurator)
	city, err := e.store.CreateCity(context.Background(), models.CreateCityRequest{Name: "Lisle", CenterLat: 41.8, CenterLng: -88.1, BBoxMinLat: 41.7, BBoxMaxLat: 41.9, BBoxMinLng: -88.2, BBoxMaxLng: -88})
	if err != nil {
		t.Fatal(err)
	}
	r := e.do(t, http.MethodPost, "/api/v1/admin/topics", curator, map[string]any{"city_id": testdb.SeedCityID, "name": "Shared theme", "city_ids": []string{city.ID}})
	if r.status != 201 {
		t.Fatal(string(r.raw))
	}
	id := r.body["id"].(string)
	for _, body := range []map[string]any{{"description": "Curator edit"}, {"city_ids": nil}} {
		got := e.do(t, http.MethodPatch, "/api/v1/admin/topics/"+id, curator, body)
		if got.status != 200 || len(got.body["city_ids"].([]any)) != 2 {
			t.Fatalf("omitted/null changed coverage: %s", got.raw)
		}
	}
	got := e.do(t, http.MethodPatch, "/api/v1/admin/topics/"+id, curator, map[string]any{"city_ids": []string{}})
	if got.status != 200 || len(got.body["city_ids"].([]any)) != 1 || got.body["city_id"] != testdb.SeedCityID {
		t.Fatalf("empty set removed original city: %s", got.raw)
	}
	for _, bad := range []string{"bad-id", "00000000-0000-4000-8000-00000000ffff"} {
		got := e.do(t, http.MethodPatch, "/api/v1/admin/topics/"+id, curator, map[string]any{"city_ids": []string{bad}, "name": "Do not save"})
		if got.status != 422 {
			t.Fatalf("bad reference: %d %s", got.status, got.raw)
		}
	}
	got = e.get(t, "/api/v1/topics/"+id, "")
	if got.status != 200 || got.body["name"] != "Shared theme" || got.body["description"] != "Curator edit" {
		t.Fatalf("failed patch clobbered curator fields: %s", got.raw)
	}
	for _, body := range []map[string]any{
		{"city_id": testdb.SeedCityID, "title": "Unknown stop", "stops": []map[string]any{{"place_id": "00000000-0000-4000-8000-00000000ffff"}}},
		{"city_id": testdb.SeedCityID, "title": "Unknown topic", "topic_id": "00000000-0000-4000-8000-00000000ffff", "stops": []map[string]any{{"place_id": testdb.SeedPlaceStationID}}},
	} {
		got := e.do(t, http.MethodPost, "/api/v1/admin/tours", curator, body)
		if got.status != 422 {
			t.Fatalf("invalid tour reference: %d %s", got.status, got.raw)
		}
	}
}
