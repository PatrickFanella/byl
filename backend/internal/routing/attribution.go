package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"invisible-museum/backend/internal/models"
)

// InspectPath performs one strict request per original story leg. A refusal or
// omission becomes an explicit unknown checkpoint, without resnapping or retry.
func (v *Valhalla) InspectPath(ctx context.Context, mode models.TravelMode, route Route) (*models.PathAttribution, error) {
	out := &models.PathAttribution{Provider: "valhalla", Method: "trace_attributes/edge_walk", Legs: []models.AttributedLeg{}}
	if !mode.Valid() || len(route.Legs) < 1 || len(route.Legs) > 30 {
		out.Failure = "invalid_attribution_request"
		return out, nil
	}
	costing := "pedestrian"
	if mode == models.TravelModeBike {
		costing = "bicycle"
	}
	points, edges := 0, 0
	for _, leg := range route.Legs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		shape := leg.EncodedShape
		if shape == "" {
			shape = EncodePolyline6(leg.Geometry)
		}
		if len(leg.Geometry) < 2 || len(shape) > 1<<20 {
			out.Failure = "attribution_limit"
			return out, nil
		}
		body, _ := json.Marshal(map[string]any{"encoded_polyline": shape, "costing": costing, "shape_match": "edge_walk", "units": "kilometers", "filters": map[string]any{"action": "include", "attributes": []string{"shape", "osm_changeset", "edge.id", "edge.way_id", "edge.begin_osm_node_id", "edge.end_osm_node_id", "edge.forward", "edge.begin_shape_index", "edge.end_shape_index", "edge.use", "edge.traversability", "edge.travel_mode", "edge.bridge", "edge.tunnel"}}})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+"/trace_attributes", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Byl-path-quality/1")
		response, err := v.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			out.Failure = "attribution_unavailable"
			return out, nil
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
		_ = response.Body.Close()
		if readErr != nil || len(raw) > 4<<20 {
			out.Failure = "attribution_response_limit"
			return out, nil
		}
		if response.StatusCode != http.StatusOK {
			out.Failure = "exact_match_unavailable"
			return out, nil
		}
		var result struct {
			Shape        string `json:"shape"`
			OSMChangeset int64  `json:"osm_changeset"`
			Edges        []struct {
				ID      int64 `json:"id"`
				WayID   int64 `json:"way_id"`
				NodeID  int64 `json:"node_id"`
				EndNode struct {
					NodeID int64 `json:"node_id"`
				} `json:"end_node"`
				Forward         *bool  `json:"forward"`
				Use             string `json:"use"`
				Traversability  string `json:"traversability"`
				TravelMode      string `json:"travel_mode"`
				Bridge          bool   `json:"bridge"`
				Tunnel          bool   `json:"tunnel"`
				BeginShapeIndex *int   `json:"begin_shape_index"`
				EndShapeIndex   *int   `json:"end_shape_index"`
			} `json:"edges"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Shape == "" {
			out.Failure = "attribution_unreadable"
			return out, nil
		}
		attributed := models.AttributedLeg{Geometry: DecodePolyline6(result.Shape), OSMChangeset: result.OSMChangeset, Edges: []models.PathEdge{}}
		points += len(attributed.Geometry)
		edges += len(result.Edges)
		if points > 16384 || edges > 1024 {
			out.Failure = "attribution_limit"
			return out, nil
		}
		for _, edge := range result.Edges {
			if edge.Forward == nil || edge.BeginShapeIndex == nil || edge.EndShapeIndex == nil {
				out.Failure = "attribution_incomplete"
				return out, nil
			}
			attributed.Edges = append(attributed.Edges, models.PathEdge{ID: edge.ID, WayID: edge.WayID, BeginNodeID: edge.NodeID, EndNodeID: edge.EndNode.NodeID, Forward: *edge.Forward, Use: edge.Use, Traversability: edge.Traversability, TravelMode: edge.TravelMode, Bridge: edge.Bridge, Tunnel: edge.Tunnel, BeginShapeIndex: *edge.BeginShapeIndex, EndShapeIndex: *edge.EndShapeIndex})
		}
		out.Legs = append(out.Legs, attributed)
	}
	return out, nil
}
