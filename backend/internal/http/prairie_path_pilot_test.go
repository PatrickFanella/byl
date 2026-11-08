package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/jobs"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/routing"
)

const prairieDirectory = "../../../docs/product/pilots/prairie-path"

type prairieStop struct {
	Key, Name, Address, Municipality string
	Position                         struct {
		Latitude, Longitude float64
		SourceURL           string `json:"source_url"`
	}
	ProposedArrival struct {
		Latitude, Longitude float64
		OSMNodeID           int64 `json:"osm_node_id"`
	} `json:"proposed_arrival"`
	PlannedDwellMinutes int                  `json:"planned_dwell_minutes"`
	SourceIDs           []string             `json:"source_ids"`
	MomentTitle         string               `json:"moment_title"`
	DateStart           string               `json:"date_start"`
	DatePrecision       models.DatePrecision `json:"date_precision"`
	Body                string
	Gaps                []string
}
type prairieSource struct {
	ID, URL, Title, Publisher string
	RetrievedAt               *models.Date `json:"retrieved_at"`
}

func prairieRead(t *testing.T, name string, target any) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(prairieDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	return raw
}

type prairieResearch []models.DiscoveryItem

func (f prairieResearch) Process(_ context.Context, task jobs.ResearchTask) (models.ResearchOutput, error) {
	if task.Stage != "research" {
		return models.ResearchOutput{}, jobs.ResearchFailure{Code: "invalid_output"}
	}
	raw, err := json.Marshal(models.DiscoveryReport{Items: f, Sources: []models.DiscoverySource{}, Providers: []models.DiscoveryProvider{{Name: "inspected-pilot-manifest-replay", Outcome: "recorded"}}, Gaps: []string{"Public arrivals, rights-cleared illustrations and route access remain unreviewed."}})
	return models.ResearchOutput{Data: raw}, err
}

// This is a disposable replay of the inspected source manifest. Provider discovery is
// recorded input; synthetic correction below is not a real field observation.
func TestPrairiePathPilotReplayReconcilesDraftsAndKeepsPublicationBlocked(t *testing.T) {
	ctx := context.Background()
	var brief models.RouteBriefInput
	prairieRead(t, "brief.json", &brief)
	var stops []prairieStop
	prairieRead(t, "stops.json", &stops)
	var sources []prairieSource
	prairieRead(t, "sources.json", &sources)
	sourceByID := map[string]prairieSource{}
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	if len(stops) != 3 || brief.DurationMinutes != nil {
		t.Fatal("fixed pilot or unsupported time promise changed")
	}
	e := newTestEnv(t)
	e.drain(t)
	cities := map[string]string{}
	for _, input := range []models.CreateCityRequest{
		{Slug: "villa-park", Name: "Villa Park", Region: "Illinois", CountryCode: "US", CenterLat: 41.884, CenterLng: -87.976, BBoxMinLat: 41.87, BBoxMaxLat: 41.90, BBoxMinLng: -87.99, BBoxMaxLng: -87.95, Timezone: "America/Chicago"},
		{Slug: "wheaton", Name: "Wheaton", Region: "Illinois", CountryCode: "US", CenterLat: 41.864, CenterLng: -88.11, BBoxMinLat: 41.84, BBoxMaxLat: 41.90, BBoxMinLng: -88.15, BBoxMaxLng: -88.08, Timezone: "America/Chicago"},
	} {
		city, err := e.store.CreateCity(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		cities[input.Name] = city.ID
	}
	curator := e.signIn(t, "prairie-curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "prairie-visitor@example.com", "")
	created := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs", curator, brief)
	if created.status != 201 {
		t.Fatalf("brief %d %s", created.status, created.raw)
	}
	briefID := created.body["id"].(string)
	base := "/api/v1/admin/route-briefs/" + briefID
	var placesBefore int
	if err := e.store.Pool().QueryRow(ctx, `SELECT count(*) FROM places`).Scan(&placesBefore); err != nil {
		t.Fatal(err)
	}
	records := []models.ReconciliationRecord{}
	for _, stop := range stops {
		lat, lng := stop.Position.Latitude, stop.Position.Longitude
		item := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: stop.Name, Text: stop.Body, URL: sourceByID[stop.SourceIDs[0]].URL, Lat: &lat, Lng: &lng, Area: stop.Municipality, Evidence: "recorded_source_review", Origins: []models.DiscoveryOrigin{}, Identity: &models.IdentityFacts{CityID: cities[stop.Municipality], PlaceName: stop.Name, Address: stop.Address}}
		in := models.ReconciliationRequest{Version: 1, Item: item}
		first := e.do(t, http.MethodPost, base+"/reconciliation", curator, in)
		if first.status != 200 {
			t.Fatalf("reconcile %s %d %s", stop.Name, first.status, first.raw)
		}
		var record models.ReconciliationRecord
		if err := json.Unmarshal(first.raw, &record); err != nil {
			t.Fatal(err)
		}
		if record.PlaceCandidateID == nil {
			t.Fatalf("new place did not remain a candidate: %+v", record)
		}
		repeated := e.do(t, http.MethodPost, base+"/reconciliation", curator, in)
		var retry models.ReconciliationRecord
		if err := json.Unmarshal(repeated.raw, &retry); err != nil {
			t.Fatal(err)
		}
		if repeated.status != 200 || retry.ID != record.ID || retry.PlaceCandidateID == nil || *retry.PlaceCandidateID != *record.PlaceCandidateID {
			t.Fatal("replay duplicated pending identity")
		}
		records = append(records, record)
	}
	var beforeApproval int
	if err := e.store.Pool().QueryRow(ctx, `SELECT count(*) FROM places`).Scan(&beforeApproval); err != nil || beforeApproval != placesBefore {
		t.Fatal("reconciliation materialized places before review", err)
	}
	material := prairieResearch{}
	placeIDs := []string{}
	momentIDs := []string{}
	for n, stop := range stops {
		id := *records[n].PlaceCandidateID
		approved := e.do(t, http.MethodPost, "/api/v1/admin/candidates/"+id+"/approve", curator, map[string]any{"note": "Disposable pilot review: draft identity only. Position is a location lead; arrivals remain unreviewed."})
		if approved.status != 200 {
			t.Fatalf("candidate approval %d %s", approved.status, approved.raw)
		}
		candidate, err := e.store.GetCandidate(ctx, id)
		if err != nil || candidate.ApprovedEntityID == nil {
			t.Fatal("missing draft entity", err)
		}
		place := *candidate.ApprovedEntityID
		placeIDs = append(placeIDs, place)
		citations := []models.Source{}
		for _, sourceID := range stop.SourceIDs {
			source := sourceByID[sourceID]
			citations = append(citations, models.Source{Title: source.Title, Publisher: source.Publisher, URL: source.URL, RetrievedAt: source.RetrievedAt})
		}
		moment, err := e.store.CreateMoment(ctx, models.CreateMomentRequest{PlaceID: place, Title: stop.MomentTitle, Body: stop.Body, DateStart: stop.DateStart, DatePrecision: stop.DatePrecision, Sources: citations})
		if err != nil {
			t.Fatal(err)
		}
		momentIDs = append(momentIDs, moment.ID)
		decision := models.IdentityDecisionRequest{Version: records[n].ReviewVersion, Outcome: "existing_place_existing_moment", PlaceID: place, MomentID: moment.ID, Note: "Disposable curator links the supported short draft. No history, arrival or media publication is approved."}
		reviewed := e.do(t, http.MethodPut, base+"/reconciliation/"+records[n].ID+"/decision", curator, decision)
		if reviewed.status != 200 {
			t.Fatalf("identity review %d %s", reviewed.status, reviewed.raw)
		}
		material = append(material, models.DiscoveryItem{Kind: models.CandidateKindMoment, EntityID: moment.ID, Title: moment.Title, Text: stop.Body, URL: citations[0].URL, Area: stop.Municipality, Evidence: "recorded_draft_collection", Origins: []models.DiscoveryOrigin{}})
	}
	e.worker.Research = material
	e.worker.Reconcile = true
	e.worker.Assemble = true
	started := e.do(t, http.MethodPost, base+"/research", curator, map[string]any{"version": 1})
	if started.status != 202 {
		t.Fatalf("start %d %s", started.status, started.raw)
	}
	runID := started.body["id"].(string)
	e.drain(t)
	itinerary, err := e.store.GetBriefItinerary(ctx, briefID, runID)
	if err != nil || itinerary.Proposal == nil || len(itinerary.Proposal.Stops) != 3 || itinerary.Proposal.TravelMinutes != nil {
		raw, _ := json.MarshalIndent(itinerary, "", "  ")
		t.Fatalf("local assembly: %s %v", raw, err)
	}
	dwell := 0
	for _, stop := range stops {
		dwell += stop.PlannedDwellMinutes
	}
	var timing struct {
		PlannedDwellMinutes int `json:"planned_dwell_minutes"`
	}
	prairieRead(t, "routing.json", &timing)
	if itinerary.Proposal.PlannedDwellMinutes != dwell || timing.PlannedDwellMinutes != dwell {
		t.Fatal("proposal and source dwell differ")
	}

	before, _ := json.Marshal(itinerary)
	// Re-deliver the exact completed job, as a restarted worker can. No source/LLM
	// stage or place creation may repeat.
	if _, err := e.store.Pool().Exec(ctx, `UPDATE jobs SET status='queued',run_after=now() WHERE id=$1`, started.body["job_id"]); err != nil {
		t.Fatal(err)
	}
	e.worker.Research = nil
	e.drain(t)
	after, err := e.store.GetBriefItinerary(ctx, briefID, runID)
	afterRaw, _ := json.Marshal(after)
	if err != nil || string(before) != string(afterRaw) {
		t.Fatal("completed pilot replay changed its checkpoint", err)
	}
	var placesAfter int
	if err := e.store.Pool().QueryRow(ctx, `SELECT count(*) FROM places`).Scan(&placesAfter); err != nil || placesAfter != placesBefore+3 {
		t.Fatal("pilot duplicated places", err)
	}

	var capture struct {
		FixtureSHA256 string `json:"fixture_sha256"`
		Request       struct {
			Costing, Units string
			Locations      []struct {
				Lat, Lon     float64
				Type         string
				Radius       int
				SearchCutoff int `json:"search_cutoff"`
			}
		}
	}
	prairieRead(t, "routing.json", &capture)
	if len(capture.Request.Locations) != 10 {
		t.Fatal("bounded comparison controls changed")
	}
	breaks := 0
	controls := []routing.Waypoint{}
	for _, point := range capture.Request.Locations {
		control := routing.Waypoint{Point: routing.Point{Lat: point.Lat, Lng: point.Lon}, Break: point.Type == "break"}
		controls = append(controls, control)
		if control.Break {
			if breaks >= len(stops) || control.Lat != stops[breaks].ProposedArrival.Latitude || control.Lng != stops[breaks].ProposedArrival.Longitude {
				t.Fatal("story break detached from proposed trail arrival")
			}
			breaks++
		}
	}
	if breaks != 3 {
		t.Fatal("shaping points became stories")
	}
	routeRaw := prairieRead(t, "valhalla-response.json", nil)
	if fmt.Sprintf("%x", sha256.Sum256(routeRaw)) != capture.FixtureSHA256 {
		t.Fatal("captured geometry checksum mismatch")
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/route" {
			var input struct {
				Costing, Units string
				Locations      []struct {
					Lat, Lon     float64
					Type         string
					Radius       int
					SearchCutoff int `json:"search_cutoff"`
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				http.Error(w, "invalid input", 400)
				return
			}
			expected, _ := json.Marshal(capture.Request)
			actual, _ := json.Marshal(input)
			if string(expected) != string(actual) {
				t.Error("routing inputs differ from actual capture")
				http.Error(w, "capture mismatch", 400)
				return
			}
			if _, err := w.Write(routeRaw); err != nil {
				t.Error(err)
			}
			return
		}
		if r.URL.Path == "/trace_attributes" {
			var input struct {
				Encoded    string `json:"encoded_polyline"`
				ShapeMatch string `json:"shape_match"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				http.Error(w, "bad input", 400)
				return
			}
			if input.ShapeMatch != "edge_walk" {
				t.Error("approximate matching fallback attempted")
			}
			var response struct {
				Trip struct{ Legs []struct{ Shape string } }
			}
			if err := json.Unmarshal(routeRaw, &response); err != nil {
				t.Error(err)
				return
			}
			index := 0
			for i, leg := range response.Trip.Legs {
				if leg.Shape == input.Encoded {
					index = i + 1
				}
			}
			if index == 0 {
				t.Error("attribution shape changed")
				http.Error(w, "shape mismatch", 400)
				return
			}
			if index == 2 {
				w.WriteHeader(400)
			}
			if _, err := w.Write(prairieRead(t, fmt.Sprintf("strict-attribution-leg-%d.json", index), nil)); err != nil {
				t.Error(err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer provider.Close()
	router := routing.NewValhalla(provider.URL, provider.Client())
	controlled, ok := router.(routing.ControlledRouter)
	if !ok {
		t.Fatal("Valhalla lacks controlled routing")
	}
	inspector, ok := router.(routing.PathInspector)
	if !ok {
		t.Fatal("Valhalla lacks strict inspection")
	}
	computed, err := controlled.RouteControlled(ctx, models.TravelModeBike, routing.ControlledRequest{Controls: controls, FollowReference: false})
	if err != nil || computed.DistanceM != 12372 || computed.DurationS != 3111 || len(computed.Legs) != 2 {
		t.Fatalf("computed comparison %+v %v", computed, err)
	}
	var reference struct {
		Nodes      [][]float64
		Traversals []struct {
			WayID    int64   `json:"way_id"`
			FromNode int64   `json:"from_node"`
			ToNode   int64   `json:"to_node"`
			NodeIDs  []int64 `json:"node_ids"`
			Tags     map[string]string
		}
	}
	prairieRead(t, "network-reference.json", &reference)
	if len(reference.Nodes) != 401 || len(reference.Traversals) != 69 {
		t.Fatal("bounded Main Stem reference changed")
	}
	orderedIDs := []int64{}
	coordinates := [][2]float64{}
	for _, node := range reference.Nodes {
		if len(node) != 3 {
			t.Fatal("reference node dimensions")
		}
		orderedIDs = append(orderedIDs, int64(node[0]))
		coordinates = append(coordinates, [2]float64{node[1], node[2]})
	}
	chain := []int64{}
	for _, way := range reference.Traversals {
		if way.WayID <= 0 || len(way.NodeIDs) < 2 || way.FromNode != way.NodeIDs[0] || way.ToNode != way.NodeIDs[len(way.NodeIDs)-1] || way.Tags["name"] != "Illinois Prairie Path" {
			t.Fatal("invalid Main Stem way identity")
		}
		if len(chain) == 0 {
			chain = append(chain, way.NodeIDs...)
		} else {
			if chain[len(chain)-1] != way.FromNode {
				t.Fatal("disconnected reference way")
			}
			chain = append(chain, way.NodeIDs[1:]...)
		}
	}
	if !slices.Equal(chain, orderedIDs) {
		t.Fatal("network way traversal differs from retained node chain")
	}
	for _, stop := range stops {
		if !slices.Contains(orderedIDs, stop.ProposedArrival.OSMNodeID) {
			t.Fatal("proposed arrival is outside selected Main Stem chain")
		}
	}
	for _, point := range computed.Geometry {
		if !geo.NearPolyline(point, coordinates, 25) {
			t.Fatal("captured comparison departs from Main Stem reference")
		}
	}

	attribution, err := inspector.InspectPath(ctx, models.TravelModeBike, computed)
	if err != nil || attribution == nil || attribution.Failure == "" {
		t.Fatalf("missing provider topology was treated as proof %+v %v", attribution, err)
	}
	second, err := inspector.InspectPath(ctx, models.TravelModeBike, routing.Route{Provider: computed.Provider, Legs: computed.Legs[1:]})
	if err != nil || second.Failure != "exact_match_unavailable" {
		t.Fatalf("strict rejection hidden %+v %v", second, err)
	}
	ordered := []models.StopInput{}
	for n, id := range placeIDs {
		moment := momentIDs[n]
		ordered = append(ordered, models.StopInput{PlaceID: id, FeaturedMomentID: &moment})
	}
	tourResponse := e.do(t, http.MethodPost, "/api/v1/admin/tours", curator, models.CreateTourRequest{CityID: cities["Villa Park"], Title: "Prairie Path Main Stem — private pilot", Modes: []models.TravelMode{models.TravelModeBike}, Stops: ordered})
	if tourResponse.status != 201 {
		t.Fatalf("tour %d %s", tourResponse.status, tourResponse.raw)
	}
	tourID := tourResponse.body["id"].(string)
	e.drain(t)
	tour, err := e.store.GetTour(ctx, tourID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tour.CityIDs) != 2 {
		t.Fatal("cross-municipality story identities lost", tour.CityIDs)
	}
	stored := models.TourRoute{TourID: tourID, Mode: models.TravelModeBike, Status: models.RouteStatusReady, Provider: computed.Provider, DistanceM: computed.DistanceM, DurationS: computed.DurationS, Geometry: &models.LineString{Type: "LineString", Coordinates: computed.Geometry}}
	for i, leg := range computed.Legs {
		stored.Legs = append(stored.Legs, models.RouteLeg{FromStopID: tour.Stops[i].ID, ToStopID: tour.Stops[i+1].ID, FromPosition: i + 1, ToPosition: i + 2, NavigationSource: "unapproved_proposed_arrival_comparison", DistanceM: leg.DistanceM, DurationS: leg.DurationS, Geometry: leg.Geometry})
	}
	// This fixture imports the actual comparison for private review. It does not
	// pretend the 10-control demo request qualifies the complete corridor plan.
	if err := e.store.UpsertRoute(ctx, stored); err != nil {
		t.Fatal(err)
	}

	review := e.get(t, base+"/review-package?tour_id="+tourID, curator)
	pack := decodePackage(t, review)
	if pack.ReadyToPublish || len(pack.VisitorStories) != 3 || len(pack.Tasks) == 0 || pack.Routes["bike"].Geometry == nil {
		t.Fatalf("unqualified draft package %+v", pack)
	}
	for _, token := range []string{"", visitor} {
		if r := e.get(t, base+"/review-package?tour_id="+tourID, token); r.status != 401 && r.status != 403 {
			t.Fatal("pilot package became public")
		}
	}
	if r := e.get(t, "/api/v1/tours/"+tourID, ""); r.status != 404 {
		t.Fatal("private draft tour became public", r.status)
	}
	if !strings.Contains(strings.ToLower(string(review.raw)), "arrival") {
		t.Fatal("missing arrival qualification disappeared")
	}
	// Synthetic correction exercises the real proposal/review controls. It is
	// not a claim that this coordinate is an actual trail arrival.
	lat, lng := stops[0].Position.Latitude+0.0001, stops[0].Position.Longitude
	arrivalBase := "/api/v1/admin/places/" + placeIDs[0] + "/arrivals"
	proposal := e.do(t, http.MethodPost, arrivalBase, curator, models.ArrivalProposalRequest{Point: models.ArrivalPoint{Name: "Synthetic correction only; not a real entrance", Kind: "viewpoint", Latitude: &lat, Longitude: &lng, Access: []models.ArrivalAccess{{Mode: models.TravelModeWalk, Status: "unknown", Surface: "unknown", SourceURL: "https://example.org/synthetic-prairie-correction", SourceTitle: "Synthetic software exercise; no field visit", RetrievedAt: "2026-11-08"}, {Mode: models.TravelModeBike, Status: "unknown", Surface: "unknown", SourceURL: "https://example.org/synthetic-prairie-correction", SourceTitle: "Synthetic software exercise; no field visit", RetrievedAt: "2026-11-08"}}}})
	if proposal.status != 201 {
		t.Fatalf("synthetic proposal %d %s", proposal.status, proposal.raw)
	}
	corrected := e.do(t, http.MethodPost, arrivalBase+"/"+proposal.body["id"].(string)+"/review", curator, models.ArrivalReviewRequest{Action: "approve", Note: "Disposable synthetic coordinate correction, with access explicitly unknown. Never copy this approval to production."})
	if corrected.status != 200 {
		t.Fatalf("synthetic correction %d %s", corrected.status, corrected.raw)
	}
	place, err := e.store.GetPlace(ctx, placeIDs[0], "", true)
	if err != nil || place.Latitude != stops[0].Position.Latitude || place.ArrivalPoint == nil || place.ArrivalPoint.Point.Latitude == nil || *place.ArrivalPoint.Point.Latitude != lat {
		t.Fatal("correction moved historical site or lost separate arrival", err)
	}
	if folder := os.Getenv("BYL_PRAIRIE_EXPORT"); folder != "" {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "review-package.json"), review.raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
