// Package routing computes walking and cycling routes between ordered stops.
package routing

import (
	"context"
	"errors"

	"invisible-museum/backend/internal/models"
)

// Point is a WGS84 coordinate.
type Point struct {
	Lat float64
	Lng float64
}

// Leg is the path between two consecutive stops.
type Leg struct {
	DistanceM float64
	DurationS int
	Geometry  [][2]float64 // [lng, lat]
}

// Route is a full multi-stop route.
type Route struct {
	Provider  string
	DistanceM float64
	DurationS int
	Geometry  [][2]float64 // [lng, lat]
	Legs      []Leg
}

// Router computes a route through the points in order.
type Router interface {
	Route(ctx context.Context, mode models.TravelMode, points []Point) (Route, error)
}

// ErrTooFewPoints is returned for fewer than two points.
var ErrTooFewPoints = errors.New("a route needs at least two points")
