package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

type discoveryAdapter struct {
	name string
	run  func(context.Context, Request) ([]Found, error)
}

func (a discoveryAdapter) Name() string { return a.name }
func (a discoveryAdapter) Discover(ctx context.Context, r Request) ([]Found, error) {
	return a.run(ctx, r)
}

func TestDiscoveryFoundersNamesakesDuplicatesAndPartialFailure(t *testing.T) {
	in := models.RouteBriefInput{Topic: "Founders", Area: "Downers Grove", PlaceLeads: []string{"Pierce Downer"}, EventLeads: []string{"1832 settlement"}}
	reg := NewRegistry(NewHTTPClient("fixture"), nil)
	for _, name := range []string{"wikipedia_search", "loc", "internet_archive"} {
		reg.Register(discoveryAdapter{name: name, run: func(_ context.Context, r Request) ([]Found, error) {
			if !strings.Contains(r.Query, `"downers grove"`) || !strings.Contains(r.Query, `"founders"`) || !r.CatalogOnly || r.Limit != 12 {
				t.Fatalf("unbounded or irrelevant search: %+v", r)
			}
			if name == "loc" {
				if !strings.Contains(r.Query, `"pierce downer"`) {
					t.Error("place lead missing")
				}
				return nil, errors.New("secret provider response")
			}
			if name == "internet_archive" && !strings.Contains(r.Query, `"1832 settlement"`) {
				t.Error("event lead missing")
			}
			return []Found{
				{Kind: models.CandidateKindMoment, Title: "Pierce Downer", Text: "Downers Grove founder and settlement", URL: "https://history.example/founder#one"},
				{Kind: models.CandidateKindMoment, Title: "Pierce Downer", Text: "An unrelated founder in Iowa", URL: "https://history.example/wrong"},
				{Kind: models.CandidateKindMoment, Title: "Downers Grove sports results", Text: "Team won the match", URL: "https://history.example/sport"},
			}, nil
		}}, Info{SupportsQuery: true})
	}
	report, err := NewDiscovery(reg, DiscoveryPolicy{Providers: []string{"wikipedia_search", "loc", "internet_archive"}}).Run(context.Background(), in, nil, nil, nil, func(context.Context) error { return nil })
	if err != nil || len(report.Items) != 1 || len(report.Items[0].Origins) != 2 || len(report.Providers) != 3 || report.Providers[1].Outcome != "unavailable" {
		t.Fatalf("partial results: %+v %v", report, err)
	}
	if report.Items[0].Evidence != "search_snippet" || len(report.Items[0].Origins[0].Reasons) < 2 || strings.Contains(fmt.Sprint(report), "secret") {
		t.Fatal("lost provenance or leaked provider failure")
	}
}

func TestDiscoveryTrailAreaAndWordBoundaries(t *testing.T) {
	in := models.RouteBriefInput{Topic: "Rail trail", Area: "DuPage County Illinois", PlaceLeads: []string{"Illinois Prairie Path"}}
	scope := &models.DiscoveryScope{BBox: geo.BBox{MinLat: 41, MaxLat: 43, MinLng: -89, MaxLng: -87}}
	for _, tc := range []struct {
		name, text string
		lat, lng   *float64
		want       bool
	}{
		{"right branch", "Illinois Prairie Path rail trail in DuPage County Illinois", nil, nil, true},
		{"namesake", "Illinois Prairie Path rail trail in Iowa", nil, nil, false},
		{"different trail", "Prairie Trail in DuPage County Illinois", nil, nil, false},
		{"outside coordinates", "Illinois Prairie Path rail trail in DuPage County Illinois", f64(46), f64(-88), false},
		{"local coordinates", "Illinois Prairie Path rail trail", f64(42), f64(-88), true},
		{"substring", "Illinois Prairie Pathway in DuPage County Illinois", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DiscoveryReasons(in, models.DiscoveryItem{Text: tc.text, Lat: tc.lat, Lng: tc.lng}, scope)
			if (len(got) > 0) != tc.want {
				t.Fatalf("relevance: %v", got)
			}
		})
	}
}

func TestDiscoveryUsesCollectionBeforeNetwork(t *testing.T) {
	items := []models.DiscoveryItem{}
	for i := range 12 {
		items = append(items, models.DiscoveryItem{Kind: models.CandidateKindPlace, EntityID: fmt.Sprint(i), Title: "Founder", Text: "Downers Grove founders", Evidence: "published_collection"})
	}
	report, err := NewDiscovery(NewRegistry(NewHTTPClient("fixture"), nil), DiscoveryPolicy{Seeds: DefaultDiscoverySeeds, Providers: []string{"loc"}}).Run(context.Background(), models.RouteBriefInput{Topic: "Founders", Area: "Downers Grove"}, nil, items, nil, func(context.Context) error { t.Fatal("collection reuse made a request"); return nil })
	if err != nil || len(report.Items) != 12 || len(report.Sources) != 0 || len(report.Providers) != 0 {
		t.Fatalf("reuse: %+v %v", report, err)
	}
}

func TestDiscoveryInspectsBoundedReferencesAndRetainsEvidence(t *testing.T) {
	paths := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<title>Downers Grove founders</title><article><p>Downers Grove founders made their homes in the region and developed a settlement. This fixture provides more than twenty words of untrusted historical source text for inspection.</p><a href="/founders">Reference</a><a href="/founders#same">Duplicate</a><a href="http://other.test/founders">Off policy</a><a href="/login">Sign in</a></article>`)
	}))
	defer srv.Close()
	c := evidenceTestClient(t, srv)
	used := 0
	report, err := NewDiscovery(NewRegistry(c, nil), DiscoveryPolicy{Seeds: []string{"http://public.test/start"}}).Run(context.Background(), models.RouteBriefInput{Topic: "Founders", Area: "Downers Grove"}, nil, nil, nil, func(context.Context) error { used++; return nil })
	if err != nil || used != 2 || fmt.Sprint(paths) != "[/start /founders]" || len(report.Sources) != 2 || len(report.Items) != 2 {
		t.Fatalf("references: %+v paths=%v used=%d err=%v", report, paths, used, err)
	}
	for _, s := range report.Sources {
		if !s.Result.Untrusted || s.Result.MediaReuse != "unresolved" || len(s.Result.ContentSHA256) != 64 || len(s.Result.Passages) == 0 {
			t.Fatalf("missing evidence: %+v", s)
		}
	}
	if report.Sources[1].Via != "http://public.test/start" {
		t.Fatal("lost reference parent")
	}
}

func TestDiscoveryBudgetsRedirectsAndDoesNotRetryProviders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		redirect bool
	}{{"redirect", true}, {"unavailable", false}} {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				if tc.redirect {
					http.Redirect(w, r, "/again", http.StatusFound)
				} else {
					w.WriteHeader(503)
				}
			}))
			defer srv.Close()
			c := evidenceTestClient(t, srv)
			reg := NewRegistry(c, nil)
			reg.Register(NewLOC(c, "http://public.test"), Info{SupportsQuery: true})
			used := 0
			report, err := NewDiscovery(reg, DiscoveryPolicy{Providers: []string{"loc"}}).Run(context.Background(), models.RouteBriefInput{Topic: "Founders", Area: "Downers Grove"}, nil, nil, nil, func(context.Context) error {
				if used == 1 {
					return ErrDiscoveryBudget
				}
				used++
				return nil
			})
			if err != nil || hits != 1 || used != 1 || len(report.Providers) != 1 || report.Providers[0].Outcome != "unavailable" {
				t.Fatalf("HTTP budget: %+v hits=%d used=%d err=%v", report, hits, used, err)
			}
			if tc.redirect && !strings.Contains(fmt.Sprint(report.Gaps), "request_budget") {
				t.Fatal("budget gap missing")
			}
		})
	}
}

func TestDiscoveryWikipediaSearchAndArchiveCatalogOnly(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search":
			if r.URL.Query().Get("srnamespace") != "0" || r.URL.Query().Get("srlimit") != "12" {
				t.Error("search bounds missing")
			}
			_, _ = fmt.Fprint(w, `{"query":{"search":[{"pageid":42,"title":"Founder","snippet":"<span>Downers Grove</span> founders"}]}}`)
		case "/advancedsearch.php":
			_, _ = fmt.Fprint(w, `{"response":{"docs":[{"identifier":"photo","title":"Downers Grove founder","description":"A photograph of a founder","mediatype":"image"}]}}`)
		default:
			t.Errorf("unexpected media fetch: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	c := evidenceTestClient(t, srv)
	reg := NewRegistry(c, nil)
	reg.Register(NewWikipediaSearch(c, "http://public.test/search"), Info{SupportsQuery: true})
	reg.Register(NewInternetArchive(c, "http://public.test"), Info{SupportsQuery: true})
	report, err := NewDiscovery(reg, DiscoveryPolicy{Providers: []string{"wikipedia_search", "internet_archive"}}).Run(context.Background(), models.RouteBriefInput{Topic: "Founders", Area: "Downers Grove"}, nil, nil, nil, func(context.Context) error { return nil })
	if err != nil || hits != 2 || len(report.Items) != 2 || report.Items[0].Evidence != "search_snippet" || report.Items[1].Evidence != "catalog_only" {
		t.Fatalf("search/catalog: %+v hits=%d err=%v", report, hits, err)
	}
}

func TestDiscoveryReusesRetainedEvidenceAndBlocksOffPolicyRedirect(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, "http://other.test/founders", http.StatusFound)
	}))
	defer srv.Close()
	c := evidenceTestClient(t, srv)
	result := models.EvidenceResult{Outcome: "inspected_page", FinalURL: "http://public.test/retained", Title: "Downers Grove founders", Untrusted: true, MediaReuse: "unresolved", Passages: []models.EvidencePassage{{Reference: "paragraph:1", Text: "Downers Grove founders lived here."}}}
	retained := []models.SourceEvidence{{ID: "snapshot", URL: result.FinalURL, Result: &result}}
	report, err := NewDiscovery(NewRegistry(c, nil), DiscoveryPolicy{Seeds: []string{"http://public.test/retained", "http://public.test/start"}}).Run(context.Background(), models.RouteBriefInput{Topic: "Founders", Area: "Downers Grove", SourceLinks: []string{"http://outside.test/one", "http://outside.test/two", "http://outside.test/three", "http://outside.test/four"}}, nil, nil, retained, func(context.Context) error { return nil })
	if err != nil || hits != 1 || len(report.Items) != 1 || len(report.Sources) != 2 || report.Sources[1].Result.Outcome != "blocked" || report.Sources[0].Via != "retained:snapshot" {
		t.Fatalf("reuse/policy: %+v hits=%d err=%v", report, hits, err)
	}
}
