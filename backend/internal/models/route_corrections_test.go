package models

import (
	"strings"
	"testing"
	"time"
)

func TestRouteCorrectionRequiresBoundedDecisionAndFutureReassessment(t *testing.T) {
	now := time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)
	future := &Date{now.AddDate(0, 0, 1)}
	past := &Date{now.AddDate(0, 0, -1)}
	valid := ApproveRouteCorrectionRequest{Version: strings.Repeat("a", 64), InvestigationID: "investigation", Note: "Independently reviewed crossing"}
	for _, tc := range []struct {
		name   string
		mutate func(*ApproveRouteCorrectionRequest)
		want   bool
	}{
		{"permanent", func(*ApproveRouteCorrectionRequest) {}, true},
		{"temporary", func(r *ApproveRouteCorrectionRequest) { r.Temporary = true; r.ReviewOn = future }, true},
		{"no review date", func(r *ApproveRouteCorrectionRequest) { r.Temporary = true }, false},
		{"past review date", func(r *ApproveRouteCorrectionRequest) { r.Temporary = true; r.ReviewOn = past }, false},
		{"empty rationale", func(r *ApproveRouteCorrectionRequest) { r.Note = " " }, false},
		{"stale token shape", func(r *ApproveRouteCorrectionRequest) { r.Version = "" }, false},
		{"permanent with expiry", func(r *ApproveRouteCorrectionRequest) { r.ReviewOn = future }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.mutate(&in)
			if got := len(in.Validate(now)) == 0; got != tc.want {
				t.Fatalf("valid=%v fields=%+v", got, in.Validate(now))
			}
		})
	}
}
