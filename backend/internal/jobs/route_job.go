package jobs

import (
	"context"
	"errors"
	"fmt"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
)

func (w *Worker) computeRoute(ctx context.Context, p models.ComputeRoutePayload) error {
	if !p.Mode.Valid() {
		return fmt.Errorf("invalid mode %q", p.Mode)
	}
	pts, digest, err := w.store.TourRouteInputs(ctx, p.TourID, p.Mode, p.RouteVersion)
	if errors.Is(err, repository.ErrRouteSuperseded) {
		return err
	}
	if err != nil {
		return fmt.Errorf("load stops: %w", err)
	}
	if len(pts) < 2 {
		return routeInputFailure{cause: fmt.Errorf("tour has %d stops; a route needs at least two", len(pts)), digest: digest}
	}
	points := make([]routing.Point, 0, len(pts))
	for _, pt := range pts {
		points = append(points, routing.Point{Lat: pt.Lat, Lng: pt.Lng})
	}
	route, err := w.router.Route(ctx, p.Mode, points)
	if err != nil {
		return routeInputFailure{cause: err, digest: digest}
	}
	tr := models.TourRoute{
		TourID: p.TourID, Mode: p.Mode, Status: models.RouteStatusReady, Provider: route.Provider,
		DistanceM: route.DistanceM, DurationS: route.DurationS,
		Geometry: &models.LineString{Type: "LineString", Coordinates: route.Geometry},
		Legs:     make([]models.RouteLeg, 0, len(route.Legs)),
	}
	for i, leg := range route.Legs {
		tr.Legs = append(tr.Legs, models.RouteLeg{FromPosition: i + 1, ToPosition: i + 2, DistanceM: leg.DistanceM, DurationS: leg.DurationS, Geometry: leg.Geometry})
	}
	err = w.store.UpsertRouteVersioned(ctx, tr, p.RouteVersion, digest)
	if errors.Is(err, repository.ErrRouteSuperseded) {
		return err
	}
	return err
}

// routeInputFailure retains a digest for safe final-failure handling, never coordinates.
type routeInputFailure struct {
	cause  error
	digest string
}

func (e routeInputFailure) Error() string { return e.cause.Error() }
func (e routeInputFailure) Unwrap() error { return e.cause }
