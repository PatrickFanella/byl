// Package transit provides bounded scheduled connections, never live arrivals or
// an unrestricted journey planner. The first provider supports one CTA rail ride.
package transit

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // The release image has no host timezone database.
)

const (
	FeedURL    = "https://www.transitchicago.com/downloads/sch_data/google_transit.zip"
	LicenseURL = "https://www.transitchicago.com/developers/terms/"
	MaxFeedAge = 24 * time.Hour
	// The November 2026 feed is 68.7 MiB and expands to 409 MiB, with 5.92 million stop_times rows
	// across two overlapping service periods. These limits leave room for a feed about twice that size.
	maxArchive  = 160 << 20
	maxTable    = 1 << 30
	maxExpanded = 5 << 28 // 1.25 GiB
	maxRows     = 15_000_000
)

type Source struct {
	URL         string    `json:"url"`
	Publisher   string    `json:"publisher"`
	SHA256      string    `json:"sha256"`
	RetrievedAt time.Time `json:"retrieved_at"`
	LicenseURL  string    `json:"license_url"`
	Timezone    string    `json:"timezone"`
}

type Stop struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ParentID  string  `json:"parent_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Request struct {
	BoardStopID  string
	AlightStopID string
	DepartureAt  time.Time // Earliest acceptable boarding, after walking/buffer.
	Mode         string
	Transfers    int
}

type Planner interface {
	Connect(context.Context, Request, time.Time) Connection
}

type Connection struct {
	State       string     `json:"state"`
	Scheduled   bool       `json:"scheduled"`
	Live        bool       `json:"live"`
	Source      *Source    `json:"source,omitempty"`
	Board       *Stop      `json:"board,omitempty"`
	Alight      *Stop      `json:"alight,omitempty"`
	TripID      string     `json:"trip_id,omitempty"`
	RouteID     string     `json:"route_id,omitempty"`
	Headsign    string     `json:"headsign,omitempty"`
	DepartureAt *time.Time `json:"departure_at,omitempty"`
	ArrivalAt   *time.Time `json:"arrival_at,omitempty"`
	Issues      []string   `json:"issues"`
}

func unavailable(state, issue string, source *Source) Connection {
	return Connection{State: state, Scheduled: true, Source: source, Issues: []string{issue}}
}

type service struct {
	start, end string
	weekdays   [7]bool
}
type trip struct {
	service   string
	frequency bool
}
type stopTime struct {
	stop, headsign               string
	sequence, arrival, departure int
	pickup, dropoff              bool
}

// Feed keeps only the Blue Line timetable, not the source archive or bus network.
type Feed struct {
	Source     Source
	stops      map[string]Stop
	services   map[string]service
	exceptions map[string]map[string]bool
	trips      map[string]trip
	times      map[string][]stopTime
	platforms  map[string]bool
	zone       *time.Location
}

type row struct {
	fields map[string]int
	values []string
}

func (r row) get(name string) string {
	if i, ok := r.fields[name]; ok {
		return r.values[i]
	}
	return ""
}

// table reads in place without extracting archive paths. Limits apply even to
// optional/unselected rows; oversized or malformed feeds fail closed.
func table(files map[string]*zip.File, name string, required []string, visit func(row) error) error {
	f, ok := files[name]
	if !ok {
		return fmt.Errorf("missing GTFS table %s", name)
	}
	if f.UncompressedSize64 > maxTable {
		return errors.New("GTFS table exceeds size limit")
	}
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	r := csv.NewReader(io.LimitReader(in, maxTable+1))
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		return err
	}
	fields := map[string]int{}
	for i, h := range header {
		h = strings.TrimPrefix(h, "\ufeff")
		if _, ok := fields[h]; ok {
			return errors.New("duplicate GTFS column")
		}
		fields[h] = i
	}
	for _, h := range required {
		if _, ok := fields[h]; !ok {
			return fmt.Errorf("missing GTFS column %s", h)
		}
	}
	for count := 0; ; count++ {
		values, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if count >= maxRows {
			return errors.New("GTFS row limit exceeded")
		}
		for _, v := range values {
			if len(v) > 4096 {
				return errors.New("GTFS field exceeds size limit")
			}
		}
		if err = visit(row{fields, values}); err != nil {
			return err
		}
	}
}

func date(raw string) bool {
	t, e := time.Parse("20060102", raw)
	return e == nil && t.Format("20060102") == raw
}
func clock(raw string) (int, error) {
	p := strings.Split(raw, ":")
	if len(p) != 3 {
		return 0, errors.New("invalid GTFS time")
	}
	v := [3]int{}
	for i, s := range p {
		if len(s) != 2 {
			return 0, errors.New("invalid GTFS time")
		}
		n, e := strconv.Atoi(s)
		if e != nil || n < 0 {
			return 0, errors.New("invalid GTFS time")
		}
		v[i] = n
	}
	if v[0] >= 48 || v[1] >= 60 || v[2] >= 60 {
		return 0, errors.New("unsupported GTFS time")
	}
	return v[0]*3600 + v[1]*60 + v[2], nil
}

func ParseCTA(data []byte, retrievedAt time.Time) (*Feed, error) {
	if len(data) == 0 || len(data) > maxArchive {
		return nil, errors.New("GTFS archive exceeds size limit")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	var size uint64
	for _, file := range z.File {
		if file.UncompressedSize64 > maxTable {
			return nil, errors.New("GTFS table exceeds size limit")
		}
		if _, ok := files[file.Name]; ok {
			return nil, errors.New("duplicate GTFS table")
		}
		files[file.Name] = file
		size += file.UncompressedSize64
		if size > maxExpanded {
			return nil, errors.New("GTFS archive expands beyond size limit")
		}
	}
	hash := sha256.Sum256(data)
	zone, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return nil, err
	}
	f := &Feed{Source: Source{URL: FeedURL, Publisher: "Chicago Transit Authority", SHA256: hex.EncodeToString(hash[:]), RetrievedAt: retrievedAt.UTC(), LicenseURL: LicenseURL, Timezone: zone.String()}, stops: map[string]Stop{}, services: map[string]service{}, exceptions: map[string]map[string]bool{}, trips: map[string]trip{}, times: map[string][]stopTime{}, zone: zone}
	agencyOK := false
	err = table(files, "agency.txt", []string{"agency_id", "agency_timezone"}, func(r row) error {
		if r.get("agency_id") == "50066" && r.get("agency_timezone") == "America/Chicago" {
			agencyOK = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !agencyOK {
		return nil, errors.New("unsupported GTFS agency/timezone")
	}
	routeOK := false
	err = table(files, "routes.txt", []string{"route_id", "agency_id", "route_type"}, func(r row) error {
		if r.get("route_id") == "Blue" && r.get("agency_id") == "50066" && r.get("route_type") == "1" {
			routeOK = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !routeOK {
		return nil, errors.New("CTA Blue Line is unavailable")
	}
	err = table(files, "stops.txt", []string{"stop_id", "stop_name", "stop_lat", "stop_lon"}, func(r row) error {
		lat, e1 := strconv.ParseFloat(r.get("stop_lat"), 64)
		lon, e2 := strconv.ParseFloat(r.get("stop_lon"), 64)
		if e1 != nil || e2 != nil || math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
			return errors.New("invalid GTFS stop")
		}
		id := r.get("stop_id")
		if id == "" || f.stops[id].ID != "" {
			return errors.New("duplicate or empty GTFS stop")
		}
		f.stops[id] = Stop{id, r.get("stop_name"), r.get("parent_station"), lat, lon}
		if len(f.stops) > 40_000 {
			return errors.New("GTFS stop limit exceeded")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = table(files, "calendar.txt", []string{"service_id", "start_date", "end_date", "sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}, func(r row) error {
		s := service{start: r.get("start_date"), end: r.get("end_date")}
		if !date(s.start) || !date(s.end) || s.start > s.end {
			return errors.New("invalid GTFS service interval")
		}
		for i, d := range []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"} {
			v := r.get(d)
			if v != "0" && v != "1" {
				return errors.New("invalid GTFS weekday")
			}
			s.weekdays[i] = v == "1"
		}
		id := r.get("service_id")
		if _, ok := f.services[id]; id == "" || ok {
			return errors.New("duplicate or empty GTFS service")
		}
		f.services[id] = s
		if len(f.services) > 10_000 {
			return errors.New("GTFS service limit exceeded")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = table(files, "calendar_dates.txt", []string{"service_id", "date", "exception_type"}, func(r row) error {
		id, day, kind := r.get("service_id"), r.get("date"), r.get("exception_type")
		if id == "" || !date(day) || (kind != "1" && kind != "2") {
			return errors.New("invalid GTFS service exception")
		}
		if f.exceptions[id] == nil {
			f.exceptions[id] = map[string]bool{}
		}
		if _, ok := f.exceptions[id][day]; ok {
			return errors.New("conflicting GTFS exception")
		}
		f.exceptions[id][day] = kind == "1"
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = table(files, "trips.txt", []string{"route_id", "service_id", "trip_id"}, func(r row) error {
		if r.get("route_id") != "Blue" {
			return nil
		}
		id := r.get("trip_id")
		if _, ok := f.trips[id]; id == "" || ok {
			return errors.New("duplicate or empty GTFS trip")
		}
		serviceID := r.get("service_id")
		if _, ok := f.services[serviceID]; !ok && f.exceptions[serviceID] == nil {
			return errors.New("GTFS trip has no calendar")
		}
		f.trips[id] = trip{service: serviceID}
		if len(f.trips) > 10_000 {
			return errors.New("GTFS selected trip limit exceeded")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := files["frequencies.txt"]; ok {
		err = table(files, "frequencies.txt", []string{"trip_id"}, func(r row) error {
			id := r.get("trip_id")
			if t, ok := f.trips[id]; ok {
				t.frequency = true
				f.trips[id] = t
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	err = table(files, "stop_times.txt", []string{"trip_id", "stop_id", "stop_sequence", "arrival_time", "departure_time"}, func(r row) error {
		id := r.get("trip_id")
		if _, ok := f.trips[id]; !ok {
			return nil
		}
		arrival, e1 := clock(r.get("arrival_time"))
		departure, e2 := clock(r.get("departure_time"))
		seq, e3 := strconv.Atoi(r.get("stop_sequence"))
		if e1 != nil || e2 != nil || e3 != nil || seq < 0 || departure < arrival || f.stops[r.get("stop_id")].ID == "" {
			return errors.New("invalid selected GTFS stop time")
		}
		if len(f.times[id]) >= 100 {
			return errors.New("GTFS trip exceeds stop limit")
		}
		pickup, dropoff := r.get("pickup_type"), r.get("drop_off_type")
		f.times[id] = append(f.times[id], stopTime{r.get("stop_id"), r.get("stop_headsign"), seq, arrival, departure, pickup == "" || pickup == "0", dropoff == "" || dropoff == "0"})
		if f.platforms == nil {
			f.platforms = map[string]bool{}
		}
		f.platforms[r.get("stop_id")] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	for id, times := range f.times {
		sort.Slice(times, func(i, j int) bool { return times[i].sequence < times[j].sequence })
		for i := 1; i < len(times); i++ {
			if times[i].sequence == times[i-1].sequence || times[i].arrival < times[i-1].departure {
				return nil, errors.New("nonmonotonic selected GTFS trip")
			}
		}
		f.times[id] = times
	}
	if len(f.trips) == 0 || len(f.times) == 0 {
		return nil, errors.New("no supported GTFS trips")
	}
	return f, nil
}

func (f *Feed) active(id string, day time.Time) bool {
	key := day.Format("20060102")
	if value, ok := f.exceptions[id][key]; ok {
		return value
	}
	s, ok := f.services[id]
	return ok && s.start <= key && key <= s.end && s.weekdays[day.Weekday()]
}

func (f *Feed) Connect(r Request, now time.Time) Connection {
	if r.Mode != "walk" || r.Transfers != 0 {
		return unavailable("unsupported", "only_one_walk_mode_rail_ride_supported", &f.Source)
	}
	if r.DepartureAt.IsZero() || r.DepartureAt.Before(now) || r.DepartureAt.After(now.Add(7*24*time.Hour)) {
		return unavailable("unavailable", "departure_outside_supported_window", &f.Source)
	}
	if f.Source.RetrievedAt.After(now.Add(5*time.Minute)) || now.Sub(f.Source.RetrievedAt) > MaxFeedAge {
		return unavailable("stale", "schedule_source_stale", &f.Source)
	}
	board, ok := f.stops[r.BoardStopID]
	alight, ok2 := f.stops[r.AlightStopID]
	if !ok || !ok2 || board.ID == alight.ID || board.ParentID == "" || alight.ParentID == "" || !f.platforms[board.ID] || !f.platforms[alight.ID] || f.stops[board.ParentID].ID == "" || f.stops[alight.ParentID].ID == "" {
		return unavailable("unavailable", "selected_platform_unavailable", &f.Source)
	}
	result := unavailable("no_service", "no_supported_scheduled_connection", &f.Source)
	result.Board, result.Alight = &board, &alight
	local := r.DepartureAt.In(f.zone)
	// Include the previous service date for GTFS times beyond 24:00. As specified
	// by GTFS, clock seconds start at local noon minus twelve elapsed hours.
	for delta := -1; delta <= 1; delta++ {
		day := time.Date(local.Year(), local.Month(), local.Day()+delta, 12, 0, 0, 0, f.zone)
		start := day.Add(-12 * time.Hour)
		for id, t := range f.trips {
			if t.frequency || !f.active(t.service, day) {
				continue
			}
			for i, from := range f.times[id] {
				if from.stop != board.ID || !from.pickup {
					continue
				}
				dep := start.Add(time.Duration(from.departure) * time.Second)
				if dep.Before(r.DepartureAt) || dep.After(r.DepartureAt.Add(6*time.Hour)) {
					continue
				}
				for _, to := range f.times[id][i+1:] {
					if to.stop != alight.ID || !to.dropoff {
						continue
					}
					arrival := start.Add(time.Duration(to.arrival) * time.Second)
					if !arrival.After(dep) || arrival.Sub(dep) > 3*time.Hour {
						continue
					}
					if result.DepartureAt != nil && (dep.After(*result.DepartureAt) || (dep.Equal(*result.DepartureAt) && id >= result.TripID)) {
						continue
					}
					dep, arrival = dep.UTC(), arrival.UTC()
					result = Connection{State: "scheduled", Scheduled: true, Source: &f.Source, Board: &board, Alight: &alight, TripID: id, RouteID: "Blue", Headsign: from.headsign, DepartureAt: &dep, ArrivalAt: &arrival, Issues: []string{"scheduled_not_live", "check_current_service_alerts", "walking_directions_require_separate_review"}}
				}
			}
		}
	}
	return result
}

// Client fetches only the official CTA endpoint, at most once per fresh day.
// A failed refresh never turns an old timetable into a current connection.
type Client struct {
	HTTP        *http.Client
	mu          sync.Mutex
	feed        *Feed
	attemptedAt time.Time
	lastErr     error
	// refreshing is closed when the refresh in progress finishes; nil when none is running.
	refreshing chan struct{}
}

// load returns the current feed. One refresh runs at a time, detached from any request, so a
// caller that gives up neither cancels the download for everyone else nor starts another one.
func (c *Client) load(ctx context.Context, now time.Time) (*Feed, error) {
	c.mu.Lock()
	if c.feed != nil && now.Sub(c.feed.Source.RetrievedAt) <= MaxFeedAge && now.After(c.feed.Source.RetrievedAt.Add(-5*time.Minute)) {
		defer c.mu.Unlock()
		return c.feed, nil
	}
	if c.lastErr != nil && now.Sub(c.attemptedAt) < time.Minute {
		defer c.mu.Unlock()
		return c.feed, c.lastErr
	}
	done := c.refreshing
	if done == nil {
		done = make(chan struct{})
		c.refreshing = done
		go c.refresh(now, done)
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.feed, c.lastErr
}

func (c *Client) refresh(now time.Time, done chan struct{}) {
	feed, err := c.fetch(now)
	c.mu.Lock()
	if err == nil {
		c.feed = feed
	}
	c.attemptedAt, c.lastErr = now, err
	c.refreshing = nil
	c.mu.Unlock()
	close(done)
}

// fetch downloads and parses the feed, bounded by the client timeout rather than a caller's context.
func (c *Client) fetch(now time.Time) (*Feed, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, FeedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Byl-ScheduledTransit/1")
	client := http.Client{Timeout: 45 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	if client.Timeout <= 0 || client.Timeout > 45*time.Second {
		client.Timeout = 45 * time.Second
	}
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || r.URL.Scheme != "https" || r.URL.Hostname() != "www.transitchicago.com" || r.URL.User != nil || (r.URL.Port() != "" && r.URL.Port() != "443") {
			return errors.New("unsupported CTA redirect")
		}
		if previousRedirect != nil {
			return previousRedirect(r, via)
		}
		return nil
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("CTA timetable provider unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxArchive+1))
	if err != nil {
		return nil, err
	}
	return ParseCTA(data, now)
}

func (c *Client) Connect(ctx context.Context, r Request, now time.Time) Connection {
	if r.Mode != "walk" || r.Transfers != 0 {
		return unavailable("unsupported", "only_one_walk_mode_rail_ride_supported", nil)
	}
	f, err := c.load(ctx, now)
	if err != nil {
		if f != nil {
			return unavailable("stale", "schedule_refresh_unavailable", &f.Source)
		}
		return unavailable("provider_unavailable", "schedule_source_unavailable", nil)
	}
	return f.Connect(r, now)
}
