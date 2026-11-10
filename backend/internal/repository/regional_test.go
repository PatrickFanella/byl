package repository

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func regionalCity(t *testing.T, s *Store, ctx context.Context, name string) (*models.CityDetail, *models.PlaceDetail) {
	t.Helper()
	city, err := s.CreateCity(ctx, models.CreateCityRequest{Name: name, Region: "Illinois", CenterLat: 41.8, CenterLng: -88.1, BBoxMinLat: 41.7, BBoxMaxLat: 41.9, BBoxMinLng: -88.2, BBoxMaxLng: -88})
	if err != nil {
		t.Fatal(err)
	}
	place, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: city.ID, Name: "Fixture depot", Latitude: 41.8, Longitude: -88.1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlaceStatus(ctx, place.ID, models.EntityStatusPublished); err != nil {
		t.Fatal(err)
	}
	return city, place
}

func TestRegionalTourRetainsMunicipalitiesAndDiscoveryDeduplicates(t *testing.T) {
	s, ctx := newStore(t)
	city, place := regionalCity(t, s, ctx, "Lisle")
	topic, err := s.CreateTopic(ctx, models.CreateTopicRequest{CityID: testdb.SeedCityID, CityIDs: []string{city.ID, strings.ToUpper(city.ID)}, Name: "Regional rail"})
	if err != nil {
		t.Fatal(err)
	}
	if len(topic.CityIDs) != 2 || !slices.Contains(topic.CityIDs, city.ID) || topic.CityID != testdb.SeedCityID {
		t.Fatalf("topic coverage: %+v", topic)
	}
	second, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: city.ID, Name: "Fixture crossing", Latitude: 41.81, Longitude: -88.1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlaceStatus(ctx, second.ID, models.EntityStatusPublished); err != nil {
		t.Fatal(err)
	}
	tour, err := s.CreateTour(ctx, CreateTourInput{CityID: testdb.SeedCityID, Title: "Regional corridor", TopicID: &topic.ID, Status: models.EntityStatusPublished, Stops: []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: place.ID}, {PlaceID: second.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if tour.CityID != testdb.SeedCityID || len(tour.CityIDs) != 2 || tour.Stops[1].Place.CityID != city.ID || tour.Stops[0].Place.CityID != testdb.SeedCityID {
		t.Fatalf("municipalities lost: %+v", tour)
	}
	for _, id := range []string{testdb.SeedCityID, city.ID} {
		found, next, err := s.ListTours(ctx, TourFilter{CityID: id, TopicID: topic.ID, PublicOnly: true, Limit: 1})
		if err != nil || len(found) != 1 || found[0].ID != tour.ID || next != "" {
			t.Fatalf("city discovery duplicated/lost tour: %+v %q %v", found, next, err)
		}
	}
	listed, err := s.ListTopics(ctx, city.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != topic.ID {
		t.Fatalf("shared theme: %+v %v", listed, err)
	}
	matched, err := s.TopicsBySlugs(ctx, city.ID, []string{topic.Slug, topic.Slug})
	if err != nil || len(matched) != 1 {
		t.Fatalf("shared topic slug duplicated: %+v %v", matched, err)
	}
	for _, visibility := range []models.TourVisibility{models.TourVisibilityPrivate, models.TourVisibilityUnlisted} {
		if _, err := s.UpdateTour(ctx, tour.ID, UpdateTourInput{Visibility: &visibility}); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{testdb.SeedCityID, city.ID} {
			found, _, err := s.ListTours(ctx, TourFilter{CityID: id, TopicID: topic.ID, PublicOnly: true})
			if err != nil || len(found) != 0 {
				t.Fatalf("hidden tour leaked in city discovery: %+v %v", found, err)
			}
		}
	}
}

func TestRegionalReferenceFailuresAreAtomicAndPersonalRoutesStayLocal(t *testing.T) {
	s, ctx := newStore(t)
	city, place := regionalCity(t, s, ctx, "Lisle")
	topic := testdb.SeedTopicRailID
	before := countJobs(t, s, ctx, models.JobKindComputeRoute)
	_, err := s.CreateTour(ctx, CreateTourInput{CityID: testdb.SeedCityID, Title: "Uncovered tour", TopicID: &topic, Stops: []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: place.ID}}})
	if !errors.Is(err, ErrInvalidReference) || countJobs(t, s, ctx, models.JobKindComputeRoute) != before {
		t.Fatalf("bad coverage created a tour/job: %v", err)
	}
	if err := s.ReplaceStops(ctx, testdb.SeedTourID, []models.StopInput{{PlaceID: place.ID}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("uncovered stops: %v", err)
	}
	tour, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil || len(tour.Stops) != 4 {
		t.Fatal("invalid replacement lost original stops")
	}
	if _, err := s.UpdateTopic(ctx, topic, models.UpdateTopicRequest{CityIDs: []string{city.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceStops(ctx, testdb.SeedTourID, []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: place.ID, FeaturedMomentID: ptr(testdb.SeedMomentTalkiesID)}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("foreign featured moment accepted: %v", err)
	}
	if err := s.ReplaceStops(ctx, testdb.SeedTourID, []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: place.ID}}); err != nil {
		t.Fatal(err)
	}
	topicBefore, err := s.GetTopic(ctx, topic, "")
	if err != nil {
		t.Fatal(err)
	}
	oldName := topicBefore.Name
	changed := "Must not be saved"
	if _, err := s.UpdateTopic(ctx, topic, models.UpdateTopicRequest{Name: &changed, CityIDs: []string{}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("used municipality removed: %v", err)
	}
	loaded, err := s.GetTopic(ctx, topic, "")
	if err != nil || loaded.Name != oldName || !slices.Contains(loaded.CityIDs, city.ID) {
		t.Fatalf("failed patch changed topic: %+v %v", loaded, err)
	}
	for _, bad := range []string{"not-a-city", "00000000-0000-4000-8000-00000000ffff"} {
		if _, err := s.CreateTopic(ctx, models.CreateTopicRequest{CityID: testdb.SeedCityID, CityIDs: []string{bad}, Name: "Bad reference"}); !errors.Is(err, ErrInvalidReference) {
			t.Fatalf("unknown municipality: %v", err)
		}
	}
	owner, err := s.UpsertAccount(ctx, models.AuthProviderDev, "regional-owner", "owner@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTour(ctx, CreateTourInput{CityID: testdb.SeedCityID, Title: "Personal", OwnerAccountID: &owner.ID, Stops: []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: place.ID}}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("personal route crossed a city: %v", err)
	}
}

func TestRegionalMomentTopicsAndStableSlugReferences(t *testing.T) {
	s, ctx := newStore(t)
	city, place := regionalCity(t, s, ctx, "Lisle")
	if _, err := s.CreateMoment(ctx, models.CreateMomentRequest{PlaceID: place.ID, Title: "Uncovered event", TopicIDs: []string{testdb.SeedTopicRailID}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("uncovered moment topic: %v", err)
	}
	shared, err := s.UpdateTopic(ctx, testdb.SeedTopicRailID, models.UpdateTopicRequest{CityIDs: []string{city.ID}})
	if err != nil {
		t.Fatal(err)
	}
	moment, err := s.CreateMoment(ctx, models.CreateMomentRequest{PlaceID: place.ID, Title: "Fixture history", TopicIDs: []string{shared.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(moment.Topics) != 1 || moment.Topics[0].ID != shared.ID {
		t.Fatal("shared theme was duplicated")
	}
	if _, err := s.UpdateTopic(ctx, shared.ID, models.UpdateTopicRequest{CityIDs: []string{}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("moment lost its topic coverage: %v", err)
	}
	if _, err := s.UpdateTopic(ctx, shared.ID, models.UpdateTopicRequest{Description: ptr("Edited by curator")}); err != nil {
		t.Fatal(err)
	}
	third, _ := regionalCity(t, s, ctx, "Naperville")
	if _, err := s.CreateTopic(ctx, models.CreateTopicRequest{CityID: third.ID, CityIDs: []string{city.ID}, Slug: shared.Slug, Name: "Other rail theme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTopic(ctx, shared.Slug, city.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("ambiguous shared slug selected silently: %v", err)
	}
	local, err := s.CreateTopic(ctx, models.CreateTopicRequest{CityID: city.ID, Slug: shared.Slug, Name: "Local theme"})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := s.GetTopic(ctx, local.Slug, city.ID)
	if err != nil || resolved.ID != local.ID {
		t.Fatalf("original local reference changed: %+v %v", resolved, err)
	}
	resolved, err = s.GetTopic(ctx, shared.Slug, testdb.SeedCityID)
	if err != nil || resolved.ID != shared.ID {
		t.Fatalf("original seed reference changed: %+v %v", resolved, err)
	}
}

func TestRegionalTopicCoverageSerializesWithMomentCreation(t *testing.T) {
	s, ctx := newStore(t)
	city, place := regionalCity(t, s, ctx, "Lisle")
	for range 3 {
		topic, err := s.CreateTopic(ctx, models.CreateTopicRequest{CityID: testdb.SeedCityID, CityIDs: []string{city.ID}, Name: "Concurrent theme"})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := s.CreateMoment(ctx, models.CreateMomentRequest{PlaceID: place.ID, Title: "Concurrent history", TopicIDs: []string{topic.ID}})
			results <- err
		}()
		go func() {
			<-start
			_, err := s.UpdateTopic(ctx, topic.ID, models.UpdateTopicRequest{CityIDs: []string{}})
			results <- err
		}()
		close(start)
		winners := 0
		for range 2 {
			err := <-results
			if err == nil {
				winners++
			} else if !errors.Is(err, ErrInvalidReference) {
				t.Fatal(err)
			}
		}
		if winners != 1 {
			t.Fatalf("concurrent writes violated coverage: %d succeeded", winners)
		}
	}
}

func TestRegionalTopicCoverageSerializesWithTourCreation(t *testing.T) {
	s, ctx := newStore(t)
	city, place := regionalCity(t, s, ctx, "Lisle")
	for range 3 {
		topic, err := s.CreateTopic(ctx, models.CreateTopicRequest{CityID: testdb.SeedCityID, CityIDs: []string{city.ID}, Name: "Concurrent tour theme"})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := s.CreateTour(ctx, CreateTourInput{CityID: testdb.SeedCityID, Title: "Concurrent regional tour", TopicID: &topic.ID, Stops: []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: place.ID}}})
			results <- err
		}()
		go func() {
			<-start
			_, err := s.UpdateTopic(ctx, topic.ID, models.UpdateTopicRequest{CityIDs: []string{}})
			results <- err
		}()
		close(start)
		winners := 0
		for range 2 {
			err := <-results
			if err == nil {
				winners++
			} else if !errors.Is(err, ErrInvalidReference) {
				t.Fatal(err)
			}
		}
		if winners != 1 {
			t.Fatalf("concurrent tour writes violated coverage: %d succeeded", winners)
		}
	}
}

func TestPublicPlaceSlugIgnoresDraftsInOtherCities(t *testing.T) {
	s, ctx := newStore(t)
	seed, err := s.GetPlace(ctx, testdb.SeedPlaceStationID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := regionalCity(t, s, ctx, "Lisle")
	if _, err := s.CreatePlace(ctx, models.CreatePlaceRequest{CityID: other.ID, Slug: seed.Slug, Name: "Unpublished namesake", Latitude: 41.8, Longitude: -88.1}); err != nil {
		t.Fatal(err)
	}
	public, err := s.GetPlace(ctx, seed.Slug, "", false)
	if err != nil || public.ID != seed.ID {
		t.Fatalf("a draft elsewhere must not make the public slug ambiguous: %+v %v", public, err)
	}
	if _, err := s.PublicHappenings(ctx, seed.Slug, 10, 0, time.Now()); err != nil {
		t.Fatalf("public happenings by slug: %v", err)
	}
	if _, err := s.GetPlace(ctx, seed.Slug, "", true); !errors.Is(err, ErrConflict) {
		t.Fatalf("curator lookups still see both places: %v", err)
	}
}
