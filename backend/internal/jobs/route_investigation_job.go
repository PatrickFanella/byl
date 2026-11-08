package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"time"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/sources"
)

func (w *Worker) investigateRouteReport(ctx context.Context, job *models.Job) (string, error) {
	var p models.InvestigateRouteReportPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return "", ErrNoRetry
	}
	i, err := w.store.GetRouteInvestigation(ctx, "", p.InvestigationID)
	if err != nil || i.Result != nil {
		return "", err
	}
	seed := i.Input.Observations[0].Report.Submitted
	revision, err := w.store.GetTourRevision(ctx, seed.TourID, seed.RevisionID)
	if err != nil {
		return "", err
	}
	result := models.RouteInvestigationResult{
		Status: "review", ComparisonDirection: "published", Before: revision.Snapshot.Routes[seed.Mode],
		AffectedStopIDs: []string{}, Evidence: []models.SourceEvidence{}, Issues: []models.PathQualityIssue{},
		StoriesPreserved: true, CheckedAt: w.now().UTC(),
		Arrivals: []models.ArrivalSelection{}, ReviewedCorridor: revision.Snapshot.Tour.Corridor,
	}
	for _, stop := range revision.Snapshot.Tour.Stops {
		result.Arrivals = append(result.Arrivals, stop.Place.SelectArrival(seed.Mode))
	}
	add := func(code, severity, origin, detail string) {
		result.Issues = append(result.Issues, models.PathQualityIssue{Code: code, Severity: severity, Origin: origin, Detail: detail})
		if severity != "information" {
			result.Status = "needs_research"
		}
	}
	add("observations_unverified", "needs_research", "unknown", "Reports are unverified observations. Grouping and distinct accounts or URLs do not establish independent corroboration.")
	if seed.Direction != "forward" {
		add("report_direction_unresolved", "needs_research", "unknown", "The comparison follows the published leg direction; reverse or unknown reported travel requires separate directional access review.")
	}
	if i.Input.Research != nil {
		add("historical_research_required", "needs_research", "unknown", "New place and historical leads are in the linked sourcing brief; this path proposal retains every existing story and narration.")
	}
	if seed.Affected.Kind != "segment" && seed.Alternative != nil {
		add("stop_alternative_requires_segment", "needs_research", "unknown", "An alternative drawn around a stop remains a lead. Identify a specific segment before using its points as routing controls.")
	}
	urls := []string{}
	for _, observation := range i.Input.Observations {
		if url := observation.Report.Submitted.SourceURL; url != "" {
			urls = append(urls, url)
		}
	}
	if i.Input.Research != nil {
		urls = append(urls, i.Input.Research.SourceLinks...)
	}
	for _, url := range urls {
		if _, _, _, err := sources.NormalizeEvidenceURL(url); err != nil {
			add("source_url_blocked", "needs_research", "unknown", "A submitted source URL cannot be retrieved safely; its original lead remains private on the observation or research input.")
		}
	}
	pending := false
	for _, id := range i.Input.EvidenceIDs {
		e, err := w.store.GetSourceEvidence(ctx, id)
		if err != nil {
			return "", err
		}
		result.Evidence = append(result.Evidence, *e)
		if e.Result == nil {
			pending = true
			add("source_lookup_pending", "needs_research", "unknown", "A source lookup has no completed snapshot; partial research cannot establish access or history.")
		} else if e.Result.Outcome != "inspected_page" {
			add("source_lookup_failed", "needs_research", "unknown", "A source could not be inspected as a readable page; review its retained retrieval outcome.")
		} else {
			add("source_interpretation_required", "needs_research", "inspected_source", "The linked page was inspected, but its relevance, currency and mode-specific meaning require curator review.")
		}
	}
	if pending && job.Attempts < job.MaxAttempts {
		return "", errors.New("investigation_sources_pending")
	}
	if len(i.Input.EvidenceIDs) == 0 {
		add("supporting_sources_missing", "needs_research", "unknown", "No retrievable mapping or operator source is available for the reported alteration.")
	}
	// Provider calls have one bounded deadline, including strict attribution.
	routeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	after, affected, computeErr := w.investigationAlternative(routeCtx, result.Before, &revision.Snapshot.Tour, seed)
	result.AffectedStopIDs = affected
	if computeErr != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		add("alternative_unavailable", "blocked", "unknown", "No complete alternative was computed for the affected legs. Review arrival anchors, provider capability and mode restrictions; no shortcut was accepted.")
	} else {
		result.After = after
		distance, duration := after.DistanceM-result.Before.DistanceM, after.DurationS-result.Before.DurationS
		result.DistanceChangeM, result.DurationChangeS = &distance, &duration
		if seed.Alternative != nil || seed.Affected.Kind == "entrance" && seed.Annotation != nil {
			add("report_controls_unreviewed", "needs_research", "unknown", "Submitted alternative points or an entrance annotation guided computation only; they are not approved access or a new public arrival.")
		}
		inspections, err := w.store.ApprovedPathInspections(ctx, &revision.Snapshot.Tour)
		if err != nil {
			return "", err
		}
		path := routing.Route{Provider: after.Provider, DistanceM: after.DistanceM, DurationS: after.DurationS, Geometry: after.Geometry.Coordinates}
		for _, leg := range after.Legs {
			path.Legs = append(path.Legs, routing.Leg{DistanceM: leg.DistanceM, DurationS: leg.DurationS, Geometry: leg.Geometry})
		}
		var attribution *models.PathAttribution
		if inspector, ok := w.router.(routing.PathInspector); ok {
			attribution, err = inspector.InspectPath(routeCtx, seed.Mode, path)
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if err != nil {
				attribution = &models.PathAttribution{Provider: path.Provider, Failure: "attribution_unavailable"}
			}
		}
		inputJSON, err := json.Marshal(i.Input)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(inputJSON)
		quality := routing.AssessPathQuality(&revision.Snapshot.Tour, seed.Mode, path, attribution, inspections, hex.EncodeToString(digest[:]), w.now())
		quality.RouteSHA256 = after.RoutingFingerprint()
		result.Quality = &quality
		for _, issue := range quality.Issues {
			result.Issues = append(result.Issues, issue)
			if issue.Severity != "information" {
				result.Status = "needs_research"
			}
		}
	}
	if err := w.store.CompleteRouteInvestigation(ctx, i.ID, job, result); errors.Is(err, repository.ErrResearchInactive) {
		return "investigation_superseded", nil
	} else if err != nil {
		return "", err
	}
	return "private_alternative_requires_review", nil
}

// investigationAlternative replaces affected legs only. Story IDs, moments,
// narration, other mode routes, approved arrivals and corridor intent stay frozen.
func (w *Worker) investigationAlternative(ctx context.Context, before models.TourRoute, tour *models.TourDetail, report models.RouteReportInput) (*models.TourRoute, []string, error) {
	bad := errors.New("invalid_investigation_path")
	affected := []string{}
	if report.Affected.Kind == "segment" {
		affected = append(affected, report.Affected.FromStopID, report.Affected.ToStopID)
	} else {
		affected = append(affected, report.Affected.StopID)
	}
	positions := map[int]string{}
	for _, stop := range tour.Stops {
		positions[stop.Position] = stop.ID
	}
	if before.Status != models.RouteStatusReady || len(before.Legs) == 0 || len(before.Legs) > 30 || w.router == nil {
		return nil, affected, bad
	}
	segmentIndex := -1
	if report.Affected.Kind == "segment" {
		a := report.Affected
		if report.Direction != "reverse" {
			segmentIndex = slices.IndexFunc(before.Legs, func(leg models.RouteLeg) bool {
				return positions[leg.FromPosition] == a.FromStopID && positions[leg.ToPosition] == a.ToStopID
			})
		}
		if segmentIndex < 0 && report.Direction != "forward" {
			segmentIndex = slices.IndexFunc(before.Legs, func(leg models.RouteLeg) bool {
				return positions[leg.FromPosition] == a.ToStopID && positions[leg.ToPosition] == a.FromStopID
			})
		}
	}
	after := before
	after.ID, after.Journey, after.Quality, after.ComputedAt = "", nil, nil, nil
	after.Legs = slices.Clone(before.Legs)
	after.Geometry = &models.LineString{Type: "LineString", Coordinates: [][2]float64{}}
	after.DistanceM, after.DurationS = 0, 0
	changed, totalPoints := 0, 0
	for j, original := range before.Legs {
		from, to := positions[original.FromPosition], positions[original.ToPosition]
		if from == "" || to == "" || len(original.Geometry) < 2 {
			return nil, affected, bad
		}
		a := report.Affected
		replace := a.Kind == "segment" && j == segmentIndex || a.Kind != "segment" && (from == a.StopID || to == a.StopID)
		if replace {
			for _, id := range []string{from, to} {
				if !slices.Contains(affected, id) {
					affected = append(affected, id)
				}
			}
			points := [][2]float64{original.Geometry[0], original.Geometry[len(original.Geometry)-1]}
			if a.Kind == "entrance" && report.Annotation != nil {
				point := [2]float64{*report.Annotation.Lng, *report.Annotation.Lat}
				if from == a.StopID {
					points[0] = point
				}
				if to == a.StopID {
					points[1] = point
				}
			}
			var path routing.Route
			var err error
			if a.Kind == "segment" && report.Alternative != nil {
				provider, ok := w.router.(routing.ControlledRouter)
				if !ok {
					return nil, affected, bad
				}
				via := slices.Clone(report.Alternative.Coordinates)
				if from != a.FromStopID {
					slices.Reverse(via)
				}
				controls := []routing.Waypoint{{Point: routing.Point{Lng: points[0][0], Lat: points[0][1]}, Break: true}}
				for _, p := range via {
					if geo.DistanceMeters(p[1], p[0], points[0][1], points[0][0]) < 1 || geo.DistanceMeters(p[1], p[0], points[1][1], points[1][0]) < 1 {
						continue
					}
					controls = append(controls, routing.Waypoint{Point: routing.Point{Lng: p[0], Lat: p[1]}})
				}
				controls = append(controls, routing.Waypoint{Point: routing.Point{Lng: points[1][0], Lat: points[1][1]}, Break: true})
				path, err = provider.RouteControlled(ctx, report.Mode, routing.ControlledRequest{Controls: controls, FollowReference: true})
			} else {
				path, err = w.router.Route(ctx, report.Mode, []routing.Point{{Lng: points[0][0], Lat: points[0][1]}, {Lng: points[1][0], Lat: points[1][1]}})
			}
			if err != nil || len(path.Provider) == 0 || len(path.Provider) > 100 || len(path.Legs) != 1 {
				return nil, affected, bad
			}
			leg := path.Legs[0]
			if len(leg.Geometry) < 2 || len(leg.Geometry) > 16384 || math.IsNaN(leg.DistanceM) || math.IsInf(leg.DistanceM, 0) || leg.DistanceM < 0 || leg.DistanceM > 5e6 || leg.DurationS < 0 || leg.DurationS > 604800 {
				return nil, affected, bad
			}
			for _, p := range leg.Geometry {
				if math.IsNaN(p[0]) || math.IsNaN(p[1]) || math.IsInf(p[0], 0) || math.IsInf(p[1], 0) || math.Abs(p[0]) > 180 || math.Abs(p[1]) > 90 {
					return nil, affected, bad
				}
			}
			if geo.DistanceMeters(leg.Geometry[0][1], leg.Geometry[0][0], points[0][1], points[0][0]) > 100 || geo.DistanceMeters(leg.Geometry[len(leg.Geometry)-1][1], leg.Geometry[len(leg.Geometry)-1][0], points[1][1], points[1][0]) > 100 {
				return nil, affected, bad
			}
			after.Legs[j].Geometry, after.Legs[j].DistanceM, after.Legs[j].DurationS = leg.Geometry, leg.DistanceM, leg.DurationS
			after.Legs[j].NavigationSource = "unreviewed_report_proposal"
			after.Legs[j].FromStopID, after.Legs[j].ToStopID = from, to
			if after.Provider != path.Provider {
				after.Provider = "mixed"
			}
			changed++
		}
		leg := after.Legs[j]
		totalPoints += len(leg.Geometry)
		if totalPoints > 16384 {
			return nil, affected, bad
		}
		after.Geometry.Coordinates = append(after.Geometry.Coordinates, leg.Geometry...)
		after.DistanceM += leg.DistanceM
		after.DurationS += leg.DurationS
	}
	if changed == 0 {
		return nil, affected, bad
	}
	return &after, affected, nil
}
