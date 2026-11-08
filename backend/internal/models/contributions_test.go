package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func contributionSummaryFixture() ContributionSummary {
	no := false
	zero := 0
	day := Date{ContributionDay(time.Now())}
	return ContributionSummary{SchemaVersion: 1, DetectorVersion: "uncalibrated-v1", Calibrated: &no, RevisionID: "00000000-0000-4000-8000-000000000001", SegmentID: "main", Mode: TravelModeWalk, Direction: "forward", ObservationPeriod: &day, QualifiedPassages: 1, SustainedDepartureRejoins: &zero}
}

func TestContributionSummaryAllowsOnlyBoundedUncalibratedDaySummaries(t *testing.T) {
	now := time.Now()
	valid := contributionSummaryFixture()
	if fields := valid.Validate(now); len(fields) > 0 {
		t.Fatalf("valid summary: %v", fields)
	}
	for _, change := range []func(*ContributionSummary){
		func(s *ContributionSummary) { s.Calibrated = nil }, func(s *ContributionSummary) { yes := true; s.Calibrated = &yes },
		func(s *ContributionSummary) { s.SchemaVersion = 2 }, func(s *ContributionSummary) { s.DetectorVersion = "future" },
		func(s *ContributionSummary) { s.SegmentID = strings.Repeat("s", 121) }, func(s *ContributionSummary) { s.Mode = "drive" },
		func(s *ContributionSummary) { s.Direction = "unknown" }, func(s *ContributionSummary) { s.QualifiedPassages = 2 },
		func(s *ContributionSummary) { two := 2; s.SustainedDepartureRejoins = &two },
		func(s *ContributionSummary) { s.SustainedDepartureRejoins = nil }, func(s *ContributionSummary) { s.ObservationPeriod = nil },
		func(s *ContributionSummary) {
			d := Date{ContributionDay(now).AddDate(0, 0, 1)}
			s.ObservationPeriod = &d
		},
		func(s *ContributionSummary) {
			d := Date{ContributionDay(now).AddDate(0, 0, -30)}
			s.ObservationPeriod = &d
		},
	} {
		in := valid
		change(&in)
		if len(in.Validate(now)) == 0 {
			t.Fatal("invalid summary accepted")
		}
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err = json.Unmarshal(raw, &shape); err != nil {
		t.Fatal(err)
	}
	if len(shape) != 10 {
		t.Fatalf("minimal summary fields=%d", len(shape))
	}
	for _, field := range []string{"coordinates", "timestamp", "account_id", "device_id", "trace", "trip_id"} {
		if _, ok := shape[field]; ok {
			t.Fatalf("unexpected %s", field)
		}
	}
}

func TestContributionConsentIsExplicitAndCalendarDaysRejectPreciseTimes(t *testing.T) {
	now := time.Now()
	day := Date{ContributionDay(now)}
	in := ContributionConsentRequest{Version: 1, Accepted: true, TourID: "tour", RevisionID: "revision", Mode: TravelModeWalk, RequestedOn: &day}
	if len(in.Validate(now)) > 0 {
		t.Fatal("explicit consent rejected")
	}
	in.Accepted = false
	if len(in.Validate(now)) == 0 {
		t.Fatal("navigation cannot imply contribution consent")
	}
	for _, raw := range []string{`"2026-11-08T00:00:00Z"`, `1794096000000`, `"2026-11-08 00:00"`} {
		var date Date
		if json.Unmarshal([]byte(raw), &date) == nil {
			t.Fatal("precise trip time accepted as observation day")
		}
	}
	if ValidContributionKey("") || ValidContributionKey("short") || ValidContributionKey(strings.Repeat("x", 129)) || ValidContributionKey(strings.Repeat("x", 16)+"\n") {
		t.Fatal("invalid replay key accepted")
	}
	if !ValidContributionKey(strings.Repeat("k", 32)) {
		t.Fatal("bounded replay key rejected")
	}
}
