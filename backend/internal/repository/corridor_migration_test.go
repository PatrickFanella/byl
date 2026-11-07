package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
	"invisible-museum/backend/internal/testdb"
)

func TestCorridorMigrationRetainsExistingRowsAndRouteHashes(t *testing.T) {
	pool := testdb.OpenVersion(t, 13)
	ctx := context.Background()
	s := New(pool)
	readyFixtureRoutes(t, s, ctx, testdb.SeedTourID)
	rows, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename<>'goose_db_version' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	before := map[string]string{}
	for _, table := range tables {
		query := fmt.Sprintf(`SELECT md5(coalesce(string_agg(h,'' ORDER BY h),'')) FROM (SELECT md5(to_jsonb(t)::text) h FROM %s t) rows`, pgx.Identifier{table}.Sanitize())
		var hash string
		if err := pool.QueryRow(ctx, query).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		before[table] = hash
	}
	var oldHash string
	if err := pool.QueryRow(ctx, `SELECT byl_route_input_hash($1,'walk')`, testdb.SeedTourID).Scan(&oldHash); err != nil {
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
	for _, table := range tables {
		var after string
		query := fmt.Sprintf(`SELECT md5(coalesce(string_agg(h,'' ORDER BY h),'')) FROM (SELECT md5(to_jsonb(t)::text) h FROM %s t) rows`, pgx.Identifier{table}.Sanitize())
		if err := pool.QueryRow(ctx, query).Scan(&after); err != nil || after != before[table] {
			t.Fatalf("migration changed %s: %v", table, err)
		}
	}
	var hash string
	if err := pool.QueryRow(ctx, `SELECT byl_route_input_hash($1,'walk')`, testdb.SeedTourID).Scan(&hash); err != nil || hash != oldHash {
		t.Fatalf("legacy ready input hash changed %s %v", hash, err)
	}
	tour, err := s.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil || tour.Corridor != nil {
		t.Fatalf("migration invented corridor intent %+v %v", tour, err)
	}
	if err := goose.Down(db, testdb.MigrationsDir()); err == nil {
		t.Fatal("automatic down removed reviewed intent")
	}
}
