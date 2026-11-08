package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"invisible-museum/backend/internal/jobs"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/routing"
)

const haymarketDirectory = "../../../docs/product/pilots/haymarket"

type haymarketStop struct {
	Key, Name, Address string
	Position           struct {
		Latitude, Longitude float64
		SourceURL           string `json:"source_url"`
	}
	PlannedDwellMinutes int                  `json:"planned_dwell_minutes"`
	SourceIDs           []string             `json:"source_ids"`
	MomentTitle         string               `json:"moment_title"`
	DateStart           string               `json:"date_start"`
	DatePrecision       models.DatePrecision `json:"date_precision"`
	Body                string
	Gaps                []string
}
type haymarketSource struct {
	ID, URL, Title, Publisher string
	RetrievedAt               *models.Date `json:"retrieved_at"`
}

func haymarketRead(t *testing.T, name string, target any) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(haymarketDirectory, name))
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

type haymarketResearch []models.DiscoveryItem

func (f haymarketResearch) Process(_ context.Context, task jobs.ResearchTask) (models.ResearchOutput, error) {
	if task.Stage != "research" {
		return models.ResearchOutput{}, jobs.ResearchFailure{Code: "invalid_output"}
	}
	raw, err := json.Marshal(models.DiscoveryReport{Items: f, Sources: []models.DiscoverySource{}, Providers: []models.DiscoveryProvider{{Name: "inspected-pilot-manifest-replay", Outcome: "recorded"}}, Gaps: []string{"Public arrivals, rights-cleared illustrations and route access remain unreviewed."}})
	return models.ResearchOutput{Data: raw}, err
}

// This is a disposable replay of the inspected source manifest. Provider discovery is
// recorded input. Location leads and computed geometry are not field evidence.
func TestHaymarketPilotReplayReconcilesDraftsAndKeepsPublicationBlocked(t *testing.T) {
	ctx := context.Background()
	var brief models.RouteBriefInput
	haymarketRead(t, "brief.json", &brief)
	var stops []haymarketStop
	haymarketRead(t, "stops.json", &stops)
	var sources []haymarketSource
	haymarketRead(t, "sources.json", &sources)
	sourceByID := map[string]haymarketSource{}
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	if len(stops) != 2 || brief.DurationMinutes != nil {
		t.Fatal("fixed pilot or unsupported time promise changed")
	}
	var extension struct {
		Included            bool `json:"included_in_walking_geometry"`
		ScheduledConnection any  `json:"scheduled_connection"`
	}
	haymarketRead(t, "forest-park-extension.json", &extension)
	if extension.Included || extension.ScheduledConnection != nil {
		t.Fatal("unqualified cemetery extension entered walking draft")
	}
	for _, stop := range stops {
		if strings.Contains(stop.Name, "Greif") || strings.Contains(stop.Name, "Forest Park") {
			t.Fatal("unresolved lead became a pinned story")
		}
	}

	e := newTestEnv(t)
	e.drain(t)
	city, err := e.store.CreateCity(ctx, models.CreateCityRequest{Slug: "chicago", Name: "Chicago", Region: "Illinois", CountryCode: "US", CenterLat: 41.88, CenterLng: -87.64, BBoxMinLat: 41.6, BBoxMaxLat: 42.1, BBoxMinLng: -87.9, BBoxMaxLng: -87.5, Timezone: "America/Chicago"})
	if err != nil {
		t.Fatal(err)
	}
	curator := e.signIn(t, "haymarket-curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "haymarket-visitor@example.com", "")
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
		item := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: stop.Name, Text: stop.Body, URL: sourceByID[stop.SourceIDs[0]].URL, Lat: &lat, Lng: &lng, Area: "Chicago", Evidence: "recorded_source_review", Origins: []models.DiscoveryOrigin{}, Identity: &models.IdentityFacts{CityID: city.ID, PlaceName: stop.Name, Address: stop.Address}}
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
	material := haymarketResearch{}
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
		material = append(material, models.DiscoveryItem{Kind: models.CandidateKindMoment, EntityID: moment.ID, Title: moment.Title, Text: stop.Body, URL: citations[0].URL, Area: "Chicago", Evidence: "recorded_draft_collection", Origins: []models.DiscoveryOrigin{}})
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
	if err != nil || itinerary.Proposal == nil || len(itinerary.Proposal.Stops) != 2 || itinerary.Proposal.TravelMinutes != nil {
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
	haymarketRead(t, "routing.json", &timing)
	if itinerary.Proposal.PlannedDwellMinutes != dwell || timing.PlannedDwellMinutes != dwell {
		t.Fatalf("packet/proposal dwell mismatch: proposal %d, stops %d, routing %d", itinerary.Proposal.PlannedDwellMinutes, dwell, timing.PlannedDwellMinutes)
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
	if err := e.store.Pool().QueryRow(ctx, `SELECT count(*) FROM places`).Scan(&placesAfter); err != nil || placesAfter != placesBefore+2 {
		t.Fatal("pilot duplicated places", err)
	}

	// Replay real computed geometry locally. Only /route is captured; strict
	// attribution is deliberately unavailable, so quality cannot become pass.
	routeRaw := haymarketRead(t, "valhalla-response.json", nil)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/route" {
			http.Error(w, "strict attribution not captured", http.StatusBadRequest)
			return
		}
		var in struct {
			Costing   string
			Locations []struct{ Lat, Lon float64 }
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Costing != "pedestrian" || len(in.Locations) != len(stops) {
			t.Error("routing inputs differ from captured request")
			http.Error(w, "bad capture input", 400)
			return
		}
		for n, p := range in.Locations {
			if p.Lat != stops[n].Position.Latitude || p.Lon != stops[n].Position.Longitude {
				t.Error("routing point changed without a fresh capture")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(routeRaw)
	}))
	defer provider.Close()
	e.worker = jobs.New(e.store, routing.NewValhalla(provider.URL, provider.Client()), narration.None{}, e.blob, 1, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ordered := []models.StopInput{}
	for n, id := range placeIDs {
		moment := momentIDs[n]
		ordered = append(ordered, models.StopInput{PlaceID: id, FeaturedMomentID: &moment})
	}
	tourResponse := e.do(t, http.MethodPost, "/api/v1/admin/tours", curator, models.CreateTourRequest{CityID: city.ID, Title: "Haymarket and the eight-hour day — private pilot", Modes: []models.TravelMode{models.TravelModeWalk}, Stops: ordered})
	if tourResponse.status != 201 {
		t.Fatalf("tour %d %s", tourResponse.status, tourResponse.raw)
	}
	tourID := tourResponse.body["id"].(string)
	e.drain(t)
	route, err := e.store.GetTourRoute(ctx, tourID, models.TravelModeWalk)
	if err != nil || route.Geometry == nil || route.DistanceM != 1716 || route.DurationS != 1300 || len(route.Legs) != 1 {
		t.Fatalf("captured route %+v %v", route, err)
	}
	review := e.get(t, base+"/review-package?tour_id="+tourID, curator)
	pack := decodePackage(t, review)
	if pack.ReadyToPublish || len(pack.VisitorStories) != 2 || len(pack.Tasks) == 0 || pack.Routes["walk"].Geometry == nil {
		t.Fatalf("unqualified draft package %+v", pack)
	}
	for _, task := range pack.Tasks {
		if task.Code == "tour_itinerary_mismatch" || task.Code == "start_unresolved" || task.Code == "end_unresolved" || task.Code == "brief_not_in_review" {
			t.Fatalf("intake names or itinerary order unresolved: %+v", task)
		}
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
	if folder := os.Getenv("BYL_HAYMARKET_EXPORT"); folder != "" {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "review-package.json"), review.raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
