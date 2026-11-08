package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func TestMuseumCatalogueCoordinateMatchesInspectedBuilding(t *testing.T) {
	pool := Open(t)
	var lat, lng float64
	if err := pool.QueryRow(context.Background(), `SELECT latitude,longitude FROM places WHERE id='00000000-0000-4000-8000-00000000b0b4'`).Scan(&lat, &lng); err != nil {
		t.Fatal(err)
	}
	if math.Abs(lat-41.7923663) > 0.0000001 || math.Abs(lng-(-88.0045442)) > 0.0000001 {
		t.Fatalf("museum catalogue coordinate %.7f/%.7f differs from inspected building", lat, lng)
	}
}

func museumMigrationHashes(t *testing.T, pool *pgxpool.Pool, normalizeCorrection bool) map[string]string {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename<>'goose_db_version' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	hashes := map[string]string{}
	for _, name := range names {
		value := "to_jsonb(t)"
		if normalizeCorrection && name == "places" {
			value = `CASE WHEN id='00000000-0000-4000-8000-00000000b0b4' THEN to_jsonb(t)-ARRAY['latitude','longitude','updated_at'] ELSE to_jsonb(t) END`
		}
		if normalizeCorrection && name == "tour_revision_state" {
			value = `CASE WHEN tour_id IN (SELECT tour_id FROM tour_stops WHERE place_id='00000000-0000-4000-8000-00000000b0b4') THEN to_jsonb(t)-ARRAY['draft_version','route_version','release_requested_version'] ELSE to_jsonb(t) END`
		}
		query := fmt.Sprintf(`SELECT md5(coalesce(string_agg(h,'' ORDER BY h),'')) FROM (SELECT md5((%s)::text) h FROM %s t) rows`, value, pgx.Identifier{name}.Sanitize())
		var hash string
		if err := pool.QueryRow(ctx, query).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		hashes[name] = hash
	}
	return hashes
}

func TestMuseumForwardCorrectionPreservesRowsAndCuratorCoordinates(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(fmt.Sprintf("curator_edited_%t", edited), func(t *testing.T) {
			ctx := context.Background()
			pool := OpenVersion(t, 21)
			if edited {
				if _, err := pool.Exec(ctx, `UPDATE places SET latitude=41.7925,longitude=-88.0045 WHERE id='00000000-0000-4000-8000-00000000b0b4'`); err != nil {
					t.Fatal(err)
				}
			}
			// Retain a separately reviewed synthetic arrival, including its metadata.
			var actor, proposal string
			if err := pool.QueryRow(ctx, `INSERT INTO accounts(provider,provider_subject,role) VALUES('dev','museum-migration-fixture','curator') RETURNING id`).Scan(&actor); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `INSERT INTO place_arrival_proposals(place_id,base_version,point,status,created_by,reviewed_by,review_note,reviewed_at) VALUES('00000000-0000-4000-8000-00000000b0b4',0,'{"name":"Synthetic reviewed arrival","kind":"viewpoint","latitude":41.7926,"longitude":-88.0048,"access":[]}', 'approved',$1,$1,'Migration fixture only; no field claim',now()) RETURNING id`, actor).Scan(&proposal); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO place_arrival_current(place_id,proposal_id,version) VALUES('00000000-0000-4000-8000-00000000b0b4',$1,1)`, proposal); err != nil {
				t.Fatal(err)
			}
			before := museumMigrationHashes(t, pool, !edited)
			var draft, route int64
			var oldInput string
			if err := pool.QueryRow(ctx, `SELECT draft_version,route_version,byl_route_input_hash(tour_id,'walk') FROM tour_revision_state WHERE tour_id=$1`, SeedTourID).Scan(&draft, &route, &oldInput); err != nil {
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
			if err := goose.UpTo(db, MigrationsDir(), 22); err != nil {
				t.Fatal(err)
			}
			after := museumMigrationHashes(t, pool, !edited)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("migration changed rows outside its guarded correction", before, after)
			}
			var lat, lng float64
			if err := pool.QueryRow(ctx, `SELECT latitude,longitude FROM places WHERE id='00000000-0000-4000-8000-00000000b0b4'`).Scan(&lat, &lng); err != nil {
				t.Fatal(err)
			}
			wantLat, wantLng := 41.7923663, -88.0045442
			if edited {
				wantLat, wantLng = 41.7925, -88.0045
			}
			if lat != wantLat || lng != wantLng {
				t.Fatal("coordinate guard", lat, lng)
			}
			var newDraft, newRoute int64
			var requested *int64
			var newInput string
			if err := pool.QueryRow(ctx, `SELECT draft_version,route_version,release_requested_version,byl_route_input_hash(tour_id,'walk') FROM tour_revision_state WHERE tour_id=$1`, SeedTourID).Scan(&newDraft, &newRoute, &requested, &newInput); err != nil {
				t.Fatal(err)
			}
			if !edited && (newDraft != draft+1 || newRoute != route+1 || requested != nil || newInput == oldInput) {
				t.Fatal("correction did not fence stale review and routing inputs")
			}
			if err := goose.Up(db, MigrationsDir()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, museumMigrationHashes(t, pool, !edited)) {
				t.Fatal("migration replay changed content")
			}
			if err := goose.Down(db, MigrationsDir()); err == nil {
				t.Fatal("automatic down restored the known wrong pin")
			}
		})
	}
}
