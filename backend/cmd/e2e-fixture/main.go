//go:build e2e

// Command e2e-fixture prepares a clearly synthetic, unlisted route for browser
// report tests. It is excluded from ordinary builds and production images.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"invisible-museum/backend/internal/db"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func main() {
	if err := prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare() error {
	dsn := os.Getenv("DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || os.Getenv("BYL_E2E_FIXTURE") != "1" || parsed.User == nil || parsed.User.Username() != "museum" || parsed.Path != "/museum_test" {
		return errors.New("e2e fixture requires the explicitly enabled isolated museum_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := repository.New(pool)
	if _, err := store.GetTour(ctx, "e2e-route-report-fixture", false); err == nil {
		return nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	seed, err := store.GetTour(ctx, "downtown-memory-loop", false)
	if err != nil {
		return err
	}
	stops := []models.StopInput{}
	for _, stop := range seed.Stops {
		stops = append(stops, models.StopInput{PlaceID: stop.Place.ID, FeaturedMomentID: stop.FeaturedMomentID, Note: stop.Note})
	}
	tour, err := store.CreateTour(ctx, repository.CreateTourInput{CityID: seed.CityID, Slug: "e2e-route-report-fixture", Title: "Synthetic route report fixture", Summary: "Synthetic browser persistence fixture. No inspected route or real-world access claim.", Modes: []models.TravelMode{models.TravelModeWalk}, Status: models.EntityStatusDraft, Visibility: models.TourVisibilityUnlisted, Stops: stops})
	if err != nil {
		return err
	}
	// The API starts after this program. Retire only this fixture's queued route
	// computation so the development straight-line worker cannot replace its stamp.
	if _, err = pool.Exec(ctx, `DELETE FROM jobs WHERE kind='compute_route' AND payload->>'tour_id'=$1`, tour.ID); err != nil {
		return err
	}
	_, digest, err := store.TourNavigationInputs(ctx, tour.ID, models.TravelModeWalk, tour.RouteVersion)
	if err != nil {
		return err
	}
	route := models.TourRoute{TourID: tour.ID, Mode: models.TravelModeWalk, Status: models.RouteStatusReady, Provider: "valhalla", DistanceM: 1000, DurationS: 300, Geometry: &models.LineString{Type: "LineString", Coordinates: [][2]float64{}}, Legs: []models.RouteLeg{}}
	attribution := &models.PathAttribution{Provider: "valhalla", Method: "trace_attributes/edge_walk", Legs: []models.AttributedLeg{}}
	for i, stop := range tour.Stops {
		point := [2]float64{stop.Place.Longitude, stop.Place.Latitude}
		route.Geometry.Coordinates = append(route.Geometry.Coordinates, point)
		if i == 0 {
			continue
		}
		previous := tour.Stops[i-1]
		geometry := [][2]float64{{previous.Place.Longitude, previous.Place.Latitude}, point}
		route.Legs = append(route.Legs, models.RouteLeg{FromStopID: previous.ID, ToStopID: stop.ID, FromPosition: previous.Position, ToPosition: stop.Position, Geometry: geometry, DistanceM: 1000 / float64(len(tour.Stops)-1), DurationS: 100})
		attribution.Legs = append(attribution.Legs, models.AttributedLeg{Geometry: geometry})
	}
	// This is an explicit synthetic checkpoint, like the HTTP persistence fixtures.
	// It exercises reporting/publication contracts, not network or access assessment.
	quality := models.PathQualityReport{PathQualitySummary: models.PathQualitySummary{Version: 1, Status: "pass", Issues: []models.PathQualityIssue{}, CheckedAt: time.Now().UTC(), InputCurrent: true}, InputHash: digest, Mode: models.TravelModeWalk, Provider: "valhalla", RouteSHA256: route.RoutingFingerprint(), Attribution: attribution, Inspections: []models.PathInspection{{SegmentID: "synthetic-browser", Purpose: "network", EvidenceID: "synthetic-browser-inspection", URL: "https://example.org/synthetic-browser-fixture", ContentSHA256: strings.Repeat("a", 64), RetrievedAt: time.Now().UTC(), Reference: "fixture", Quote: "Synthetic browser persistence checkpoint. Not a verified path or real-world access evidence."}}}
	if err = store.UpsertRouteQualified(ctx, route, tour.RouteVersion, digest, quality); err != nil {
		return err
	}
	_, err = store.PublishTourRevision(ctx, tour.ID, "", models.PublishRevisionRequest{DraftVersion: tour.DraftVersion, CurrentRevisionID: tour.RevisionID})
	return err
}
