package transit

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func recordedCTA(t *testing.T, mutate func(map[string]string)) ([]byte, time.Time) {
	t.Helper()
	files := map[string]string{}
	entries, err := os.ReadDir("testdata/cta-blue")
	if err != nil {
		t.Fatal(err)
	}
	var provenance struct {
		RetrievedAt time.Time `json:"retrieved_at"`
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join("testdata/cta-blue", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(entry.Name(), ".txt") {
			files[entry.Name()] = string(raw)
		}
		if entry.Name() == "provenance.json" {
			if err := json.Unmarshal(raw, &provenance); err != nil {
				t.Fatal(err)
			}
		}
	}
	if mutate != nil {
		mutate(files)
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for name, raw := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), provenance.RetrievedAt
}

func recordedRequest() Request {
	return Request{BoardStopID: "30374", AlightStopID: "30077", Mode: "walk", DepartureAt: time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC)}
}

func TestCTARecordedChicagoToForestParkConnection(t *testing.T) {
	data, now := recordedCTA(t, nil)
	f, err := ParseCTA(data, now)
	if err != nil {
		t.Fatal(err)
	}
	c := f.Connect(recordedRequest(), now)
	if c.State != "scheduled" || !c.Scheduled || c.Live || c.TripID != "92281538010" || c.RouteID != "Blue" || c.Headsign != "Forest Park" {
		t.Fatalf("connection: %+v", c)
	}
	if c.Board.ID != "30374" || c.Alight.ID != "30077" || c.Source.Timezone != "America/Chicago" || len(c.Source.SHA256) != 64 {
		t.Fatal("missing source/platform context")
	}
	if c.DepartureAt.Format(time.RFC3339) != "2026-11-08T16:09:30Z" || c.ArrivalAt.Format(time.RFC3339) != "2026-11-08T16:53:00Z" {
		t.Fatalf("times: %v %v", c.DepartureAt, c.ArrivalAt)
	}
}

func TestCTAFailuresNeverInventAConnection(t *testing.T) {
	data, now := recordedCTA(t, nil)
	f, err := ParseCTA(data, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, state string
		change      func(*Request)
		clock       time.Time
	}{
		{"missed departure", "no_service", func(r *Request) { r.DepartureAt = r.DepartureAt.Add(time.Hour) }, now},
		{"wrong direction", "no_service", func(r *Request) { r.BoardStopID, r.AlightStopID = r.AlightStopID, r.BoardStopID }, now},
		{"missing platform", "unavailable", func(r *Request) { r.BoardStopID = "not-a-stop" }, now},
		{"different rail line", "unavailable", func(r *Request) { r.BoardStopID = "30074" }, now},
		{"unsupported transfer", "unsupported", func(r *Request) { r.Transfers = 1 }, now},
		{"bicycle unknown", "unsupported", func(r *Request) { r.Mode = "bike" }, now},
		{"stale", "stale", func(r *Request) { r.DepartureAt = r.DepartureAt.Add(48 * time.Hour) }, now.Add(48 * time.Hour)},
		{"outside calendar", "no_service", func(r *Request) { r.DepartureAt = time.Date(2026, 12, 6, 16, 0, 0, 0, time.UTC) }, time.Date(2026, 12, 6, 12, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := recordedRequest()
			tc.change(&r)
			candidate := *f
			if tc.name == "outside calendar" {
				candidate.Source.RetrievedAt = tc.clock
			}
			c := candidate.Connect(r, tc.clock)
			if c.State != tc.state || c.DepartureAt != nil || c.ArrivalAt != nil || c.TripID != "" {
				t.Fatalf("invented/incorrect service: %+v", c)
			}
		})
	}
}

func TestCTACalendarExceptionFrequencyAndPickupRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"removed service", func(f map[string]string) {
			f["calendar_dates.txt"] = "service_id,date,exception_type\n109209,20261108,2\n"
		}},
		{"frequency only", func(f map[string]string) {
			f["frequencies.txt"] = "trip_id,start_time,end_time,headway_secs\n92281538010,10:00:00,11:00:00,600\n"
		}},
		{"no pickup", func(f map[string]string) {
			f["stop_times.txt"] = strings.Replace(f["stop_times.txt"], "Forest Park,0,91265", "Forest Park,1,91265", 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, now := recordedCTA(t, tc.mutate)
			f, err := ParseCTA(data, now)
			if err != nil {
				t.Fatal(err)
			}
			c := f.Connect(recordedRequest(), now)
			if c.State != "no_service" {
				t.Fatalf("unsupported service promoted: %+v", c)
			}
		})
	}
}

func TestCTAGTFSServiceTimesBeyondMidnight(t *testing.T) {
	data, _ := recordedCTA(t, func(f map[string]string) {
		f["stop_times.txt"] = strings.ReplaceAll(strings.ReplaceAll(f["stop_times.txt"], "10:09:30", "34:09:30"), "10:53:00", "34:53:00")
	})
	now := time.Date(2026, 11, 9, 15, 59, 0, 0, time.UTC)
	f, err := ParseCTA(data, now)
	if err != nil {
		t.Fatal(err)
	}
	r := recordedRequest()
	r.DepartureAt = now
	c := f.Connect(r, now)
	if c.State != "scheduled" || c.DepartureAt.Format(time.RFC3339) != "2026-11-09T16:09:30Z" {
		t.Fatalf("previous service day: %+v", c)
	}
}

func TestCTAMalformedFeedFailsClosed(t *testing.T) {
	for _, mutate := range []func(map[string]string){
		func(f map[string]string) { delete(f, "calendar.txt") },
		func(f map[string]string) {
			f["agency.txt"] = strings.ReplaceAll(f["agency.txt"], "America/Chicago", "Etc/UTC")
		},
		func(f map[string]string) {
			f["stop_times.txt"] = strings.ReplaceAll(f["stop_times.txt"], "10:53:00", "09:53:00")
		},
		func(f map[string]string) { f["stops.txt"] = strings.ReplaceAll(f["stops.txt"], "41.885737", "NaN") },
		func(f map[string]string) {
			f["calendar_dates.txt"] = "service_id,date,exception_type\n109209,20261108,1\n109209,20261108,2\n"
		},
	} {
		data, now := recordedCTA(t, mutate)
		if _, err := ParseCTA(data, now); err == nil {
			t.Fatal("malformed feed accepted")
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCTAClientCoalescesFetchAndDoesNotReuseStaleService(t *testing.T) {
	data, now := recordedCTA(t, nil)
	var calls atomic.Int32
	var failed atomic.Bool
	c := Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != FeedURL {
			t.Error("unexpected provider URL")
		}
		calls.Add(1)
		if failed.Load() {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if result := c.Connect(context.Background(), recordedRequest(), now); result.State != "scheduled" {
				t.Errorf("connection: %s", result.State)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate fetches: %d", calls.Load())
	}
	failed.Store(true)
	later := now.Add(25 * time.Hour)
	r := recordedRequest()
	r.DepartureAt = later.Add(time.Hour)
	for range 2 {
		result := c.Connect(context.Background(), r, later)
		if result.State != "stale" || result.DepartureAt != nil {
			t.Fatalf("stale timetable reused: %+v", result)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("refresh failure retry storm: %d", calls.Load())
	}
}

func TestCTAProviderFailureAndUnsupportedMode(t *testing.T) {
	var calls int
	c := Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})}}
	r := recordedRequest()
	r.Mode = "bike"
	if result := c.Connect(context.Background(), r, time.Now()); result.State != "unsupported" || calls != 0 {
		t.Fatal("unsupported mode fetched or invented service")
	}
	r.Mode = "walk"
	if result := c.Connect(context.Background(), r, time.Now()); result.State != "provider_unavailable" || result.DepartureAt != nil {
		t.Fatalf("provider failure: %+v", result)
	}
}

func TestCTAInjectedClientCannotFollowForeignRedirect(t *testing.T) {
	data, now := recordedCTA(t, nil)
	var foreign bool
	c := Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == FeedURL {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.example/feed.zip"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		foreign = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
	result := c.Connect(context.Background(), recordedRequest(), now)
	if foreign || result.State != "provider_unavailable" || result.DepartureAt != nil {
		t.Fatalf("foreign source fetched/accepted: fetched=%v result=%+v", foreign, result)
	}
}

func TestCTAClientRefreshSurvivesCancelledCallers(t *testing.T) {
	data, now := recordedCTA(t, nil)
	var calls atomic.Int32
	release := make(chan struct{})
	c := Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		if result := c.Connect(ctx, recordedRequest(), now); result.State == "scheduled" {
			t.Fatal("a cancelled caller received a connection before the feed loaded")
		}
		cancel()
	}
	if calls.Load() != 1 {
		t.Fatalf("each cancelled caller restarted the download: %d fetches", calls.Load())
	}
	close(release)
	if result := c.Connect(context.Background(), recordedRequest(), now); result.State != "scheduled" {
		t.Fatalf("the shared refresh did not finish for later callers: %s", result.State)
	}
	if calls.Load() != 1 {
		t.Fatalf("fetches: %d", calls.Load())
	}
}
