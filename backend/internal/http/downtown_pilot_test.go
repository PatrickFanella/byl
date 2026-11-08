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
	"strings"
	"testing"

	"invisible-museum/backend/internal/jobs"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/testdb"
)

const downtownDirectory = "../../../docs/product/pilots/downers-downtown"

type downtownStop struct {
	Name, Address, Body string
	ExistingPlaceID     string               `json:"existing_place_id"`
	ExistingMomentID    string               `json:"existing_moment_id"`
	RetainedStorySHA256 string               `json:"retained_story_sha256"`
	HistoricalAddresses []string             `json:"historical_addresses"`
	MomentTitle         string               `json:"moment_title"`
	DateStart           string               `json:"date_start"`
	DatePrecision       models.DatePrecision `json:"date_precision"`
	SourceIDs           []string             `json:"source_ids"`
	Position            struct{ Latitude, Longitude float64 }
}

func downtownRead(t *testing.T, name string, target any) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(downtownDirectory, name))
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

type downtownResearch []models.DiscoveryItem

func (d downtownResearch) Process(_ context.Context, task jobs.ResearchTask) (models.ResearchOutput, error) {
	if task.Stage != "research" {
		return models.ResearchOutput{}, jobs.ResearchFailure{Code: "invalid_output"}
	}
	raw, err := json.Marshal(models.DiscoveryReport{Items: d, Sources: []models.DiscoverySource{}, Providers: []models.DiscoveryProvider{{Name: "inspected-downtown-manifest-replay", Outcome: "recorded"}}, Gaps: []string{"Brick segment, current crossings, arrivals and illustration rights remain unreviewed."}})
	return models.ResearchOutput{Data: raw}, err
}

// This replay exercises private draft preparation, not real-world access approval.
func TestDowntownPilotReusesStoriesComputesLoopAndRetainsCrossingReport(t *testing.T) {
	ctx := context.Background()
	var stops []downtownStop
	downtownRead(t, "stops.json", &stops)
	var brief models.RouteBriefInput
	downtownRead(t, "brief.json", &brief)
	var sources []struct {
		ID, URL, Title, Publisher string
		RetrievedAt               *models.Date `json:"retrieved_at"`
	}
	downtownRead(t, "sources.json", &sources)
	if len(stops) != 5 || brief.TripShape != "loop" || brief.DurationMinutes != nil {
		t.Fatal("pilot identity or time assumptions changed")
	}
	e := newTestEnv(t)
	e.drain(t)
	curator := e.signIn(t, "downtown-curator@example.com", models.AccountRoleCurator)
	created := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs", curator, brief)
	if created.status != 201 {
		t.Fatalf("brief %d %s", created.status, created.raw)
	}
	base := "/api/v1/admin/route-briefs/" + created.body["id"].(string)
	var placesBefore int
	if err := e.store.Pool().QueryRow(ctx, `SELECT count(*) FROM places`).Scan(&placesBefore); err != nil {
		t.Fatal(err)
	}
	ordered := []models.StopInput{}
	material := downtownResearch{}
	for _, stop := range stops {
		lat, lng := stop.Position.Latitude, stop.Position.Longitude
		item := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: stop.Name, Text: stop.Body, Area: "Downers Grove", Evidence: "recorded_source_review", Origins: []models.DiscoveryOrigin{}, Lat: &lat, Lng: &lng, Identity: &models.IdentityFacts{CityID: testdb.SeedCityID, PlaceName: stop.Name, Address: stop.Address, HistoricalAddresses: stop.HistoricalAddresses}}
		for _, source := range sources {
			if source.ID == stop.SourceIDs[0] {
				item.URL = source.URL
			}
		}
		input := models.ReconciliationRequest{Version: 1, Item: item}
		first := e.do(t, http.MethodPost, base+"/reconciliation", curator, input)
		if first.status != 200 {
			t.Fatalf("reconcile %s %d %s", stop.Name, first.status, first.raw)
		}
		var record models.ReconciliationRecord
		if err := json.Unmarshal(first.raw, &record); err != nil {
			t.Fatal(err)
		}
		repeated := e.do(t, http.MethodPost, base+"/reconciliation", curator, input)
		if repeated.status != 200 || repeated.body["id"] != record.ID {
			t.Fatal("identity replay duplicated a record")
		}
		place := stop.ExistingPlaceID
		if place != "" {
			if record.Proposal.PlaceID != place || record.PlaceCandidateID != nil {
				t.Fatalf("existing identity not reused: %+v", record)
			}
		} else if record.PlaceCandidateID != nil {
			id := *record.PlaceCandidateID
			approved := e.do(t, http.MethodPost, "/api/v1/admin/candidates/"+id+"/approve", curator, map[string]any{"note": "Disposable draft identity review only; no real arrival/access approval."})
			if approved.status != 200 {
				t.Fatalf("candidate %d %s", approved.status, approved.raw)
			}
			candidate, err := e.store.GetCandidate(ctx, id)
			if err != nil || candidate.ApprovedEntityID == nil {
				t.Fatal("draft place missing", err)
			}
			place = *candidate.ApprovedEntityID
		} else {
			if record.Proposal.Outcome != "needs_review" {
				t.Fatalf("unexplained identity hold: %+v", record)
			}
			// A historical address or nearby distinct place intentionally blocks
			// automatic creation. A curator explicitly creates the draft instead.
			draft := e.do(t, http.MethodPost, "/api/v1/admin/places", curator, map[string]any{"city_id": testdb.SeedCityID, "name": stop.Name, "address": stop.Address, "latitude": lat, "longitude": lng})
			if draft.status != 201 {
				t.Fatalf("manual draft %d %s", draft.status, draft.raw)
			}
			place = draft.body["id"].(string)
		}
		momentID := stop.ExistingMomentID
		if momentID == "" {
			citations := []models.Source{}
			for _, id := range stop.SourceIDs {
				for _, source := range sources {
					if source.ID == id {
						citations = append(citations, models.Source{Title: source.Title, Publisher: source.Publisher, URL: source.URL, RetrievedAt: source.RetrievedAt})
					}
				}
			}
			moment, err := e.store.CreateMoment(ctx, models.CreateMomentRequest{PlaceID: place, Title: stop.MomentTitle, Body: stop.Body, DateStart: stop.DateStart, DatePrecision: stop.DatePrecision, Sources: citations})
			if err != nil {
				t.Fatal(err)
			}
			momentID = moment.ID
		} else {
			var body string
			if err := e.store.Pool().QueryRow(ctx, `SELECT body FROM moments WHERE id=$1 AND place_id=$2`, momentID, place).Scan(&body); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", sha256.Sum256([]byte(body))) != stop.RetainedStorySHA256 || body == stop.Body {
				t.Fatal("existing Tivoli prose replaced or selection note mistaken for story")
			}
		}
		decision := models.IdentityDecisionRequest{Version: record.ReviewVersion, Outcome: "existing_place_existing_moment", PlaceID: place, MomentID: momentID, HistoricalAddresses: stop.HistoricalAddresses, Note: "Disposable curator selection; retains original place/story identities and address aliases. No public release."}
		if r := e.do(t, http.MethodPut, base+"/reconciliation/"+record.ID+"/decision", curator, decision); r.status != 200 {
			t.Fatalf("identity decision %d %s", r.status, r.raw)
		}
		ordered = append(ordered, models.StopInput{PlaceID: place, FeaturedMomentID: &momentID})
		material = append(material, models.DiscoveryItem{Kind: models.CandidateKindMoment, EntityID: momentID, Title: stop.MomentTitle, Text: stop.Body, URL: item.URL, Area: "Downers Grove", Evidence: "recorded_draft_collection", Origins: []models.DiscoveryOrigin{}})
	}
	var placesAfter int
	if err := e.store.Pool().QueryRow(ctx, `SELECT count(*) FROM places`).Scan(&placesAfter); err != nil || placesAfter != placesBefore+3 {
		t.Fatal("station/theatre duplicated", err)
	}
	e.worker.Research, e.worker.Reconcile, e.worker.Assemble = material, true, true
	if r := e.do(t, http.MethodPost, base+"/research", curator, map[string]any{"version": 1}); r.status != 202 {
		t.Fatalf("research %d %s", r.status, r.raw)
	}
	e.drain(t)
	tourResponse := e.do(t, http.MethodPost, "/api/v1/admin/tours", curator, models.CreateTourRequest{CityID: testdb.SeedCityID, Title: "Downtown architecture and commerce — private pilot", Modes: []models.TravelMode{models.TravelModeWalk}, Stops: ordered})
	if tourResponse.status != 201 {
		t.Fatalf("tour %d %s", tourResponse.status, tourResponse.raw)
	}
	tourID := tourResponse.body["id"].(string)
	e.drain(t)
	routeRaw := downtownRead(t, "valhalla-response.json", nil)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/route" {
			http.Error(w, "network attribution not captured", 400)
			return
		}
		var in struct {
			Costing   string
			Locations []struct {
				Lat, Lon float64
				Type     string
			}
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Costing != "pedestrian" || len(in.Locations) != len(stops)+1 {
			t.Error("captured input mismatch")
			http.Error(w, "wrong route input", 400)
			return
		}
		for i, point := range in.Locations {
			stop := stops[i%len(stops)]
			if point.Lat != stop.Position.Latitude || point.Lon != stop.Position.Longitude || point.Type != "break" {
				t.Error("capture controls changed")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(routeRaw); err != nil {
			t.Error(err)
		}
	}))
	defer provider.Close()
	points := []routing.Point{}
	for _, stop := range stops {
		points = append(points, routing.Point{Lat: stop.Position.Latitude, Lng: stop.Position.Longitude})
	}
	points = append(points, points[0])
	computed, err := routing.NewValhalla(provider.URL, provider.Client()).Route(ctx, models.TravelModeWalk, points)
	if err != nil || computed.DistanceM != 1246 || computed.DurationS != 886 || len(computed.Legs) != 5 {
		t.Fatalf("captured loop %+v %v", computed, err)
	}
	// Import the actual comparison into a disposable preview, without asserting
	// controlled navigation, quality approval, network attribution or arrivals.
	// Production corridor controls correctly reject a naive straight reference.
	draftTour, err := e.store.GetTour(ctx, tourID, false)
	if err != nil {
		t.Fatal(err)
	}
	stored := models.TourRoute{TourID: tourID, Mode: models.TravelModeWalk, Status: models.RouteStatusReady, Provider: computed.Provider, DistanceM: computed.DistanceM, DurationS: computed.DurationS, Geometry: &models.LineString{Type: "LineString", Coordinates: computed.Geometry}}
	for i, leg := range computed.Legs {
		next := (i + 1) % len(stops)
		stored.Legs = append(stored.Legs, models.RouteLeg{FromStopID: draftTour.Stops[i].ID, ToStopID: draftTour.Stops[next].ID, FromPosition: i + 1, ToPosition: next + 1, Return: next == 0, NavigationSource: "unapproved_computed_comparison", DistanceM: leg.DistanceM, DurationS: leg.DurationS, Geometry: leg.Geometry})
	}
	if err := e.store.UpsertRoute(ctx, stored); err != nil {
		t.Fatal(err)
	}
	route, err := e.store.GetTourRoute(ctx, tourID, models.TravelModeWalk)
	if err != nil || route.Geometry == nil || len(route.Legs) != 5 || !route.Legs[4].Return || route.Quality != nil {
		t.Fatalf("unqualified comparison %+v %v", route, err)
	}

	review := e.get(t, base+"/review-package?tour_id="+tourID, curator)
	pack := decodePackage(t, review)
	if pack.ReadyToPublish || len(pack.VisitorStories) != 5 || len(pack.Tasks) == 0 || pack.Routes["walk"].Geometry == nil {
		t.Fatalf("unqualified pilot package %+v", pack)
	}
	if r := e.get(t, "/api/v1/tours/"+tourID, ""); r.status != 404 {
		t.Fatal("private pilot released")
	}
	// A frozen seed route is a stand-in for report submission only. The new pilot
	// remains private; this cannot be mistaken for an observed downtown crossing.
	e.publishFixture(t, testdb.SeedTourID)
	publicPath := "/api/v1/tours/" + testdb.SeedTourID
	before := e.get(t, publicPath, "")
	input := reportBody(t, before.body)
	input["reason"], input["better"], input["details"] = "improvement", "Synthetic test: review a crossing before selecting a public approach.", "SOFTWARE FIXTURE; not a field report, crossing inspection or access approval."
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	status, _, report := submitReport(t, e, curator, "synthetic-downtown-crossing", string(raw))
	if status != 201 {
		t.Fatalf("crossing report %d %+v", status, report)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/route-reports/"+report["id"].(string)+"/state", curator, map[string]any{"version": 1, "status": "accepted", "reason": "Synthetic review only. Independent route proposal still required."}); r.status != 200 {
		t.Fatalf("report review %d %s", r.status, r.raw)
	}
	after := e.get(t, publicPath, "")
	if string(before.raw) != string(after.raw) || strings.Contains(string(after.raw), "SOFTWARE FIXTURE") {
		t.Fatal("private crossing report changed published route")
	}
	if folder := os.Getenv("BYL_DOWNTOWN_EXPORT"); folder != "" {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "review-package.json"), review.raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
