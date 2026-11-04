package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

func commonsServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case q.Get("list") == "geosearch" && q.Get("gsnamespace") == "6":
			_, _ = w.Write(fixture(t, "commons_geosearch.json"))
		case q.Get("prop") == "imageinfo":
			_, _ = w.Write(fixture(t, "commons_imageinfo.json"))
		default:
			w.WriteHeader(400)
		}
	}))
}

func TestCommonsAdapterProducesLicensedMediaFinds(t *testing.T) {
	srv := commonsServer(t)
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	finds, err := NewCommons(c, srv.URL+"/w/api.php").Discover(context.Background(), Request{Center: geo.Point{Lat: 41.7959, Lng: -88.0092}, RadiusM: 1000, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(finds) != 3 {
		t.Fatalf("expected 3 media finds, got %d", len(finds))
	}
	byID := map[string]Found{}
	for _, f := range finds {
		byID[f.ExternalID] = f
	}
	station, ok := byID["commons:176527995"]
	if !ok {
		station, ok = byID["commons:133781029"]
	}
	for _, f := range finds {
		if f.Kind != models.CandidateKindMedia || f.MimeType != "image/jpeg" || f.Width == 0 || f.Height == 0 {
			t.Fatalf("media find: %+v", f)
		}
		if !strings.HasPrefix(f.MediaURL, "https://upload.wikimedia.org/") || !strings.HasPrefix(f.URL, "https://commons.wikimedia.org/wiki/File:") {
			t.Fatalf("urls: %+v", f)
		}
		if f.License != "CC-BY-SA-4.0" && f.License != "CC-BY-2.0" {
			t.Fatalf("licence should be mapped: %+v", f)
		}
		if f.LicenseURL == "" || f.Credit == "" || strings.Contains(f.Credit, "<a ") || strings.Contains(f.Text, "<") {
			t.Fatalf("credit and caption must be plain text: credit=%q text=%q licurl=%q", f.Credit, f.Text, f.LicenseURL)
		}
	}
	_ = ok
	_ = station
}

func TestCommonsUnknownLicenceStaysEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("list") == "geosearch" {
			_, _ = w.Write([]byte(`{"query":{"geosearch":[{"pageid":1,"ns":6,"title":"File:Odd.jpg","lat":41.79,"lon":-88.0}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"query":{"pages":{"1":{"pageid":1,"title":"File:Odd.jpg","imageinfo":[{"url":"https://upload.wikimedia.org/x/Odd.jpg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Odd.jpg","mime":"image/jpeg","width":10,"height":10,"extmetadata":{"LicenseShortName":{"value":"Fair use"},"Artist":{"value":"Someone"}}}]}}}}`))
	}))
	defer srv.Close()
	c := NewHTTPClient("ua")
	c.MinInterval = 0
	finds, err := NewCommons(c, srv.URL).Discover(context.Background(), Request{Center: geo.Point{Lat: 41.79, Lng: -88}})
	if err != nil || len(finds) != 1 || finds[0].License != "" {
		t.Fatalf("%v %+v", err, finds)
	}
}
