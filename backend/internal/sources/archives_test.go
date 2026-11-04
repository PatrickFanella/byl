package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestInternetArchiveAdapterSplitsTextsAndMedia(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/advancedsearch.php" {
			w.WriteHeader(404)
			return
		}
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "ia_search.json"))
	}))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	finds, err := NewInternetArchive(c, srv.URL).Discover(context.Background(), Request{Query: "downers grove", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "downers grove") || !strings.Contains(gotQuery, "mediatype:") {
		t.Fatalf("query %q", gotQuery)
	}
	if len(finds) == 0 {
		t.Fatal("expected finds from the fixture")
	}
	for _, f := range finds {
		if !strings.HasPrefix(f.ExternalID, "ia:") || !strings.HasPrefix(f.URL, "https://archive.org/details/") || f.Title == "" {
			t.Fatalf("find %+v", f)
		}
		switch f.Kind {
		case models.CandidateKindMoment:
			if f.Text == "" {
				t.Fatalf("text find without text: %+v", f)
			}
		case models.CandidateKindMedia:
			if !strings.HasPrefix(f.MediaURL, "https://archive.org/download/") {
				t.Fatalf("media find without download url: %+v", f)
			}
		default:
			t.Fatalf("unexpected kind %+v", f)
		}
		if f.Credit == "" {
			t.Fatalf("credit should fall back to the archive: %+v", f)
		}
	}
}

func TestLOCAdapterLicensesOnlyNoKnownRestrictions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fo") != "json" || r.URL.Query().Get("q") == "" {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "loc_search.json"))
	}))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	finds, err := NewLOC(c, srv.URL).Discover(context.Background(), Request{Query: "downers grove", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Found{}
	for _, f := range finds {
		byID[f.ExternalID] = f
	}
	photo := byID["loc:2017760345"]
	if photo.Kind != models.CandidateKindMedia || photo.License != "PD-US-LOC-no-known-restrictions" || !strings.HasPrefix(photo.MediaURL, "https://tile.loc.gov/") || strings.Contains(photo.MediaURL, "#") {
		t.Fatalf("photo: %+v", photo)
	}
	if photo.Credit != "Detroit Publishing Co." || photo.URL != "https://www.loc.gov/item/2017760345/" {
		t.Fatalf("photo credit/url: %+v", photo)
	}
	restricted := byID["loc:2017760346"]
	if restricted.Kind != models.CandidateKindMedia || restricted.License != "" {
		t.Fatalf("restricted photo must have an empty licence: %+v", restricted)
	}
	paper := byID["loc:sn83045487"]
	if paper.Kind != models.CandidateKindMoment || !strings.Contains(paper.Text, "Weekly newspaper") {
		t.Fatalf("newspaper should be a moment find: %+v", paper)
	}
}

func TestURLAdapterExtractsArticleText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/blodgett":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(fixture(t, "local_page.html"))
		case "/huge":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body><p>" + strings.Repeat("x", 3<<20) + "</p></body></html>"))
		case "/redirect":
			http.Redirect(w, r, "https://evil.example/else", http.StatusFound)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	a := NewURL(c, []string{srv.URL + "/blodgett", srv.URL + "/huge", srv.URL + "/redirect", srv.URL + "/missing"})
	finds, err := a.Discover(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(finds) != 1 {
		t.Fatalf("only the readable page should yield a find, got %d: %+v", len(finds), finds)
	}
	f := finds[0]
	if f.Kind != models.CandidateKindMoment || !strings.HasPrefix(f.ExternalID, "url:") || len(f.ExternalID) != len("url:")+64 {
		t.Fatalf("find %+v", f)
	}
	if f.Title != "Blodgett House - Downers Grove Historical Society" || f.URL != srv.URL+"/blodgett" {
		t.Fatalf("title/url %+v", f)
	}
	for _, bad := range []string{"window.analytics", "display:none", "Donate", "We use cookies", "Privacy"} {
		if strings.Contains(f.Text, bad) {
			t.Fatalf("text should not contain %q: %q", bad, f.Text)
		}
	}
	if !strings.Contains(f.Text, "Israel Blodgett") || !strings.Contains(f.Text, "moved to the museum campus in 2008") {
		t.Fatalf("text missing article content: %q", f.Text)
	}
	if f.Credit == "" || f.License != "" {
		t.Fatalf("credit should be the host and licence empty: %+v", f)
	}
	filtered, _ := a.Discover(context.Background(), Request{Query: "Tivoli"})
	if len(filtered) != 0 {
		t.Fatalf("query filter should drop pages without the term, got %d", len(filtered))
	}
	// Skipped pages are reported, not swallowed.
	skips := a.LastSkips()
	if len(skips) != 3 {
		t.Fatalf("expected 3 skips (huge, redirect, missing), got %v", skips)
	}
}

func TestURLAdapterCapsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>Long</title></head><body><main><p>" + strings.Repeat("word ", 10000) + "</p></main></body></html>"))
	}))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	finds, err := NewURL(c, []string{srv.URL}).Discover(context.Background(), Request{})
	if err != nil || len(finds) != 1 {
		t.Fatalf("%v %d", err, len(finds))
	}
	if len(finds[0].Text) > 20000 {
		t.Fatalf("text should be capped at 20000 chars, got %d", len(finds[0].Text))
	}
}
