package repository

import (
	"context"
	"database/sql"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func TestRevisionMigrationPinsLegacyProgressWithoutRewritingSourceRows(t *testing.T) {
	pool := testdb.OpenVersion(t, 11)
	s := New(pool)
	ctx := context.Background()
	a, err := s.UpsertAccount(ctx, models.AuthProviderDev, "legacy-migration", "legacy@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	storyBefore, err := s.GetMoment(ctx, testdb.SeedMomentBurlingtonID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tour_progress(account_id,tour_id,mode,visited_positions,started_at) VALUES($1,$2,'bike',ARRAY[1,4,99],$3)`, a.ID, testdb.SeedTourID, time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	var before, places string
	if err := pool.QueryRow(ctx, `SELECT md5(to_jsonb(p)::text),(SELECT md5(string_agg(to_jsonb(x)::text,'' ORDER BY x.id)) FROM places x) FROM tour_progress p WHERE account_id=$1`, a.ID).Scan(&before, &places); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := goose.Up(db, testdb.MigrationsDir()); err != nil {
		t.Fatal(err)
	}
	var after, placesAfter string
	if err := pool.QueryRow(ctx, `SELECT md5(to_jsonb(p)::text),(SELECT md5(string_agg(to_jsonb(x)::text,'' ORDER BY x.id)) FROM places x) FROM tour_progress p WHERE account_id=$1`, a.ID).Scan(&after, &placesAfter); err != nil {
		t.Fatal(err)
	}
	if before != after || places != placesAfter {
		t.Fatal("migration rewrote legacy progress or place rows")
	}
	p, err := s.GetProgress(ctx, a.ID, testdb.SeedTourID)
	if err != nil || !slices.Equal(p.VisitedPositions, []int{1, 4}) || p.Mode != models.TravelModeBike || p.StartedAt.Year() != 2020 {
		t.Fatalf("migration mapping %+v %v", p, err)
	}
	r, err := s.GetTourRevision(ctx, testdb.SeedTourID, p.RevisionID)
	if err != nil || r.Number != 1 || len(r.Snapshot.Moments) == 0 {
		t.Fatalf("backfill %+v %v", r, err)
	}
	if !reflect.DeepEqual(publicProjection(t, storyBefore), publicProjection(t, r.Snapshot.Moments[storyBefore.ID])) {
		t.Fatal("backfilled snapshot lost derived public story metadata")
	}
	progress, err := s.GetRevisionProgress(ctx, a.ID, p.RevisionID)
	if err != nil || len(progress.VisitedStopIDs) != 2 || !slices.Contains(progress.VisitedStopIDs, r.Snapshot.Tour.Stops[0].ID) || !slices.Contains(progress.VisitedStopIDs, r.Snapshot.Tour.Stops[3].ID) {
		t.Fatalf("stable IDs %+v %v", progress, err)
	}
	if err := goose.Down(db, testdb.MigrationsDir()); err == nil {
		t.Fatal("automatic down removed retained revisions")
	}
}
