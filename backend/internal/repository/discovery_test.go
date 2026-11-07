package repository

import (
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/sources"
	"invisible-museum/backend/internal/testdb"
)

func TestDiscoveryCollectionReuseIsPublishedAndGeographic(t *testing.T) {
	s, ctx := newStore(t)
	in := models.RouteBriefInput{Topic: "Railroad", Area: "Downers Grove", PlaceLeads: []string{"Main Street Station"}}
	items, scope, err := s.DiscoveryCollection(ctx, in)
	if err != nil || scope == nil || scope.CityID != testdb.SeedCityID || len(items) == 0 {
		t.Fatalf("collection: %+v %+v %v", items, scope, err)
	}
	found := false
	for _, item := range items {
		if item.Evidence != "published_collection" || item.EntityID == "" {
			t.Fatal("missing collection provenance")
		}
		if item.EntityID == testdb.SeedPlaceStationID {
			found = true
			if len(sources.DiscoveryReasons(in, item, scope)) == 0 {
				t.Fatal("known station did not match")
			}
		}
	}
	if !found {
		t.Fatal("known station not reused")
	}
	in.Area = "Downers Grove Iowa"
	items, scope, err = s.DiscoveryCollection(ctx, in)
	if err != nil || scope != nil || len(items) != 0 {
		t.Fatalf("namesake area leaked collection: %+v %+v %v", items, scope, err)
	}
}
