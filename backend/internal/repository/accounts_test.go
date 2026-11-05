package repository

import (
	"errors"
	"slices"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestUpsertAccountRolesAndIdempotency(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderGoogle, "sub-1", "Person@Example.com", "Person", false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Role != models.AccountRoleVisitor || a.Email != "person@example.com" {
		t.Fatalf("account: %+v", a)
	}
	admin, _ := s.UpsertAccount(ctx, models.AuthProviderGoogle, "sub-2", "boss@example.com", "Boss", true)
	if admin.Role != models.AccountRoleAdmin {
		t.Fatalf("bootstrap admin: %+v", admin)
	}
	if err := s.SetRole(ctx, a.ID, models.AccountRoleCurator); err != nil {
		t.Fatal(err)
	}
	again, _ := s.UpsertAccount(ctx, models.AuthProviderGoogle, "sub-1", "person@example.com", "Person Renamed", false)
	if again.ID != a.ID || again.Role != models.AccountRoleCurator || again.DisplayName != "Person Renamed" {
		t.Fatalf("re-upsert must keep id and role, update name: %+v", again)
	}
}

func TestSessionsExpireAndRevoke(t *testing.T) {
	s, ctx := newStore(t)
	a, _ := s.UpsertAccount(ctx, models.AuthProviderDev, "d1", "d1@example.com", "", false)
	now := time.Now()
	if err := s.CreateSession(ctx, a.ID, "hash-live", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, a.ID, "hash-dead", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.AccountByToken(ctx, "hash-live", now)
	if err != nil || got.ID != a.ID {
		t.Fatalf("live session: %v %+v", err, got)
	}
	if _, err := s.AccountByToken(ctx, "hash-dead", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session should be ErrNotFound, got %v", err)
	}
	var lastUsed *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT last_used_at FROM sessions WHERE token_hash = 'hash-dead'`).Scan(&lastUsed); err != nil {
		t.Fatal(err)
	}
	if lastUsed != nil {
		t.Fatal("expired session must not be touched")
	}
	if err := s.pool.QueryRow(ctx, `SELECT last_used_at FROM sessions WHERE token_hash = 'hash-live'`).Scan(&lastUsed); err != nil || lastUsed == nil {
		t.Fatalf("live session should record last_used_at: %v", err)
	}
	if err := s.RevokeSession(ctx, "hash-live"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AccountByToken(ctx, "hash-live", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked session should be ErrNotFound, got %v", err)
	}
}

func TestSavedAndProgress(t *testing.T) {
	s, ctx := newStore(t)
	a, _ := s.UpsertAccount(ctx, models.AuthProviderDev, "d2", "d2@example.com", "", false)
	if err := s.Save(ctx, a.ID, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, a.ID, testdb.SeedTourID); err != nil {
		t.Fatalf("saving twice must be idempotent: %v", err)
	}
	saved, err := s.ListSaved(ctx, a.ID)
	if err != nil || len(saved) != 1 || saved[0].Slug != "downtown-memory-loop" {
		t.Fatalf("saved: %v %+v", err, saved)
	}
	if err := s.Unsave(ctx, a.ID, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	saved, _ = s.ListSaved(ctx, a.ID)
	if len(saved) != 0 {
		t.Fatal("expected empty after unsave")
	}

	p, err := s.PutProgress(ctx, models.TourProgress{AccountID: a.ID, TourID: testdb.SeedTourID, Mode: models.TravelModeBike, VisitedPositions: []int{1, 2}}, false)
	if err != nil || len(p.VisitedPositions) != 2 || p.CompletedAt != nil {
		t.Fatalf("progress: %v %+v", err, p)
	}
	done := time.Now()
	p, err = s.PutProgress(ctx, models.TourProgress{AccountID: a.ID, TourID: testdb.SeedTourID, Mode: models.TravelModeBike, VisitedPositions: []int{1, 2, 3, 4}, CompletedAt: &done}, false)
	if err != nil || p.CompletedAt == nil || len(p.VisitedPositions) != 4 {
		t.Fatalf("progress update: %v %+v", err, p)
	}
	all, _ := s.ListProgress(ctx, a.ID)
	if len(all) != 1 {
		t.Fatalf("expected one progress row, got %d", len(all))
	}
	if _, err := s.GetProgress(ctx, a.ID, "00000000-0000-4000-8000-00000000ffff"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestConcurrentProgressUpdatesKeepEveryStop(t *testing.T) {
	s, ctx := newStore(t)
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "concurrent", "concurrent@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 4)
	for position := 1; position <= 4; position++ {
		go func(p int) {
			<-start
			_, err := s.PutProgress(ctx, models.TourProgress{AccountID: a.ID, TourID: testdb.SeedTourID, Mode: models.TravelModeWalk, VisitedPositions: []int{p}}, false)
			results <- err
		}(position)
	}
	close(start)
	for range 4 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	p, err := s.GetProgress(ctx, a.ID, testdb.SeedTourID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.VisitedPositions, []int{1, 2, 3, 4}) {
		t.Fatalf("concurrent updates lost stops: %v", p.VisitedPositions)
	}
}
