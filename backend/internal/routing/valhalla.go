package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

// Valhalla calls a self-hosted Valhalla routing server.
type Valhalla struct {
	baseURL string
	client  *http.Client
}

// NewValhalla builds a client for the server at baseURL.
func NewValhalla(baseURL string, client *http.Client) Router {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Valhalla{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

type valhallaResponse struct {
	Trip struct {
		Summary struct {
			Length float64 `json:"length"`
			Time   float64 `json:"time"`
		} `json:"summary"`
		Legs []struct {
			Summary struct {
				Length float64 `json:"length"`
				Time   float64 `json:"time"`
			} `json:"summary"`
			Shape string `json:"shape"`
		} `json:"legs"`
	} `json:"trip"`
	Error string `json:"error"`
}

func (v *Valhalla) Route(ctx context.Context, mode models.TravelMode, points []Point) (Route, error) {
	controls := make([]Waypoint, 0, len(points))
	for _, point := range points {
		controls = append(controls, Waypoint{Point: point, Break: true})
	}
	return v.route(ctx, mode, controls, false, false)
}

// RouteControlled retains break boundaries while via points shape each leg.
func (v *Valhalla) RouteControlled(ctx context.Context, mode models.TravelMode, request ControlledRequest) (Route, error) {
	return v.route(ctx, mode, request.Controls, true, request.FollowReference)
}

func (v *Valhalla) route(ctx context.Context, mode models.TravelMode, controls []Waypoint, controlled, followReference bool) (Route, error) {
	if len(controls) < 2 {
		return Route{}, ErrTooFewPoints
	}
	if !mode.Valid() || len(controls) > maxControls || !controls[0].Break || !controls[len(controls)-1].Break {
		return Route{}, fmt.Errorf("valhalla: invalid routing controls")
	}
	costing := "pedestrian"
	if mode == models.TravelModeBike {
		costing = "bicycle"
	}
	locations := make([]map[string]any, 0, len(controls))
	breaks := 0
	for _, p := range controls {
		if !finitePoint(p.Point) {
			return Route{}, fmt.Errorf("valhalla: invalid routing coordinate")
		}
		kind := "via"
		if p.Break {
			kind = "break"
			breaks++
		}
		location := map[string]any{"lat": p.Lat, "lon": p.Lng, "type": kind}
		if controlled {
			location["radius"], location["search_cutoff"] = 20, 25
		}
		locations = append(locations, location)
	}
	body, _ := json.Marshal(map[string]any{"locations": locations, "costing": costing, "units": "kilometers"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+"/route", bytes.NewReader(body))
	if err != nil {
		return Route{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Byl-routing/1")
	resp, err := v.client.Do(req)
	if err != nil {
		return Route{}, fmt.Errorf("valhalla: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil || len(raw) > 16<<20 {
		return Route{}, fmt.Errorf("valhalla: unreadable or oversized response")
	}
	var out valhallaResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return Route{}, fmt.Errorf("valhalla: status %d: unreadable response", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		msg := out.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return Route{}, fmt.Errorf("valhalla: status %d: %s", resp.StatusCode, msg)
	}
	if len(out.Trip.Legs) != breaks-1 {
		return Route{}, fmt.Errorf("valhalla: story leg count mismatch: got %d, want %d", len(out.Trip.Legs), breaks-1)
	}
	r := Route{Provider: "valhalla", DistanceM: out.Trip.Summary.Length * 1000, DurationS: int(math.Round(out.Trip.Summary.Time))}
	for _, leg := range out.Trip.Legs {
		geom := DecodePolyline6(leg.Shape)
		r.Legs = append(r.Legs, Leg{DistanceM: leg.Summary.Length * 1000, DurationS: int(math.Round(leg.Summary.Time)), Geometry: geom, EncodedShape: leg.Shape})
		r.Geometry = append(r.Geometry, geom...)
	}
	if len(r.Legs) == 0 {
		return Route{}, fmt.Errorf("valhalla: response had no legs")
	}
	if controlled {
		if err := validateControlledRoute(controls, r, followReference); err != nil {
			return Route{}, err
		}
	}
	return r, nil
}

// These are protocol/geometry guards, not proof of public access or network
// topology. Broader corridor quality assessment belongs to issue #29.
func validateControlledRoute(controls []Waypoint, route Route, followReference bool) error {
	if !validMetrics(route.DistanceM, route.DurationS) {
		return fmt.Errorf("valhalla: invalid route metrics")
	}
	start, legIndex := 0, 0
	legDistance, legDuration := 0.0, 0
	for i := 1; i < len(controls); i++ {
		if !controls[i].Break {
			continue
		}
		leg := route.Legs[legIndex]
		legDistance += leg.DistanceM
		legDuration += leg.DurationS
		legIndex++
		if len(leg.Geometry) < 2 || !validMetrics(leg.DistanceM, leg.DurationS) {
			return fmt.Errorf("valhalla: missing story leg geometry or metrics")
		}
		var path occurrence
		progress := 0.0
		for j, coordinate := range leg.Geometry {
			point := Point{Lat: coordinate[1], Lng: coordinate[0]}
			if !finitePoint(point) {
				return fmt.Errorf("valhalla: invalid story leg geometry")
			}
			if j > 0 {
				progress += distance(path.points[j-1].point, point)
			}
			path.points = append(path.points, pathPoint{point, progress})
		}
		if distance(controls[start].Point, path.points[0].point) > referenceToleranceM || distance(controls[i].Point, path.points[len(path.points)-1].point) > referenceToleranceM {
			return fmt.Errorf("valhalla: story endpoint mismatch")
		}
		last, requestedDistance := -1.0, 0.0
		var reference [][2]float64
		for j := start; j <= i; j++ {
			point := controls[j].Point
			reference = append(reference, [2]float64{point.Lng, point.Lat})
			if j > start {
				requestedDistance += distance(controls[j-1].Point, point)
			}
			candidates := projectOccurrences(point, []occurrence{path})
			found := false
			for _, p := range candidates {
				if p >= last-1 {
					last, found = p, true
					break
				}
			}
			if !found {
				return fmt.Errorf("valhalla: missing or reversed routing control")
			}
		}
		for _, coordinate := range leg.Geometry {
			if followReference && !geo.NearPolyline(coordinate, reference, referenceToleranceM) {
				return fmt.Errorf("valhalla: route left requested control envelope")
			}
		}
		if progress+25 < requestedDistance*0.9 || leg.DistanceM+25 < requestedDistance*0.9 {
			return fmt.Errorf("valhalla: route omitted requested travel")
		}
		start = i
	}
	if math.Abs(legDistance-route.DistanceM) > max(5, float64(len(route.Legs))*2) || absDuration(legDuration-route.DurationS) > len(route.Legs)+1 {
		return fmt.Errorf("valhalla: route totals omitted story travel")
	}
	return nil
}

func absDuration(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func validMetrics(distance float64, duration int) bool {
	return !math.IsNaN(distance) && !math.IsInf(distance, 0) && distance > 0 && distance < 1e8 && duration >= 0 && duration < 1e8
}
