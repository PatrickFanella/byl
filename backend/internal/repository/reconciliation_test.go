package repository

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestReconciliationConcurrentBriefsPreserveEvidenceAndEdits(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "reconcile", "reconcile@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	briefs := []*models.RouteBrief{}
	for range 2 {
		b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Railroad", Area: "Downers Grove"}, "")
		if err != nil {
			t.Fatal(err)
		}
		briefs = append(briefs, b)
	}
	lat, lng := 41.82, -88.04
	item := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: "Fixture workshop", URL: "https://archive.example/workshop", Lat: &lat, Lng: &lng, Identity: &models.IdentityFacts{Address: "999 Fixture Street", MomentTitle: "Workshop opening", DateStart: "1900-01-01", DatePrecision: models.DatePrecisionYear}}
	type result struct {
		r   *models.ReconciliationRecord
		err error
	}
	ch := make(chan result, 2)
	start := make(chan struct{})
	for _, b := range briefs {
		go func(b *models.RouteBrief) {
			<-start
			r, err := s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: b.Version, Item: item})
			ch <- result{r, err}
		}(b)
	}
	close(start)
	var first *models.ReconciliationRecord
	for range 2 {
		got := <-ch
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.r.PlaceCandidateID == nil || got.r.MomentCandidateID != nil || got.r.Proposal.Outcome != "new_place" {
			t.Fatalf("proposal %+v", got.r)
		}
		if first != nil && first.ID != got.r.ID {
			t.Fatal("duplicate identity")
		}
		first = got.r
	}
	if err := s.UpdateCandidatePayload(ctx, *first.PlaceCandidateID, json.RawMessage(`{"name":"Curator corrected workshop"}`), nil); err != nil {
		t.Fatal(err)
	}
	item.Origins = []models.DiscoveryOrigin{{Provider: "second_archive", Query: "different query", URL: item.URL}}
	if _, err := s.ReconcileBrief(ctx, briefs[0].ID, models.ReconciliationRequest{Version: 1, Item: item}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcileBrief(ctx, briefs[0].ID, models.ReconciliationRequest{Version: 1, Item: item}); err != nil {
		t.Fatal(err)
	}
	var candidates, sources, places int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM candidates WHERE source_name='brief_reconciliation'),(SELECT count(*) FROM reconciliation_sources),(SELECT count(*) FROM places WHERE name='Fixture workshop')`).Scan(&candidates, &sources, &places); err != nil {
		t.Fatal(err)
	}
	if candidates != 1 || sources != 3 || places != 0 {
		t.Fatalf("candidates %d sources %d places %d", candidates, sources, places)
	}
	c, err := s.GetCandidate(ctx, *first.PlaceCandidateID)
	if err != nil || !strings.Contains(string(c.Payload), "Curator corrected") {
		t.Fatalf("curator edits %+v %v", c, err)
	}
	list, err := s.ListReconciliation(ctx, briefs[0].ID, 20, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("source links %+v %v", list, err)
	}
	provenance, err := s.ListReconciliationSources(ctx, briefs[0].ID, first.ID, 20, 0)
	if err != nil || len(provenance) != 2 {
		t.Fatalf("source history %+v %v", provenance, err)
	}
	// Distinct discoveries with incompatible dates remain visible and cannot create a second candidate.
	item.Identity.DateStart = "1910-01-01"
	r, err := s.ReconcileBrief(ctx, briefs[0].ID, models.ReconciliationRequest{Version: 1, Item: item})
	if err != nil || !slices.Contains(r.Assessment.Issues, "conflicting_discoveries") {
		t.Fatalf("conflict %+v %v", r, err)
	}
	// A second name at the pending address cannot mint another entity.
	item.Title = "Different workshop"
	item.Identity.MomentTitle = ""
	r, err = s.ReconcileBrief(ctx, briefs[0].ID, models.ReconciliationRequest{Version: 1, Item: item})
	if err != nil || r.PlaceCandidateID != nil || !slices.Contains(r.Assessment.Issues, "pending_identity_conflict") {
		t.Fatalf("pending collision %+v %v", r, err)
	}
	// An approved draft with curator corrections remains the same identity.
	approved, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: testdb.SeedCityID, Name: "Curator corrected workshop", Address: "1001 Corrected Street", Latitude: lat, Longitude: lng})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCandidate(ctx, *first.PlaceCandidateID, a.ID, models.CandidateStatusApproved, "Reviewed fixture", &approved.ID); err != nil {
		t.Fatal(err)
	}
	// Reuse the original complete lead, with source coordinates far enough from
	// the corrected draft to exercise the approved identity binding.
	original := first.Proposal.Item
	original.Lat = new(float64)
	*original.Lat = 41.84
	r, err = s.ReconcileBrief(ctx, briefs[0].ID, models.ReconciliationRequest{Version: 1, Item: original})
	if err != nil || r.Assessment.PlaceID != approved.ID || r.Assessment.Outcome != "needs_review" {
		t.Fatalf("approved identity %+v %v", r, err)
	}
}

func TestReconciliationDoesNotConflateRailDepots(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "depot-curator", "depot@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Railroad", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	lat, lng := 41.83, -88.03
	if _, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: testdb.SeedCityID, Name: "Great Western depot", Address: "100 Depot Street", Latitude: lat, Longitude: lng}); err != nil {
		t.Fatal(err)
	}
	item := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: "CA&E depot", Lat: &lat, Lng: &lng, URL: "https://archive.example/cae", Identity: &models.IdentityFacts{Address: "100 Depot Street"}}
	r, err := s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: 1, Item: item})
	if err != nil || r.PlaceCandidateID != nil || !slices.Contains(r.Assessment.Issues, "nearby_distinct_identity") {
		t.Fatalf("depots %+v %v", r, err)
	}
}

func TestReconciliationCuratorDecisionsAliasesAndMomentReuse(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "identity-curator", "curator@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "Cemetery history", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: testdb.SeedCityID, Name: "Main Street Cemetery", Address: "5190 Main Street", Latitude: 41.79, Longitude: -88.01})
	if err != nil {
		t.Fatal(err)
	}
	item := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: "Downer burial ground", URL: "https://archive.example/cemetery", Identity: &models.IdentityFacts{Address: "90 Main Street", Aliases: []string{"Main Street Cemetery"}}}
	r, err := s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: 1, Item: item})
	if err != nil || r.Proposal.Outcome != "needs_review" {
		t.Fatalf("lead %+v %v", r, err)
	}
	decision := models.IdentityDecisionRequest{Version: 0, Outcome: "existing_place", PlaceID: p.ID, Aliases: []string{"Downer burial ground"}, HistoricalAddresses: []string{"90 Main Street"}, Note: "Fixture archival identity reviewed; no collection content replaced."}
	d, err := s.DecideReconciliation(ctx, b.ID, r.ID, a.ID, decision)
	if err != nil || d.ReviewVersion != 1 {
		t.Fatalf("decision %+v %v", d, err)
	}
	if _, err := s.DecideReconciliation(ctx, b.ID, r.ID, a.ID, decision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale decision %v", err)
	}
	item.Identity.Address = "5190 Main Street"
	item.Identity.Aliases = nil
	r, err = s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: 1, Item: item})
	if err != nil || r.Proposal.PlaceID != p.ID || r.Proposal.Outcome != "existing_place" {
		t.Fatalf("confirmed alias %+v %v", r, err)
	}
	item.Identity.MomentTitle = "Preservation campaign"
	r, err = s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: 1, Item: item})
	if err != nil || r.MomentCandidateID == nil || r.Proposal.Outcome != "existing_place_new_moment" {
		t.Fatalf("new moment %+v %v", r, err)
	}
	if _, err := s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: 1, Item: item}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM candidates WHERE kind='moment' AND source_name='brief_reconciliation'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("moment duplicates %d %v", count, err)
	}
	m, err := s.CreateMoment(ctx, models.CreateMomentRequest{PlaceID: p.ID, Title: "Preservation campaign", DateStart: "1950-01-01", DatePrecision: models.DatePrecisionYear})
	if err != nil {
		t.Fatal(err)
	}
	item.Identity.DateStart = "1950-06-02"
	item.Identity.DatePrecision = models.DatePrecisionDay
	r, err = s.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: 1, Item: item})
	if err != nil || r.Assessment.MomentID != m.ID || r.Assessment.Outcome != "existing_place_existing_moment" {
		t.Fatalf("existing moment %+v %v", r, err)
	}
	got, err := s.GetPlace(ctx, p.ID, "", true)
	if err != nil || got.Status != models.EntityStatusDraft || got.Address != "5190 Main Street" {
		t.Fatalf("collection changed %+v %v", got, err)
	}
	bad := decision
	bad.Version = 1
	bad.Outcome = "existing_place_existing_moment"
	bad.MomentID = testdb.SeedMomentBurlingtonID
	if _, err := s.DecideReconciliation(ctx, b.ID, d.ID, a.ID, bad); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("foreign moment %v", err)
	}
}
