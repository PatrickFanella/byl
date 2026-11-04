package repository

import (
	"encoding/json"
	"errors"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestCandidateLifecycle(t *testing.T) {
	s, ctx := newStore(t)
	reviewer, _ := s.UpsertAccount(ctx, models.AuthProviderDev, "r1", "r1@example.com", "", true)
	place := testdb.SeedPlaceStationID
	c, err := s.CreateCandidate(ctx, models.Candidate{
		Kind: models.CandidateKindMoment, Origin: models.CandidateOriginImport,
		SourceName: ptr("wikipedia"), ExternalID: ptr("Q123"), TargetPlaceID: &place,
		Payload: json.RawMessage(`{"title":"A thing happened"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != models.CandidateStatusPending {
		t.Fatalf("candidate: %+v", c)
	}
	_, err = s.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindMoment, Origin: models.CandidateOriginImport, SourceName: ptr("wikipedia"), ExternalID: ptr("Q123"), Payload: json.RawMessage(`{}`)})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate source/external id should conflict, got %v", err)
	}
	pending, _, err := s.ListCandidates(ctx, CandidateFilter{Status: models.CandidateStatusPending})
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending list: %v %d", err, len(pending))
	}
	entity := testdb.SeedMomentPlatformID
	reviewed, err := s.ReviewCandidate(ctx, c.ID, reviewer.ID, models.CandidateStatusApproved, "looks right", &entity)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != models.CandidateStatusApproved || reviewed.ApprovedEntityID == nil || *reviewed.ReviewerAccountID != reviewer.ID || reviewed.ReviewedAt == nil {
		t.Fatalf("reviewed: %+v", reviewed)
	}
	if _, err := s.ReviewCandidate(ctx, c.ID, reviewer.ID, models.CandidateStatusRejected, "", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("re-reviewing must conflict, got %v", err)
	}
	pending, _, _ = s.ListCandidates(ctx, CandidateFilter{Status: models.CandidateStatusPending})
	if len(pending) != 0 {
		t.Fatal("approved candidate should leave the pending list")
	}
}
