package repository

import (
	"errors"
	"testing"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestNearestPublishedPlace(t *testing.T) {
	s, ctx := newStore(t)
	p, err := s.NearestPublishedPlace(ctx, testdb.SeedCityID, geo.Point{Lat: 41.7960, Lng: -88.0093}, 150)
	if err != nil || p.ID != testdb.SeedPlaceStationID {
		t.Fatalf("%v %+v", err, p)
	}
	if _, err := s.NearestPublishedPlace(ctx, testdb.SeedCityID, geo.Point{Lat: 41.9, Lng: -87.6}, 150); !errors.Is(err, ErrNotFound) {
		t.Fatalf("far point: %v", err)
	}
	_ = s.SetPlaceStatus(ctx, testdb.SeedPlaceStationID, models.EntityStatusDraft)
	p, err = s.NearestPublishedPlace(ctx, testdb.SeedCityID, geo.Point{Lat: 41.7960, Lng: -88.0093}, 150)
	if err != nil || p.ID != testdb.SeedPlaceTivoliID {
		t.Fatalf("draft places are skipped: %v %+v", err, p)
	}
}

func TestCandidateExistsAndFilters(t *testing.T) {
	s, ctx := newStore(t)
	ok, err := s.CandidateExists(ctx, "wikipedia", "Q1")
	if err != nil || ok {
		t.Fatalf("%v %v", err, ok)
	}
	src, ext := "wikipedia", "Q1"
	if _, err := s.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindMoment, Origin: models.CandidateOriginImport, SourceName: &src, ExternalID: &ext, Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.CandidateExists(ctx, "wikipedia", "Q1"); !ok {
		t.Fatal("expected exists")
	}
	byOrigin, _, _ := s.ListCandidates(ctx, CandidateFilter{Origin: models.CandidateOriginImport})
	bySource, _, _ := s.ListCandidates(ctx, CandidateFilter{Source: "wikipedia"})
	none, _, _ := s.ListCandidates(ctx, CandidateFilter{Source: "loc"})
	if len(byOrigin) != 1 || len(bySource) != 1 || len(none) != 0 {
		t.Fatalf("filters: %d %d %d", len(byOrigin), len(bySource), len(none))
	}
}

func TestUpdateMediaFileAndCandidatePayload(t *testing.T) {
	s, ctx := newStore(t)
	a, _ := s.CreateMedia(ctx, models.MediaAsset{Kind: models.MediaKindImage})
	wd, ht := 10, 20
	if err := s.UpdateMediaFile(ctx, a.ID, "images/x.png", "http://m/images/x.png", "image/png", &wd, &ht); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetMedia(ctx, a.ID)
	if got.StorageKey != "images/x.png" || got.MimeType != "image/png" || *got.Width != 10 {
		t.Fatalf("%+v", got)
	}
	c, _ := s.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindMoment, Origin: models.CandidateOriginImport, Payload: []byte(`{"raw_text":"x"}`)})
	draft := models.CandidateOriginDraft
	if err := s.UpdateCandidatePayload(ctx, c.ID, []byte(`{"raw_text":"x","title":"T"}`), &draft); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetCandidate(ctx, c.ID)
	if got2.Origin != models.CandidateOriginDraft || string(got2.Payload) == `{"raw_text":"x"}` {
		t.Fatalf("%+v", got2)
	}
	if err := s.SetJobNote(ctx, "00000000-0000-4000-8000-00000000ffff", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("note on missing job: %v", err)
	}
}
