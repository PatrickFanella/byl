package jobs

import (
	"context"
	"fmt"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/routing"
)

func (w *Worker) computeRoute(ctx context.Context, p models.ComputeRoutePayload) error {
	if !p.Mode.Valid() {
		return fmt.Errorf("invalid mode %q", p.Mode)
	}
	pts, err := w.store.TourStopPoints(ctx, p.TourID)
	if err != nil {
		return fmt.Errorf("load stops: %w", err)
	}
	if len(pts) < 2 {
		return fmt.Errorf("tour has %d stops; a route needs at least two", len(pts))
	}
	points := make([]routing.Point, 0, len(pts))
	for _, pt := range pts {
		points = append(points, routing.Point{Lat: pt.Lat, Lng: pt.Lng})
	}
	route, err := w.router.Route(ctx, p.Mode, points)
	if err != nil {
		return err
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
	return w.store.UpsertRoute(ctx, tr)
}
