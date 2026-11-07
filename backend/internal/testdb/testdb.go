// Package testdb opens an isolated Postgres schema with all migrations applied
// for integration tests. Tests skip when TEST_DATABASE_URL is unset.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Open returns a pool bound to a fresh schema with migrations applied.
// The schema is dropped when the test finishes.
func Open(t *testing.T) *pgxpool.Pool {
	return OpenVersion(t, 0)
}

// OpenVersion prepares an older schema for forward-migration tests; zero applies all migrations.
func OpenVersion(t *testing.T, version int64) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	schema := "test_" + randomHex(6)
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatalf("testdb: connect: %v", err)
	}
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %q`, schema)); err != nil {
		admin.Close()
		t.Fatalf("testdb: create schema: %v", err)
	}

	scoped, err := withSearchPath(base, schema)
	if err != nil {
		t.Fatal(err)
	}

	if err := migrate(scoped, version); err != nil {
		_, _ = admin.Exec(ctx, fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema))
		admin.Close()
		t.Fatalf("testdb: migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, scoped)
	if err != nil {
		t.Fatalf("testdb: open scoped pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema))
		admin.Close()
	})
	return pool
}

// MigrationsDir returns the absolute path of backend/migrations.
func MigrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

func migrate(dsn string, version int64) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if version > 0 {
		return goose.UpTo(db, MigrationsDir(), version)
	}
	return goose.Up(db, MigrationsDir())
}

func withSearchPath(base, schema string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("testdb: parse TEST_DATABASE_URL: %w", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
