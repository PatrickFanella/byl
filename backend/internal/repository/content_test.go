package repository

import (
	"context"
	"errors"
	"testing"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func newStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	return New(testdb.Open(t)), context.Background()
}

func TestGetCityBySlugAndID(t *testing.T) {
	s, ctx := newStore(t)
	for _, ref := range []string{"downers-grove", testdb.SeedCityID} {
		c, err := s.GetCity(ctx, ref)
		if err != nil {
			t.Fatalf("GetCity(%s): %v", ref, err)
		}
		if c.Name != "Downers Grove" || len(c.Topics) != 4 {
			t.Fatalf("GetCity(%s): %+v", ref, c)
		}
	}
	if _, err := s.GetCity(ctx, "nowhere"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListPlacesNearSortsByDistance(t *testing.T) {
	s, ctx := newStore(t)
	places, _, err := s.ListPlaces(ctx, PlaceFilter{
		CityID:  testdb.SeedCityID,
		Near:    &geo.Point{Lat: 41.7959, Lng: -88.0092},
		RadiusM: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != 3 {
		t.Fatalf("expected 3 places within 300 m, got %d", len(places))
	}
	if places[0].Slug != "main-street-station" || places[0].DistanceM == nil || *places[0].DistanceM > 1 {
		t.Fatalf("first place should be the station at ~0 m: %+v", places[0])
	}
	for i := 1; i < len(places); i++ {
		if *places[i].DistanceM < *places[i-1].DistanceM {
			t.Fatalf("not sorted by distance: %v", places)
		}
	}
	if places[0].MomentCount != 3 {
		t.Fatalf("station should count 3 published moments, got %d", places[0].MomentCount)
	}
}

func TestListPlacesByTopicAndBBox(t *testing.T) {
	s, ctx := newStore(t)
	places, _, err := s.ListPlaces(ctx, PlaceFilter{CityID: testdb.SeedCityID, TopicID: testdb.SeedTopicHousesID})
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != 1 || places[0].Slug != "downers-grove-museum" {
		t.Fatalf("topic filter: %+v", places)
	}
	box := geo.BBox{MinLat: 41.793, MinLng: -88.02, MaxLat: 41.80, MaxLng: -88.00}
	places, _, err = s.ListPlaces(ctx, PlaceFilter{CityID: testdb.SeedCityID, BBox: &box})
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != 3 {
		t.Fatalf("bbox should exclude the museum, got %d", len(places))
	}
	box.MinLat = 41.79
	places, _, err = s.ListPlaces(ctx, PlaceFilter{CityID: testdb.SeedCityID, BBox: &box})
	if err != nil || len(places) != 4 {
		t.Fatalf("expanded bbox should include the corrected Museum pin: %d %v", len(places), err)
	}
}

func TestGetPlaceHidesUnpublishedMoments(t *testing.T) {
	s, ctx := newStore(t)
	p, err := s.GetPlace(ctx, "downers-grove-museum", "downers-grove", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Moments) != 2 || p.PrimaryMoment == nil || p.PrimaryMoment.ID != testdb.SeedMomentBlodgettID {
		t.Fatalf("published view: %+v", p)
	}
	all, err := s.GetPlace(ctx, testdb.SeedPlaceMuseumID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Moments) != 3 {
		t.Fatalf("unpublished view should include the in_review moment, got %d", len(all.Moments))
	}
	if _, err := s.GetPlace(ctx, "nope", "downers-grove", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetMomentLoadsSourcesTopicsMedia(t *testing.T) {
	s, ctx := newStore(t)
	m, err := s.GetMoment(ctx, testdb.SeedMomentTalkiesID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 2 || m.Sources[0].Title != "Tivoli Theatre" {
		t.Fatalf("sources: %+v", m.Sources)
	}
	if len(m.Topics) != 2 || len(m.TopicIDs) != 2 {
		t.Fatalf("topics: %+v", m.Topics)
	}
	if m.DateDisplay != "December 25, 1928" || m.DateStart == nil || m.DateStart.Year() != 1928 {
		t.Fatalf("dates: %+v", m.MomentSummary)
	}
	if _, err := s.GetMoment(ctx, testdb.SeedMomentDownerID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("in_review moment must be hidden from the public view, got %v", err)
	}
}

func TestMomentPublishBlockersFlagsUnlicensedMedia(t *testing.T) {
	s, ctx := newStore(t)
	asset, err := s.CreateMedia(ctx, models.MediaAsset{Kind: models.MediaKindImage, StorageKey: "x.jpg", PublicURL: "http://x/x.jpg", MimeType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceMomentMedia(ctx, testdb.SeedMomentBurlingtonID, []string{asset.ID}); err != nil {
		t.Fatal(err)
	}
	blockers, err := s.MomentPublishBlockers(ctx, testdb.SeedMomentBurlingtonID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blockers) != 1 {
		t.Fatalf("expected one blocker, got %v", blockers)
	}
	lic := "CC-BY-4.0"
	if _, err := s.UpdateMediaMetadata(ctx, asset.ID, models.MediaMetadataRequest{License: &lic, Caption: ptr("The depot"), Credit: ptr("Archive photographer"), SourceURL: ptr("https://example.org/photo"), LicenseURL: ptr("https://creativecommons.org/licenses/by/4.0/")}); err != nil {
		t.Fatal(err)
	}
	blockers, _ = s.MomentPublishBlockers(ctx, testdb.SeedMomentBurlingtonID)
	if len(blockers) != 0 {
		t.Fatalf("expected no blockers after licensing, got %v", blockers)
	}
	m, _ := s.GetMoment(ctx, testdb.SeedMomentBurlingtonID, false)
	if len(m.Media) != 1 || m.Media[0].License != "CC-BY-4.0" {
		t.Fatalf("media: %+v", m.Media)
	}
}

func TestCreateAndUpdatePlaceAndMoment(t *testing.T) {
	s, ctx := newStore(t)
	p, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: testdb.SeedCityID, Name: "Lyman Woods", Latitude: 41.82, Longitude: -87.99})
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "lyman-woods" || p.Status != models.EntityStatusDraft || p.ArrivalRadiusM != 40 {
		t.Fatalf("created place: %+v", p)
	}
	name := "Lyman Woods Preserve"
	p2, err := s.UpdatePlace(ctx, p.ID, models.UpdatePlaceRequest{Name: &name})
	if err != nil || p2.Name != name || p2.Slug != "lyman-woods" {
		t.Fatalf("update: %v %+v", err, p2)
	}
	m, err := s.CreateMoment(ctx, models.CreateMomentRequest{
		PlaceID: p.ID, Title: "Preserve Established", Body: "First line.\n\nSecond paragraph.",
		DateStart: "1981-01-01", DatePrecision: models.DatePrecisionYear, DateDisplay: "1981",
		TopicIDs: []string{testdb.SeedTopicParksID},
		Sources:  []models.Source{{Title: "Forest Preserve District", URL: "https://example.org"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Slug != "preserve-established" || m.Excerpt != "First line." || len(m.Sources) != 1 || len(m.TopicIDs) != 1 {
		t.Fatalf("created moment: %+v", m)
	}
	if err := s.SetMomentStatus(ctx, m.ID, models.EntityStatusPublished); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPrimaryMoment(ctx, p.ID, &m.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlaceStatus(ctx, p.ID, models.EntityStatusPublished); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPlace(ctx, p.ID, "", false)
	if err != nil || got.PrimaryMoment == nil || got.PrimaryMoment.ID != m.ID {
		t.Fatalf("primary moment: %v %+v", err, got)
	}
	dup, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: testdb.SeedCityID, Name: "Lyman Woods", Latitude: 41.82, Longitude: -87.99})
	if err != nil || dup.Slug == "lyman-woods" {
		t.Fatalf("duplicate name should get a distinct slug: %v %+v", err, dup)
	}
}

func TestTopicsCarryMomentCounts(t *testing.T) {
	s, ctx := newStore(t)
	topics, err := s.ListTopics(ctx, testdb.SeedCityID)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, tp := range topics {
		counts[tp.Slug] = tp.MomentCount
	}
	if counts["rail-and-the-commuter-town"] != 4 || counts["houses-and-builders"] != 2 {
		t.Fatalf("published moment counts per topic: %v", counts)
	}
	tp, err := s.CreateTopic(ctx, models.CreateTopicRequest{CityID: testdb.SeedCityID, Name: "Schools & Libraries"})
	if err != nil || tp.Slug != "schools-libraries" {
		t.Fatalf("create topic: %v %+v", err, tp)
	}
}
