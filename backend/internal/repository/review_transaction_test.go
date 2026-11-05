package repository

import (
	"errors"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestConcurrentCandidateReviewsCreateOnlyOneEntity(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "reviewer", "reviewer@example.com", "", true)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindPlace})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- s.ReviewPendingCandidate(ctx, c.ID, func(tx *Store, candidate *models.Candidate) error {
				place, err := tx.CreatePlace(ctx, models.CreatePlaceRequest{CityID: testdb.SeedCityID, Name: "Concurrent Place", Latitude: 41.8, Longitude: -88})
				if err != nil {
					return err
				}
				_, err = tx.ReviewCandidate(ctx, candidate.ID, a.ID, models.CandidateStatusApproved, "", &place.ID)
				return err
			})
		}()
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflict++
		default:
			t.Error(err)
		}
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM places WHERE name = 'Concurrent Place'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if success != 1 || conflict != 1 || count != 1 {
		t.Fatalf("success=%d conflict=%d entities=%d", success, conflict, count)
	}
}

func TestFailedReviewRollsBackCreatedEntity(t *testing.T) {
	s, ctx := newStore(t)
	c, err := s.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindPlace})
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("review storage failed")
	err = s.ReviewPendingCandidate(ctx, c.ID, func(tx *Store, _ *models.Candidate) error {
		if _, err := tx.CreatePlace(ctx, models.CreatePlaceRequest{CityID: testdb.SeedCityID, Name: "Rolled Back Place", Latitude: 41.8, Longitude: -88}); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM places WHERE name = 'Rolled Back Place'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	current, err := s.GetCandidate(ctx, c.ID)
	if err != nil || count != 0 || current.Status != models.CandidateStatusPending {
		t.Fatalf("entity count=%d candidate=%+v err=%v", count, current, err)
	}
}

func TestAmbiguousPlaceSlugRequiresCity(t *testing.T) {
	s, ctx := newStore(t)
	city, err := s.CreateCity(ctx, models.CreateCityRequest{Name: "Other City", CountryCode: "US", CenterLat: 41, CenterLng: -88,
		BBoxMinLat: 40, BBoxMaxLat: 42, BBoxMinLng: -89, BBoxMaxLng: -87, Timezone: "America/Chicago"})
	if err != nil {
		t.Fatal(err)
	}
	place, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: city.ID, Slug: "main-street-station", Name: "Other Station", Latitude: 41, Longitude: -88})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolvePlaceID(ctx, "main-street-station", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("ambiguous slug must conflict, got %v", err)
	}
	if id, err := s.ResolvePlaceID(ctx, "main-street-station", city.ID); err != nil || id != place.ID {
		t.Fatalf("city must disambiguate: %s %v", id, err)
	}
}

func TestPublicationStatusRollsBackWhenJobEnqueueFails(t *testing.T) {
	s, ctx := newStore(t)
	before, err := s.GetMoment(ctx, testdb.SeedMomentBurlingtonID, true)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Transaction(ctx, func(tx *Store) error {
		if err := tx.SetMomentStatus(ctx, before.ID, models.EntityStatusDraft); err != nil {
			return err
		}
		return tx.Enqueue(ctx, models.JobKindGenerateNarration, make(chan int))
	})
	if err == nil {
		t.Fatal("unsupported job payload must fail")
	}
	after, err := s.GetMoment(ctx, before.ID, true)
	if err != nil || after.Status != before.Status {
		t.Fatalf("status must roll back: %+v %v", after, err)
	}
}
