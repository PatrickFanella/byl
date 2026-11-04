package testdb

import (
	"context"
	"testing"
)

func TestOpenRunsMigrations(t *testing.T) {
	pool := Open(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"seed tour", `SELECT count(*) FROM tours WHERE slug = 'downtown-memory-loop'`, 1},
		{"seed stops", `SELECT count(*) FROM tour_stops`, 4},
		{"route jobs", `SELECT count(*) FROM jobs WHERE kind = 'compute_route'`, 2},
		{"places have primary moments", `SELECT count(*) FROM places WHERE primary_moment_id IS NOT NULL`, 4},
		{"moment sources", `SELECT count(*) FROM moment_sources`, 0},
	}
	for _, tc := range cases {
		var got int
		if err := pool.QueryRow(ctx, tc.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if tc.name == "moment sources" {
			if got == 0 {
				t.Fatalf("%s: expected at least one source row", tc.name)
			}
			continue
		}
		if got != tc.want {
			t.Fatalf("%s: got %d want %d", tc.name, got, tc.want)
		}
	}
}

func TestOpenIsolatesSchemas(t *testing.T) {
	a := Open(t)
	b := Open(t)
	ctx := context.Background()
	var sa, sb string
	if err := a.QueryRow(ctx, `SELECT current_schema()`).Scan(&sa); err != nil {
		t.Fatal(err)
	}
	if err := b.QueryRow(ctx, `SELECT current_schema()`).Scan(&sb); err != nil {
		t.Fatal(err)
	}
	if sa == sb {
		t.Fatalf("expected distinct schemas, both %q", sa)
	}
}
