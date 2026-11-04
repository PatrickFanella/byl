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
	if len(points) < 2 {
		return Route{}, ErrTooFewPoints
	}
	costing := "pedestrian"
	if mode == models.TravelModeBike {
		costing = "bicycle"
	}
	locations := make([]map[string]any, 0, len(points))
	for _, p := range points {
		locations = append(locations, map[string]any{"lat": p.Lat, "lon": p.Lng, "type": "break"})
	}
	body, _ := json.Marshal(map[string]any{"locations": locations, "costing": costing, "units": "kilometers"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+"/route", bytes.NewReader(body))
	if err != nil {
		return Route{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return Route{}, fmt.Errorf("valhalla: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
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
	r := Route{Provider: "valhalla", DistanceM: out.Trip.Summary.Length * 1000, DurationS: int(math.Round(out.Trip.Summary.Time))}
	for _, leg := range out.Trip.Legs {
		geom := DecodePolyline6(leg.Shape)
		r.Legs = append(r.Legs, Leg{DistanceM: leg.Summary.Length * 1000, DurationS: int(math.Round(leg.Summary.Time)), Geometry: geom})
		r.Geometry = append(r.Geometry, geom...)
	}
	if len(r.Legs) == 0 {
		return Route{}, fmt.Errorf("valhalla: response had no legs")
	}
	return r, nil
}
