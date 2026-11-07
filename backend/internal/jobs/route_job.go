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
	tour, digest, err := w.store.TourNavigationInputs(ctx, p.TourID, p.Mode, p.RouteVersion)
	if errors.Is(err, repository.ErrRouteSuperseded) {
		return err
	}
	if err != nil {
		return fmt.Errorf("load stops: %w", err)
	}
	plan, err := routing.BuildPlan(tour, p.Mode)
	if err != nil {
		return routeInputFailure{cause: fmt.Errorf("%w: %w", ErrNoRetry, err), digest: digest}
	}
	var route routing.Route
	if plan.Controlled {
		provider, ok := w.router.(routing.ControlledRouter)
		if !ok {
			return routeInputFailure{cause: fmt.Errorf("%w: provider_does_not_support_approved_navigation", ErrNoRetry), digest: digest}
		}
		route, err = provider.RouteControlled(ctx, p.Mode, routing.ControlledRequest{Controls: plan.Controls, FollowReference: plan.FollowReference})
	} else {
		points := make([]routing.Point, 0, len(plan.Controls))
		for _, point := range plan.Controls {
			points = append(points, point.Point)
		}
		route, err = w.router.Route(ctx, p.Mode, points)
	}
	if err != nil {
		return routeInputFailure{cause: err, digest: digest}
	}
	if len(route.Legs) != len(plan.Legs) {
		return routeInputFailure{cause: fmt.Errorf("%w: provider_story_leg_count_mismatch", ErrNoRetry), digest: digest}
	}
	tr := models.TourRoute{
		TourID: p.TourID, Mode: p.Mode, Status: models.RouteStatusReady, Provider: route.Provider,
		DistanceM: route.DistanceM, DurationS: route.DurationS,
		Geometry: &models.LineString{Type: "LineString", Coordinates: route.Geometry},
		Legs:     make([]models.RouteLeg, 0, len(route.Legs)),
	}
	for i, leg := range route.Legs {
		boundary := plan.Legs[i]
		stored := models.RouteLeg{FromPosition: boundary.From, ToPosition: boundary.To, DistanceM: leg.DistanceM, DurationS: leg.DurationS, Geometry: leg.Geometry}
		if plan.Controlled {
			stored.FromStopID, stored.ToStopID = boundary.FromID, boundary.ToID
			stored.NavigationSource, stored.Return = "approved_navigation", boundary.Return
		}
		tr.Legs = append(tr.Legs, stored)
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
