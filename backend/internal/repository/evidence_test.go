package repository

import (
	"errors"
	"fmt"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestEvidenceQuotaConcurrentReplaysAndOriginalVersion(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "evidence-quota", "quota@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateRouteBrief(ctx, a.ID, models.RouteBriefInput{Topic: "History", Area: "Chicago"}, "")
	if err != nil {
		t.Fatal(err)
	}
	in := models.EvidenceRequest{Version: 1, URL: "https://example.org/history", SourceKind: "page"}
	key := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var first *models.SourceEvidence
	for i := 0; i < 19; i++ {
		e, err := s.QueueSourceEvidence(ctx, b.ID, in, "page=1", key, fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = e
		}
	}
	results := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func(n int) {
			_, err := s.QueueSourceEvidence(ctx, b.ID, in, "page=2", key, fmt.Sprintf("concurrent-%d", n))
			results <- err
		}(i)
	}
	winners, limited := 0, 0
	for range 3 {
		err := <-results
		if err == nil {
			winners++
		} else if errors.Is(err, ErrSubmissionLimit) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || limited != 2 {
		t.Fatalf("quota race: %d winners %d limited", winners, limited)
	}
	b.Submitted.Notes = "Curator's next scope"
	if _, err := s.UpdateRouteBrief(ctx, b.ID, "", models.UpdateRouteBriefRequest{Version: 1, Submitted: b.Submitted}); err != nil {
		t.Fatal(err)
	}
	again, err := s.QueueSourceEvidence(ctx, b.ID, in, "page=1", key, "0")
	if err != nil || again.ID != first.ID || again.BriefVersion != 1 {
		t.Fatalf("retry at quota/after edit changed evidence: %+v %v", again, err)
	}
	if _, err := s.QueueSourceEvidence(ctx, b.ID, in, "different-ref", key, "0"); !errors.Is(err, ErrConflict) {
		t.Fatalf("reference key conflict: %v", err)
	}
}
