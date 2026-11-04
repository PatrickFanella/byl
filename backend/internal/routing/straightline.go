package routing

import (
	"context"
	"math"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

// Speeds in metres per second used when no road network is available.
const (
	walkSpeed = 1.3
	bikeSpeed = 4.5
)

// StraightLine draws straight segments between stops. Used for development and tests.
type StraightLine struct{}

// NewStraightLine returns the local router.
func NewStraightLine() Router { return StraightLine{} }

func (StraightLine) Route(_ context.Context, mode models.TravelMode, points []Point) (Route, error) {
	if len(points) < 2 {
		return Route{}, ErrTooFewPoints
	}
	speed := walkSpeed
	if mode == models.TravelModeBike {
		speed = bikeSpeed
	}
	r := Route{Provider: "straightline", Geometry: [][2]float64{{points[0].Lng, points[0].Lat}}}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		d := geo.DistanceMeters(a.Lat, a.Lng, b.Lat, b.Lng)
		leg := Leg{DistanceM: d, DurationS: int(math.Round(d / speed)), Geometry: [][2]float64{{a.Lng, a.Lat}, {b.Lng, b.Lat}}}
		r.Legs = append(r.Legs, leg)
		r.Geometry = append(r.Geometry, [2]float64{b.Lng, b.Lat})
		r.DistanceM += d
	}
	r.DurationS = int(math.Round(r.DistanceM / speed))
	return r, nil
}
