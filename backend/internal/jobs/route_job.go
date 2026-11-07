package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	inspections := []models.PathInspection{}
	failure := func(cause error, route routing.Route) error {
		report := routing.AssessPathQuality(tour, p.Mode, route, nil, inspections, digest, w.now())
		report.Status = "blocked"
		report.Issues = append(report.Issues, models.PathQualityIssue{Code: "route_computation_failed", Severity: "blocked", Origin: "unknown", Detail: "No suitable computed path is available for this mode; review the route error."})
		return routeInputFailure{cause: cause, digest: digest, quality: &report}
	}
	inspections, err = w.store.ApprovedPathInspections(ctx, tour)
	if err != nil {
		return failure(fmt.Errorf("load retained path inspections: %w", err), routing.Route{})
	}
	plan, err := routing.BuildPlan(tour, p.Mode)
	if err != nil {
		return failure(fmt.Errorf("%w: %w", ErrNoRetry, err), routing.Route{})
	}
	var route routing.Route
	if plan.Controlled {
		provider, ok := w.router.(routing.ControlledRouter)
		if !ok {
			return failure(fmt.Errorf("%w: provider_does_not_support_approved_navigation", ErrNoRetry), routing.Route{})
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
		return failure(err, route)
	}
	if len(route.Legs) != len(plan.Legs) {
		return failure(fmt.Errorf("%w: provider_story_leg_count_mismatch", ErrNoRetry), route)
	}
	var attribution *models.PathAttribution
	if inspector, ok := w.router.(routing.PathInspector); ok {
		inspectionContext, cancel := context.WithTimeout(ctx, 60*time.Second)
		attribution, err = inspector.InspectPath(inspectionContext, p.Mode, route)
		cancel()
		if ctx.Err() != nil {
			return failure(ctx.Err(), route)
		}
		if err != nil {
			attribution = &models.PathAttribution{Provider: route.Provider, Method: "trace_attributes/edge_walk", Failure: "attribution_unavailable"}
		}
	}
	quality := routing.AssessPathQuality(tour, p.Mode, route, attribution, inspections, digest, w.now())
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
	quality.RouteSHA256 = tr.RoutingFingerprint()
	if !quality.Valid() {
		quality.Attribution = &models.PathAttribution{Provider: route.Provider, Method: "trace_attributes/edge_walk", Failure: "checkpoint_limit"}
		quality.Issues = append(quality.Issues, models.PathQualityIssue{Code: "quality_checkpoint_limit", Severity: "needs_research", Origin: "unknown", Detail: "Network evidence exceeds the bounded checkpoint; review a smaller itinerary."})
		if quality.Status == "pass" {
			quality.Status = "needs_research"
		}
	}
	err = w.store.UpsertRouteQualified(ctx, tr, p.RouteVersion, digest, quality)
	if errors.Is(err, repository.ErrRouteSuperseded) {
		return err
	}
	return err
}

// routeInputFailure retains a digest for safe final-failure handling, never coordinates.
type routeInputFailure struct {
	cause   error
	digest  string
	quality *models.PathQualityReport
}

func (e routeInputFailure) Error() string { return e.cause.Error() }
func (e routeInputFailure) Unwrap() error { return e.cause }
