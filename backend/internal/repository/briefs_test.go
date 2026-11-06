package repository

import (
	"errors"
	"fmt"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestRouteBriefConcurrentRetriesAndVersions(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "briefs", "briefs@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	in := models.RouteBriefInput{Topic: "Labor", Area: "Chicago"}
	type result struct {
		brief   *models.RouteBrief
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 4)
	for range 4 {
		go func() {
			<-start
			b, created, err := s.CreateRouteBrief(ctx, a.ID, in, "one-request")
			results <- result{b, created, err}
		}()
	}
	close(start)
	created := 0
	var first *models.RouteBrief
	for range 4 {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.created {
			created++
		}
		if first != nil && first.ID != r.brief.ID {
			t.Fatal("retry created a duplicate")
		}
		first = r.brief
	}
	if created != 1 {
		t.Fatalf("created %d times", created)
	}
	edit := in
	edit.Notes = "Curator's work"
	updated, err := s.UpdateRouteBrief(ctx, first.ID, a.ID, models.UpdateRouteBriefRequest{Version: 1, Submitted: edit})
	if err != nil {
		t.Fatal(err)
	}
	retry, wasCreated, err := s.CreateRouteBrief(ctx, a.ID, in, "one-request")
	if err != nil || wasCreated || retry.Version != updated.Version || retry.Submitted.Notes != edit.Notes {
		t.Fatalf("retry discarded edits: %+v %v", retry, err)
	}
	if _, _, err := s.CreateRouteBrief(ctx, a.ID, edit, "one-request"); !errors.Is(err, ErrConflict) {
		t.Fatalf("key reuse: %v", err)
	}
	// Two writes based on the same saved version must have only one winner.
	errorsCh := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := s.UpdateRouteBrief(ctx, first.ID, a.ID, models.UpdateRouteBriefRequest{Version: updated.Version, Submitted: edit})
			errorsCh <- err
		}()
	}
	winners, conflicts := 0, 0
	for range 2 {
		err := <-errorsCh
		if err == nil {
			winners++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("lost-update protection: %d winners, %d conflicts", winners, conflicts)
	}
}

func TestRouteBriefQuotasIncludeConcurrentSubmissions(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "quota", "quota@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	in := models.RouteBriefInput{Topic: "Labor", Area: "Chicago"}
	for i := 0; i < 9; i++ {
		if _, _, err := s.CreateRouteBrief(ctx, a.ID, in, fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	errorsCh := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func(key string) { _, _, err := s.CreateRouteBrief(ctx, a.ID, in, key); errorsCh <- err }(fmt.Sprintf("concurrent-%d", i))
	}
	winners, limited := 0, 0
	for range 3 {
		err := <-errorsCh
		if err == nil {
			winners++
		} else if errors.Is(err, ErrSubmissionLimit) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || limited != 2 {
		t.Fatalf("quota race: %d saved, %d limited", winners, limited)
	}
	if _, created, err := s.CreateRouteBrief(ctx, a.ID, in, "0"); err != nil || created {
		t.Fatalf("retry at quota: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE route_briefs SET created_at = now() - interval '2 days' WHERE owner_account_id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	for i := 10; i < 20; i++ {
		if _, _, err := s.CreateRouteBrief(ctx, a.ID, in, fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.pool.Exec(ctx, `UPDATE route_briefs SET created_at = now() - interval '2 days' WHERE owner_account_id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateRouteBrief(ctx, a.ID, in, "active-limit"); !errors.Is(err, ErrSubmissionLimit) {
		t.Fatalf("active quota: %v", err)
	}
}
