package routing

import (
	"slices"
	"time"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

// AssessPathQuality separates source inspections from provider assertions. It
// checks configured evidence for review; it cannot certify current trail access.
func AssessPathQuality(tour *models.TourDetail, mode models.TravelMode, route Route, attribution *models.PathAttribution, inspections []models.PathInspection, digest string, now time.Time) models.PathQualityReport {
	r := models.PathQualityReport{PathQualitySummary: models.PathQualitySummary{Version: 1, Status: "pass", Issues: []models.PathQualityIssue{}, CheckedAt: now, InputCurrent: true}, InputHash: digest, Mode: mode, Provider: route.Provider, Inspections: inspections, Attribution: attribution}
	seenIssues := map[string]bool{}
	add := func(code, severity, origin, segment, detail string) {
		key := code + ":" + severity + ":" + origin + ":" + segment
		if seenIssues[key] {
			return
		}
		seenIssues[key] = true
		r.Issues = append(r.Issues, models.PathQualityIssue{Code: code, Severity: severity, Origin: origin, SegmentID: segment, Detail: detail})
		if severity == "blocked" || severity == "needs_research" && r.Status == "pass" {
			r.Status = severity
		}
	}
	if route.Provider == "straightline" {
		add("development_geometry", "blocked", "provider_assertion", "", "Development straight-line geometry is unsuitable for publication quality.")
	} else if route.Provider != "valhalla" {
		add("unqualified_provider", "needs_research", "unknown", "", "The routing provider has no qualified network attribution contract.")
	}
	for _, stop := range tour.Stops {
		if a := stop.Place.SelectArrival(mode); a.NavigationPoint == nil {
			add("arrival_"+a.Eligibility, "blocked", "curator_assertion", "", "Each story stop requires an independently allowed arrival for this mode.")
		}
	}
	hasInspection := func(segment, purpose string) bool {
		return slices.ContainsFunc(inspections, func(i models.PathInspection) bool {
			return i.SegmentID == segment && i.Purpose == purpose && (purpose == "network" || i.Mode == mode) && i.EvidenceID != "" && len(i.ContentSHA256) == 64 && !i.RetrievedAt.IsZero() && i.Quote != ""
		})
	}
	var expected []qualityWay
	var requiredNodes []int64
	if tour.Corridor == nil || tour.Corridor.Intent == nil {
		add("corridor_reference_missing", "needs_research", "unknown", "", "Provide a reviewed network reference and inspected access evidence.")
	} else {
		intent := tour.Corridor.Intent
		segments := map[string]models.CorridorSegment{}
		for _, segment := range intent.Segments {
			segments[segment.ID] = segment
		}
		index := slices.IndexFunc(intent.Routes, func(r models.ModeCorridor) bool { return r.Mode == mode })
		if index < 0 {
			add("corridor_mode_missing", "blocked", "curator_assertion", "", "This mode has no selected corridor path.")
		} else {
			selection := intent.Routes[index]
			var previousNode int64
			for _, traversal := range selection.Path {
				segment, exists := segments[traversal.SegmentID]
				if !exists {
					add("missing_segment", "blocked", "curator_assertion", traversal.SegmentID, "The selected reference segment is missing.")
					continue
				}
				if !slices.Contains(segment.Modes, mode) {
					add("forbidden_mode", "blocked", "curator_assertion", segment.ID, "The reference does not allow this mode on the selected segment.")
				}
				if segment.Kind == "road_connector" && (!slices.Contains(selection.AllowedConnectors, segment.ID) || segment.Source == nil) {
					add("missing_connector", "blocked", "curator_assertion", segment.ID, "A road connector requires explicit selection and its own source context.")
				}
				accessIndex := slices.IndexFunc(segment.Access, func(a models.CorridorAccess) bool { return a.Mode == mode })
				if accessIndex < 0 || segment.Access[accessIndex].Status == "unknown" {
					add("access_unknown", "needs_research", "unknown", segment.ID, "Required mode-specific access evidence is unresolved.")
				} else if status := segment.Access[accessIndex].Status; status == "closed" || status == "restricted" {
					code := "access_restricted"
					if status == "closed" {
						code = "known_closure"
					}
					add(code, "blocked", "curator_assertion", segment.ID, "The reviewed access decision forbids this traversal.")
				}
				if !hasInspection(segment.ID, "access") {
					add("access_not_inspected", "needs_research", "unknown", segment.ID, "Retain an inspected source passage supporting this mode's access decision.")
				}
				if segment.Network == nil {
					add("network_reference_missing", "needs_research", "unknown", segment.ID, "Network way and junction identities have not been reviewed.")
					continue
				}
				if !hasInspection(segment.ID, "network") {
					add("network_not_inspected", "needs_research", "unknown", segment.ID, "Retain an inspected source passage for network identity and connectivity.")
				}
				from, to := segment.Network.FromNodeID, segment.Network.ToNodeID
				ways := slices.Clone(segment.Network.Ways)
				if traversal.Reverse {
					from, to = to, from
					slices.Reverse(ways)
				}
				if previousNode != 0 && from != previousNode {
					add("unconnected_reference_nodes", "blocked", "curator_assertion", segment.ID, "Reference junction identities disagree; coordinate overlap does not establish connectivity.")
				}
				if len(requiredNodes) == 0 {
					requiredNodes = append(requiredNodes, from)
				}
				requiredNodes = append(requiredNodes, to)
				previousNode = to
				for _, way := range ways {
					forward := !way.Reverse
					if traversal.Reverse {
						forward = !forward
					}
					expected = appendQualityWay(expected, qualityWay{id: way.ID, forward: forward, segment: segment.ID, kind: segment.Kind, from: from, to: to})
				}
			}
			if len(intent.Validate()) > 0 || intent.ValidateStops(tour) != nil {
				add("invalid_corridor_reference", "blocked", "curator_assertion", "", "The reviewed geometry, topology or story references need correction.")
			}
		}
	}
	if attribution == nil || attribution.Failure != "" || attribution.Provider != "valhalla" || attribution.Method != "trace_attributes/edge_walk" || len(attribution.Legs) != len(route.Legs) || len(route.Legs) == 0 {
		add("strict_attribution_unavailable", "needs_research", "unknown", "", "Strict network attribution is unavailable; no alternate path or map-snap result was accepted.")
		return r
	}
	var actual []qualityWay
	var actualNodes []int64
	identitiesComplete := true
	var previous *models.PathEdge
	for i, leg := range attribution.Legs {
		if !attributionGeometryMatches(route.Legs[i].Geometry, leg.Geometry) {
			add("attribution_geometry_changed", "blocked", "provider_assertion", "", "Attribution does not describe the computed story-leg geometry.")
		}
		if len(leg.Edges) == 0 || leg.OSMChangeset <= 0 {
			add("provider_network_version_missing", "needs_research", "unknown", "", "Provider network edges or dataset version are missing.")
			if len(leg.Edges) == 0 {
				identitiesComplete = false
			}
		}
		lastShape := 0
		for j, edge := range leg.Edges {
			if edge.WayID <= 0 || edge.ID <= 0 || edge.BeginNodeID <= 0 || edge.EndNodeID <= 0 {
				identitiesComplete = false
				add("provider_identity_missing", "needs_research", "unknown", "", "Provider way, edge or junction identities are incomplete.")
			}
			if edge.BeginShapeIndex != lastShape || edge.EndShapeIndex <= edge.BeginShapeIndex || edge.EndShapeIndex >= len(leg.Geometry) {
				add("provider_shape_gap", "blocked", "provider_assertion", "", "Provider edge ranges omit or overlap computed travel.")
			}
			lastShape = edge.EndShapeIndex
			wantMode := "pedestrian"
			if mode == models.TravelModeBike {
				wantMode = "bicycle"
			}
			if edge.Use == "" {
				add("provider_use_missing", "needs_research", "unknown", "", "Provider network use is missing.")
			}
			if edge.TravelMode == "" {
				add("provider_mode_missing", "needs_research", "unknown", "", "Provider travel mode is missing.")
			} else if edge.TravelMode != wantMode {
				add("provider_mode_forbidden", "blocked", "provider_assertion", "", "Provider travel mode differs from the requested mode.")
			}
			if edge.Traversability != "forward" && edge.Traversability != "both" {
				add("provider_access_unresolved", "needs_research", "provider_assertion", "", "Provider traversability does not establish travel in the returned direction.")
			}
			if mode == models.TravelModeBike && edge.Use == "steps" {
				add("cycling_steps", "blocked", "provider_assertion", "", "A cycling route cannot silently substitute carrying or walking a bicycle.")
			}
			if previous != nil && previous.EndNodeID > 0 && edge.BeginNodeID > 0 && previous.EndNodeID != edge.BeginNodeID {
				// A story boundary may split one directed edge into two partial
				// results; its identity and adjoining geometry must still agree.
				continuation := j == 0 && previous.ID == edge.ID && previous.Forward == edge.Forward && i > 0 && touchingLegs(attribution.Legs[i-1].Geometry, leg.Geometry)
				if !continuation {
					code := "disconnected_provider_edges"
					if previous.Bridge || previous.Tunnel || edge.Bridge || edge.Tunnel {
						code = "grade_separated_crossing"
					}
					add(code, "blocked", "provider_assertion", "", "Consecutive network edges do not share a junction; proximity cannot supply a connection.")
				}
			}
			actual = appendQualityWay(actual, qualityWay{id: edge.WayID, forward: edge.Forward, kind: edge.Use, from: edge.BeginNodeID, to: edge.EndNodeID})
			copy := edge
			previous = &copy
			if len(actualNodes) == 0 || actualNodes[len(actualNodes)-1] != edge.BeginNodeID {
				actualNodes = append(actualNodes, edge.BeginNodeID)
			}
			actualNodes = append(actualNodes, edge.EndNodeID)
		}
		if lastShape != len(leg.Geometry)-1 {
			add("provider_shape_gap", "blocked", "provider_assertion", "", "Network attribution does not cover the complete story leg.")
		}
	}
	if len(expected) > 0 && identitiesComplete {
		if len(actual) != len(expected) {
			add("unexpected_network_path", "blocked", "provider_assertion", "", "Returned way sequence differs from the selected references and connectors.")
		}
		for i, way := range actual {
			if i >= len(expected) {
				break
			}
			ref := expected[i]
			if way.id != ref.id || way.forward != ref.forward {
				add("unexpected_network_way", "blocked", "provider_assertion", ref.segment, "A parallel road, different way or reversed traversal cannot replace the selected network identity.")
			}
			if ref.kind == "trail" && isRoadUse(way.kind) {
				add("unapproved_road_departure", "blocked", "provider_assertion", ref.segment, "A road section needs a separately selected and sourced connector.")
			}
		}
	}
	nodeIndex := 0
	for _, node := range requiredNodes {
		if !identitiesComplete {
			break
		}
		for nodeIndex < len(actualNodes) && actualNodes[nodeIndex] != node {
			nodeIndex++
		}
		if nodeIndex == len(actualNodes) {
			add("source_junction_not_on_path", "blocked", "provider_assertion", "", "A reviewed junction is absent from the ordered provider node chain.")
			break
		}
		nodeIndex++
	}
	return r
}

type qualityWay struct {
	id            int64
	forward       bool
	segment, kind string
	from, to      int64
}

func appendQualityWay(ways []qualityWay, way qualityWay) []qualityWay {
	if len(ways) > 0 && ways[len(ways)-1].id == way.id && ways[len(ways)-1].forward == way.forward {
		ways[len(ways)-1].to = way.to
		if isRoadUse(way.kind) {
			ways[len(ways)-1].kind = way.kind
		}
		return ways
	}
	return append(ways, way)
}

func isRoadUse(use string) bool {
	return slices.Contains([]string{"road", "ramp", "turn_channel", "driveway", "alley", "parking_aisle", "emergency_access", "drive_through", "culdesac"}, use)
}

func touchingLegs(a, b [][2]float64) bool {
	return len(a) > 0 && len(b) > 0 && geo.DistanceMeters(a[len(a)-1][1], a[len(a)-1][0], b[0][1], b[0][0]) <= 3
}

func attributionGeometryMatches(a, b [][2]float64) bool {
	if len(a) < 2 || len(b) < 2 || !touchingLegs(a[:1], b) || !touchingLegs(a, b[len(b)-1:]) {
		return false
	}
	for _, pair := range [][2][][2]float64{{a, b}, {b, a}} {
		for _, point := range pair[0] {
			if !finitePoint(Point{Lat: point[1], Lng: point[0]}) || !geo.NearPolyline(point, pair[1], 3) {
				return false
			}
		}
	}
	return true
}
