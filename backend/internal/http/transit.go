package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/transit"
)

const transitCapability = "scheduled-transit-v1"

func transitHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization, X-Byl-Capabilities")
}

func (d Deps) transitCapable(r *http.Request) bool {
	if !d.TransitEnabled || d.TransitPlanner == nil {
		return false
	}
	for _, value := range strings.Split(r.Header.Get("X-Byl-Capabilities"), ",") {
		if strings.TrimSpace(value) == transitCapability {
			return true
		}
	}
	return false
}

func (d Deps) requireTransitClient(w http.ResponseWriter, r *http.Request) bool {
	transitHeaders(w)
	if !d.transitCapable(r) {
		writeError(w, http.StatusConflict, "unsupported_itinerary", "this revision requires enabled scheduled-transit support in the client")
		return false
	}
	return true
}

func (d Deps) itineraryCapabilities(w http.ResponseWriter, r *http.Request) {
	transitHeaders(w)
	writeJSON(w, http.StatusOK, map[string]any{"scheduled_transit_v1": d.TransitEnabled && d.TransitPlanner != nil, "provider": "cta_gtfs_blue_v1", "live": false, "transfers": false, "bicycles": false})
}

func decodeTransit(w http.ResponseWriter, r *http.Request, body any) bool {
	transitHeaders(w)
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "bad_request", "transit requests do not accept query parameters")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "provide one bounded transit object with only the documented fields")
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "provide one bounded transit object with only the documented fields")
		return false
	}
	return true
}

func (d Deps) adminSetTransit(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	var in models.TransitSelectionRequest
	if !decodeTransit(w, r, &in) {
		return
	}
	if in.DraftVersion < 0 || in.Remove == (in.Intent != nil) || strings.TrimSpace(in.ReviewNote) == "" || len(in.ReviewNote) > 4000 {
		writeValidation(w, models.Fields{"intent": "provide an explicit selection or removal, current draft version and review note"})
		return
	}
	var verified *models.ApprovedTransit
	now := time.Now().UTC()
	if in.Intent != nil {
		if fields := in.Intent.Validate(); len(fields) > 0 {
			writeValidation(w, fields)
			return
		}
		if !in.Intent.MatchesStops(tour) {
			writeValidation(w, models.Fields{"from_stop_id": "select adjacent story stops in this walking tour"})
			return
		}
		if in.DraftVersion != tour.DraftVersion {
			writeError(w, http.StatusConflict, "conflict", "tour draft changed; reload before selecting transit")
			return
		}
		if d.TransitPlanner == nil {
			writeError(w, http.StatusServiceUnavailable, "transit_provider_unavailable", "scheduled transit provider is not configured")
			return
		}
		connection := d.TransitPlanner.Connect(r.Context(), transit.Request{BoardStopID: in.Intent.BoardStopID, AlightStopID: in.Intent.AlightStopID, Mode: "walk", DepartureAt: now.Add(time.Duration(in.Intent.WalkToStopSeconds+in.Intent.BoardingBufferSeconds) * time.Second)}, now)
		if connection.Source == nil || connection.Board == nil || connection.Alight == nil || (connection.State != "scheduled" && connection.State != "no_service") {
			writeError(w, http.StatusConflict, "transit_source_unavailable", "selected platforms need a fresh supported timetable before curator approval")
			return
		}
		verified = &models.ApprovedTransit{Board: models.TransitPlatform(*connection.Board), Alight: models.TransitPlatform(*connection.Alight), Source: models.TransitSource(*connection.Source)}
	}
	choice, err := d.Store.SetTourTransit(r.Context(), tour.ID, accountFrom(r.Context()).ID, in, verified, now)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transit": choice, "draft_version": in.DraftVersion + 1})
}

type transitPlanRequest struct {
	Version      int       `json:"version"`
	DepartureAt  time.Time `json:"departure_at"`
	DraftVersion *int64    `json:"draft_version,omitempty"`
}

type transitPlanSegment struct {
	Kind           string             `json:"kind"`
	FromStopID     string             `json:"from_stop_id,omitempty"`
	ToStopID       string             `json:"to_stop_id,omitempty"`
	FromPlatformID string             `json:"from_platform_id,omitempty"`
	ToPlatformID   string             `json:"to_platform_id,omitempty"`
	Geometry       *models.LineString `json:"geometry,omitempty"`
	DurationS      int                `json:"duration_s"`
	DepartureAt    *time.Time         `json:"departure_at,omitempty"`
	ArrivalAt      *time.Time         `json:"arrival_at,omitempty"`
	Basis          string             `json:"basis"`
	Navigation     string             `json:"navigation"`
}

type scheduledTransitPlan struct {
	Version                int                  `json:"version"`
	TourID                 string               `json:"tour_id"`
	RevisionID             *string              `json:"revision_id"`
	DraftVersion           *int64               `json:"draft_version,omitempty"`
	ConnectorVersion       int64                `json:"connector_version"`
	State                  string               `json:"state"`
	RequestedDepartureAt   time.Time            `json:"requested_departure_at"`
	DepartureContextStopID string               `json:"departure_context_stop_id"`
	Service                transit.Connection   `json:"service"`
	Segments               []transitPlanSegment `json:"segments"`
	Assumptions            []string             `json:"assumptions"`
}

func (p scheduledTransitPlan) blocked(state string) scheduledTransitPlan {
	p.State = state
	p.Segments = []transitPlanSegment{}
	p.Service = transit.Connection{State: state, Scheduled: true, Source: p.Service.Source, Issues: []string{"selection_requires_review"}}
	return p
}

func decodeTransitPlan(w http.ResponseWriter, r *http.Request, preview bool) (transitPlanRequest, bool) {
	var in transitPlanRequest
	if !decodeTransit(w, r, &in) {
		return in, false
	}
	now := time.Now()
	if in.Version != 1 || in.DepartureAt.IsZero() || in.DepartureAt.Before(now) || in.DepartureAt.After(now.Add(7*24*time.Hour)) || (!preview && in.DraftVersion != nil) || (preview && in.DraftVersion == nil) {
		writeValidation(w, models.Fields{"departure_at": "use version 1 and a future RFC3339 departure within seven days; previews also require the draft version"})
		return in, false
	}
	return in, true
}

func (d Deps) revisionTransitPlan(w http.ResponseWriter, r *http.Request) {
	if !d.requireTransitClient(w, r) {
		return
	}
	tour, ok := d.visibleTourBase(w, r)
	if !ok {
		return
	}
	revision, err := d.Store.GetTourRevision(r.Context(), tour.ID, chi.URLParam(r, "revision"))
	if err != nil {
		d.fail(w, err)
		return
	}
	if !canSeeTour(&revision.Snapshot.Tour.TourSummary, accountFrom(r.Context())) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if revision.Snapshot.Tour.Transit == nil {
		writeError(w, http.StatusNotFound, "not_found", "this revision has no transit selection")
		return
	}
	in, ok := decodeTransitPlan(w, r, false)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, d.planTransit(r, &revision.Snapshot.Tour, in.DepartureAt))
}

func (d Deps) adminTransitPlan(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	in, ok := decodeTransitPlan(w, r, true)
	if !ok {
		return
	}
	if *in.DraftVersion != tour.DraftVersion {
		writeError(w, http.StatusConflict, "conflict", "tour draft changed; reload before previewing")
		return
	}
	if tour.Transit == nil {
		writeError(w, http.StatusNotFound, "not_found", "this draft has no transit selection")
		return
	}
	if d.TransitPlanner == nil {
		writeError(w, http.StatusServiceUnavailable, "transit_provider_unavailable", "scheduled transit provider is not configured")
		return
	}
	// Private preview may use the current qualified walking result. Publication
	// independently rechecks readiness and freezes only the non-transit legs.
	if route, err := d.Store.GetTourRoute(r.Context(), tour.ID, models.TravelModeWalk); err == nil && route.Status == models.RouteStatusReady {
		for _, leg := range route.Legs {
			from, to := "", ""
			for _, stop := range tour.Stops {
				if stop.Position == leg.FromPosition {
					from = stop.ID
				}
				if stop.Position == leg.ToPosition {
					to = stop.ID
				}
			}
			if from != tour.Transit.Intent.FromStopID || to != tour.Transit.Intent.ToStopID {
				tour.Transit.WalkingLegs = append(tour.Transit.WalkingLegs, leg)
			}
		}
	}
	writeJSON(w, http.StatusOK, d.planTransit(r, tour, in.DepartureAt))
}

func platformMatches(a models.TransitPlatform, b *transit.Stop) bool {
	return b != nil && a.ID == b.ID && a.ParentID == b.ParentID && a.Name == b.Name && geo.DistanceMeters(a.Latitude, a.Longitude, b.Latitude, b.Longitude) <= 25
}

func (d Deps) planTransit(r *http.Request, tour *models.TourDetail, departure time.Time) scheduledTransitPlan {
	departure = departure.UTC()
	choice := tour.Transit
	intent := choice.Intent
	plan := scheduledTransitPlan{Version: 1, TourID: tour.ID, RevisionID: tour.RevisionID, ConnectorVersion: choice.Version, State: "unavailable", RequestedDepartureAt: departure.UTC(), DepartureContextStopID: intent.FromStopID, Segments: []transitPlanSegment{}, Assumptions: []string{intent.WalkingAssumptions, "Plan for the time you expect to leave the selected story stop after finishing its stories.", "CTA times follow the published timetable. Check current arrivals and service alerts before boarding.", "Station locations come from the timetable. Use station signs or external directions to find public entrances; walking times are curator estimates."}}
	if tour.WorkingCopy {
		plan.RevisionID = nil
		draftVersion := tour.DraftVersion
		plan.DraftVersion = &draftVersion
	}
	if !intent.MatchesStops(tour) {
		return plan.blocked("needs_review")
	}
	boardingAfter := departure.Add(time.Duration(intent.WalkToStopSeconds+intent.BoardingBufferSeconds) * time.Second)
	c := d.TransitPlanner.Connect(r.Context(), transit.Request{BoardStopID: intent.BoardStopID, AlightStopID: intent.AlightStopID, DepartureAt: boardingAfter, Mode: string(intent.Mode)}, time.Now().UTC())
	plan.Service = c
	plan.State = c.State
	if c.Board != nil && c.Alight != nil && (!platformMatches(choice.Board, c.Board) || !platformMatches(choice.Alight, c.Alight)) {
		return plan.blocked("needs_review")
	}
	if c.State != "scheduled" || c.DepartureAt == nil || c.ArrivalAt == nil {
		return plan
	}
	if c.Source == nil || c.Source.URL != transit.FeedURL || c.Live || !c.Scheduled || c.Board == nil || c.Alight == nil || c.DepartureAt.Before(boardingAfter) || !c.ArrivalAt.After(*c.DepartureAt) {
		return plan.blocked("unavailable")
	}
	fromPosition := 0
	for _, stop := range tour.Stops {
		if stop.ID == intent.FromStopID {
			fromPosition = stop.Position
		}
	}
	walkToEnd := departure.Add(time.Duration(intent.WalkToStopSeconds) * time.Second)
	walkFromEnd := c.ArrivalAt.Add(time.Duration(intent.WalkFromStopSeconds) * time.Second)
	connector := []transitPlanSegment{
		{Kind: "walk_to_stop", FromStopID: intent.FromStopID, ToPlatformID: intent.BoardStopID, DurationS: intent.WalkToStopSeconds, DepartureAt: &departure, ArrivalAt: &walkToEnd, Basis: "curator_walking_estimate", Navigation: "external_directions_required"},
		{Kind: "boarding_wait", FromPlatformID: intent.BoardStopID, ToPlatformID: intent.BoardStopID, DurationS: int(c.DepartureAt.Sub(walkToEnd).Seconds()), DepartureAt: &walkToEnd, ArrivalAt: c.DepartureAt, Basis: "schedule_and_curator_buffer", Navigation: "check_station_signs_and_alerts"},
		{Kind: "transit", FromPlatformID: intent.BoardStopID, ToPlatformID: intent.AlightStopID, DurationS: int(c.ArrivalAt.Sub(*c.DepartureAt).Seconds()), DepartureAt: c.DepartureAt, ArrivalAt: c.ArrivalAt, Basis: "cta_static_schedule", Navigation: "scheduled_rail"},
		{Kind: "walk_from_stop", FromPlatformID: intent.AlightStopID, ToStopID: intent.ToStopID, DurationS: intent.WalkFromStopSeconds, DepartureAt: c.ArrivalAt, ArrivalAt: &walkFromEnd, Basis: "curator_walking_estimate", Navigation: "external_directions_required"},
	}
	inserted := false
	seen := map[string]bool{}
	for _, leg := range choice.WalkingLegs {
		if !inserted && leg.FromPosition >= fromPosition {
			plan.Segments = append(plan.Segments, connector...)
			inserted = true
		}
		from, to := "", ""
		for _, stop := range tour.Stops {
			if stop.Position == leg.FromPosition {
				from = stop.ID
			}
			if stop.Position == leg.ToPosition {
				to = stop.ID
			}
		}
		if from == "" || to == "" || leg.DurationS < 0 || len(leg.Geometry) < 2 || (from == intent.FromStopID && to == intent.ToStopID) {
			return plan.blocked("needs_review")
		}
		key := from + ":" + to
		if seen[key] || (leg.FromStopID != "" && leg.FromStopID != from) || (leg.ToStopID != "" && leg.ToStopID != to) {
			return plan.blocked("needs_review")
		}
		seen[key] = true
		plan.Segments = append(plan.Segments, transitPlanSegment{Kind: "walk", FromStopID: from, ToStopID: to, Geometry: &models.LineString{Type: "LineString", Coordinates: leg.Geometry}, DurationS: leg.DurationS, Basis: "retained_qualified_walking_route", Navigation: "in_app_walking"})
	}
	if !inserted {
		plan.Segments = append(plan.Segments, connector...)
	}
	for n := 0; n+1 < len(tour.Stops); n++ {
		from, to := tour.Stops[n].ID, tour.Stops[n+1].ID
		if from == intent.FromStopID && to == intent.ToStopID {
			continue
		}
		if !seen[from+":"+to] {
			return plan.blocked("needs_review")
		}
	}
	return plan
}
