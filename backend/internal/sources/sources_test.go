package sources

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func TestHTTPClientSendsUserAgentAndRetriesOn429(t *testing.T) {
	var hits atomic.Int32
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewHTTPClient("InvisibleMuseum/test (+https://example.org)")
	c.MinInterval = 0
	resp, err := c.Get(context.Background(), srv.URL+"/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ua != "InvisibleMuseum/test (+https://example.org)" || hits.Load() != 2 {
		t.Fatalf("ua %q hits %d", ua, hits.Load())
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer srv2.Close()
	if _, err := c.Get(context.Background(), srv2.URL); err == nil {
		t.Fatal("two 503s must error")
	}
}

func TestHTTPClientSpacesRequestsPerHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 80 * time.Millisecond
	start := time.Now()
	for i := 0; i < 3; i++ {
		resp, err := c.Get(context.Background(), srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("three requests should take at least two intervals, took %v", elapsed)
	}
}

func TestMapLicense(t *testing.T) {
	cases := map[string]string{
		"CC BY-SA 4.0":  "CC-BY-SA-4.0",
		"CC BY 2.0":     "CC-BY-2.0",
		"CC0":           "CC0-1.0",
		"Public domain": "PD",
		"PDM-owner":     "PD",
		"GFDL":          "GFDL-1.2",
		"":              "",
		"Fair use":      "",
		"CC BY-NC 4.0":  "",
		"CC BY-ND 4.0":  "",
		"GFDL 1.3":      "GFDL-1.3",
	}
	for in, want := range cases {
		if got := MapLicense(in); got != want {
			t.Errorf("MapLicense(%q) = %q want %q", in, got, want)
		}
	}
}

func TestRegistryListsAdaptersWithInfo(t *testing.T) {
	reg := NewRegistry(NewHTTPClient("ua"), []string{"https://example.org/history"})
	names := map[string]Info{}
	for _, i := range reg.List() {
		names[i.Name] = i
	}
	for _, n := range []string{"wikipedia", "wikimedia_commons", "internet_archive", "loc", "url"} {
		if _, ok := names[n]; !ok {
			t.Fatalf("registry missing %s: %v", n, names)
		}
	}
	if !names["wikipedia"].SupportsNear || names["internet_archive"].SupportsNear || !names["internet_archive"].SupportsQuery {
		t.Fatalf("capability flags: %+v", names)
	}
	if _, ok := reg.Get("nope"); ok {
		t.Fatal("unknown adapter must not resolve")
	}
}

// wikiServer replays the fixtures for both the geosearch and the per-page extract calls.
func wikiServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case q.Get("list") == "geosearch":
			_, _ = w.Write(fixture(t, "wikipedia_geosearch.json"))
		case q.Get("pageids") == "21270419":
			_, _ = w.Write(fixture(t, "wikipedia_page_21270419.json"))
		case q.Get("pageids") == "14577874":
			_, _ = w.Write(fixture(t, "wikipedia_page_14577874.json"))
		case q.Get("pageids") != "":
			_, _ = w.Write([]byte(`{"batchcomplete":"","query":{"pages":{"` + q.Get("pageids") + `":{"pageid":` + q.Get("pageids") + `,"ns":0,"title":"Other","extract":"","fullurl":"https://en.wikipedia.org/wiki/Other"}}}}`))
		default:
			w.WriteHeader(400)
		}
	}))
}

func TestWikipediaAdapterProducesPlaceAndMomentFinds(t *testing.T) {
	srv := wikiServer(t)
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	a := NewWikipedia(c, srv.URL+"/w/api.php")
	finds, err := a.Discover(context.Background(), Request{Center: geo.Point{Lat: 41.7959, Lng: -88.0092}, RadiusM: 1500, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Found{}
	for _, f := range finds {
		byID[f.ExternalID] = f
	}
	place, ok := byID["wikipedia:21270419:place"]
	if !ok || place.Kind != models.CandidateKindPlace || place.Title != "Tivoli Theatre (Downers Grove, Illinois)" || place.Lat == nil || *place.Lat < 41.79 {
		t.Fatalf("place find: %+v (have %v)", place, keys(byID))
	}
	if !strings.HasPrefix(place.Text, "The Tivoli Theatre is a movie theater") || strings.Contains(place.Text, "\n") {
		t.Fatalf("place text should be the first paragraph: %q", place.Text)
	}
	moment, ok := byID["wikipedia:21270419:moment"]
	if !ok || moment.Kind != models.CandidateKindMoment || moment.License != "CC-BY-SA-4.0" || moment.Credit != "Wikipedia contributors" || moment.URL != "https://en.wikipedia.org/wiki/Tivoli_Theatre_(Downers_Grove,_Illinois)" {
		t.Fatalf("moment find: %+v", moment)
	}
	if len(moment.Text) < 500 || !strings.Contains(moment.Text, "Christmas Day, 1928") {
		t.Fatalf("moment text should be the full extract: %d chars", len(moment.Text))
	}
	if moment.MediaURL == "" || !strings.HasPrefix(moment.MediaURL, "https://upload.wikimedia.org/") {
		t.Fatalf("page image should be carried as MediaURL: %q", moment.MediaURL)
	}
	// Pages without an extract still yield a place find but no moment find.
	if _, ok := byID["wikipedia:28744450:place"]; !ok {
		t.Fatalf("train wreck place missing: %v", keys(byID))
	}
	if _, ok := byID["wikipedia:28744450:moment"]; ok {
		t.Fatal("a page with an empty extract must not produce a moment find")
	}
}

func TestWikipediaAdapterHonoursLimit(t *testing.T) {
	srv := wikiServer(t)
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	finds, err := NewWikipedia(c, srv.URL+"/w/api.php").Discover(context.Background(), Request{Center: geo.Point{Lat: 41.7959, Lng: -88.0092}, RadiusM: 1500, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]bool{}
	for _, f := range finds {
		pages[strings.Split(f.ExternalID, ":")[1]] = true
	}
	if len(pages) != 1 {
		t.Fatalf("limit 1 should touch one page, got %v", pages)
	}
}

func keys(m map[string]Found) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Review fix: media downloads must not reach loopback, private, link-local or ULA addresses
// (SSRF through a curator-supplied media_url) unless the operator opts in.
func TestPublicIPClassification(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.1", "172.16.5.5", "192.168.1.1", "169.254.169.254", "0.0.0.0", "100.64.0.1", "::1", "fd00::1", "fe80::1", "::ffff:10.0.0.1", "::", "198.18.0.1", "240.0.0.1", "192.0.0.8", "64:ff9b::a00:1", "2002:a00:1::1", "2001:db8::1"}
	for _, s := range blocked {
		if publicIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "207.241.224.2", "2001:4860:4860::8888"} {
		if !publicIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestGetPublicRefusesPrivateHostsUnlessAllowed(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++; _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	ctx := context.Background()
	if _, err := c.GetPublic(ctx, srv.URL+"/x"); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("loopback by IP should be refused, got %v", err)
	}
	if _, err := c.GetPublic(ctx, "http://localhost:"+fmt.Sprint(srv.Listener.Addr().(*net.TCPAddr).Port)+"/x"); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("loopback by name should be refused, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("server was reached %d times", hits)
	}
	c.AllowPrivateHosts = true
	resp, err := c.GetPublic(ctx, srv.URL+"/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if hits != 1 {
		t.Fatalf("hits %d", hits)
	}
}

// Review fix (cycle 3): with HTTP(S)_PROXY set the dialer would only check the proxy's address,
// so the public-only transport must not honour the proxy environment.
func TestPublicOnlyTransportIgnoresProxyEnvironment(t *testing.T) {
	tr, ok := publicOnlyTransport(nil).(*http.Transport)
	if !ok {
		t.Fatal("expected an *http.Transport")
	}
	if tr.Proxy != nil {
		t.Fatal("public-only transport must dial targets directly")
	}
}

func TestGetPublicStreamsBodiesPastTheClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		for range 4 {
			_, _ = w.Write([]byte("chunk"))
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval, c.AllowPrivateHosts = 0, true
	c.HTTP.Timeout = 100 * time.Millisecond
	resp, err := c.GetPublic(context.Background(), srv.URL+"/large")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != strings.Repeat("chunk", 4) {
		t.Fatalf("a slow body must not hit the client timeout: %q %v", body, err)
	}
}
