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
	"invisible-museum/backend/internal/testdb"
)

const foundersDirectory = "../../../docs/product/pilots/downers-founders"

type foundersStop struct {
	Key, Name, Address string
	Position           struct {
		Latitude, Longitude float64
		SourceURL           string `json:"source_url"`
	}
	SourceIDs     []string             `json:"source_ids"`
	MomentTitle   string               `json:"moment_title"`
	DateStart     string               `json:"date_start"`
	DatePrecision models.DatePrecision `json:"date_precision"`
	Body          string
	Gaps          []string
}
type foundersSource struct {
	ID, URL, Title, Publisher string
	RetrievedAt               *models.Date `json:"retrieved_at"`
}

func foundersRead(t *testing.T, name string, target any) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(foundersDirectory, name))
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

type foundersResearch []models.DiscoveryItem

func (f foundersResearch) Process(_ context.Context, task jobs.ResearchTask) (models.ResearchOutput, error) {
	if task.Stage != "research" {
		return models.ResearchOutput{}, jobs.ResearchFailure{Code: "invalid_output"}
	}
	raw, err := json.Marshal(models.DiscoveryReport{Items: f, Sources: []models.DiscoverySource{}, Providers: []models.DiscoveryProvider{{Name: "inspected-pilot-manifest-replay", Outcome: "recorded"}}, Gaps: []string{"Public arrivals, rights-cleared illustrations and route access remain unreviewed."}})
	return models.ResearchOutput{Data: raw}, err
}

// This is a disposable replay of the inspected source manifest. Provider discovery is
// recorded input, and synthetic arrival approval below is explicitly not field evidence.
func TestFoundersPilotReplayReconcilesDraftsAndKeepsPublicationBlocked(t *testing.T) {
	ctx := context.Background()
	var brief models.RouteBriefInput
	foundersRead(t, "brief.json", &brief)
	var stops []foundersStop
	foundersRead(t, "stops.json", &stops)
	var sources []foundersSource
	foundersRead(t, "sources.json", &sources)
	sourceByID := map[string]foundersSource{}
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	if len(stops) != 4 || brief.DurationMinutes != nil {
		t.Fatal("fixed pilot or unsupported time promise changed")
	}
	e := newTestEnv(t)
	e.drain(t)
	curator := e.signIn(t, "founders-curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "founders-visitor@example.com", "")
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
		item := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: stop.Name, Text: stop.Body, URL: sourceByID[stop.SourceIDs[0]].URL, Lat: &lat, Lng: &lng, Area: "Downers Grove", Evidence: "recorded_source_review", Origins: []models.DiscoveryOrigin{}, Identity: &models.IdentityFacts{CityID: testdb.SeedCityID, PlaceName: stop.Name, Address: stop.Address}}
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
	material := foundersResearch{}
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
		material = append(material, models.DiscoveryItem{Kind: models.CandidateKindMoment, EntityID: moment.ID, Title: moment.Title, Text: stop.Body, URL: citations[0].URL, Area: "Downers Grove", Evidence: "recorded_draft_collection", Origins: []models.DiscoveryOrigin{}})
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
	if err != nil || itinerary.Proposal == nil || len(itinerary.Proposal.Stops) != 4 || itinerary.Proposal.TravelMinutes != nil {
		raw, _ := json.MarshalIndent(itinerary, "", "  ")
		t.Fatalf("local assembly: %s %v", raw, err)
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
	if err := e.store.Pool().QueryRow(ctx, `SELECT count(*) FROM places`).Scan(&placesAfter); err != nil || placesAfter != placesBefore+4 {
		t.Fatal("pilot duplicated places", err)
	}

	// Replay real computed geometry locally. Only /route is captured; strict
	// attribution is deliberately unavailable, so quality cannot become pass.
	routeRaw := foundersRead(t, "valhalla-response.json", nil)
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
	tourResponse := e.do(t, http.MethodPost, "/api/v1/admin/tours", curator, models.CreateTourRequest{CityID: testdb.SeedCityID, Title: "Founding Downers Grove — private pilot", Modes: []models.TravelMode{models.TravelModeWalk}, Stops: ordered})
	if tourResponse.status != 201 {
		t.Fatalf("tour %d %s", tourResponse.status, tourResponse.raw)
	}
	tourID := tourResponse.body["id"].(string)
	e.drain(t)
	route, err := e.store.GetTourRoute(ctx, tourID, models.TravelModeWalk)
	if err != nil || route.Geometry == nil || route.DistanceM != 2722 || route.DurationS != 1989 || len(route.Legs) != 3 {
		t.Fatalf("captured route %+v %v", route, err)
	}
	review := e.get(t, base+"/review-package?tour_id="+tourID, curator)
	pack := decodePackage(t, review)
	if pack.ReadyToPublish || len(pack.VisitorStories) != 4 || len(pack.Tasks) == 0 || pack.Routes["walk"].Geometry == nil {
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
	// not a claim that this coordinate is an actual cemetery entrance.
	lat, lng := stops[0].Position.Latitude+0.0001, stops[0].Position.Longitude
	arrivalBase := "/api/v1/admin/places/" + placeIDs[0] + "/arrivals"
	proposal := e.do(t, http.MethodPost, arrivalBase, curator, models.ArrivalProposalRequest{Point: models.ArrivalPoint{Name: "Synthetic correction only; not a real entrance", Kind: "viewpoint", Latitude: &lat, Longitude: &lng, Access: []models.ArrivalAccess{{Mode: models.TravelModeWalk, Status: "unknown", Surface: "unknown", SourceURL: "https://example.org/synthetic-founders-correction", SourceTitle: "Synthetic software exercise; no field visit", RetrievedAt: "2026-11-08"}, {Mode: models.TravelModeBike, Status: "unknown", Surface: "unknown", SourceURL: "https://example.org/synthetic-founders-correction", SourceTitle: "Synthetic software exercise; no field visit", RetrievedAt: "2026-11-08"}}}})
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
	if folder := os.Getenv("BYL_FOUNDERS_EXPORT"); folder != "" {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "review-package.json"), review.raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
