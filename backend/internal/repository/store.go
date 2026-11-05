// Package repository owns every SQL statement the API runs.
package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"invisible-museum/backend/internal/geo"
)

var (
	// ErrNotFound means the row does not exist or is not visible to the caller.
	ErrNotFound = errors.New("not found")
	// ErrConflict means a uniqueness or state rule was violated.
	ErrConflict = errors.New("conflict")
	// ErrInvalidReference means a referenced row does not exist.
	ErrInvalidReference = errors.New("invalid reference")
)

type database interface {
	dbtx
	Begin(context.Context) (pgx.Tx, error)
	Ping(context.Context) error
}

type transactionDB struct{ pgx.Tx }

func (t transactionDB) Ping(ctx context.Context) error { return t.Conn().Ping(ctx) }

// Store runs queries against a pool or a transaction scoped to one operation.
type Store struct {
	pool    database
	rawPool *pgxpool.Pool
}

// New wraps a pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, rawPool: pool} }

// Pool exposes the underlying pool for tests and tooling.
func (s *Store) Pool() *pgxpool.Pool { return s.rawPool }

// Transaction commits related repository operations together, rolling back all of them on error.
func (s *Store) Transaction(ctx context.Context, fn func(*Store) error) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return fn(&Store{pool: transactionDB{tx}, rawPool: s.rawPool})
	})
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Store) withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

// mapErr translates pgx errors into the package's sentinel errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.ConstraintName)
		case "23503":
			return fmt.Errorf("%w: %s", ErrInvalidReference, pgErr.ConstraintName)
		case "22P02":
			return fmt.Errorf("%w: malformed identifier", ErrInvalidReference)
		}
	}
	return err
}

// uniqueSlug returns base, or base-2, base-3, ... until no row in table matches within the scope.
func uniqueSlug(ctx context.Context, q dbtx, table, scopeCol, scopeVal, base string) (string, error) {
	if base == "" {
		base = "item"
	}
	for i := 1; i < 1000; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		var exists bool
		var err error
		if scopeCol == "" {
			err = q.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE slug = $1)`, table), candidate).Scan(&exists)
		} else {
			err = q.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE slug = $1 AND %s = $2)`, table, scopeCol), candidate, scopeVal).Scan(&exists)
		}
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find a free slug for %q", base)
}

func slugFor(explicit, name string) string {
	if s := geo.Slugify(explicit); s != "" {
		return s
	}
	return geo.Slugify(name)
}

func nullable(s *string) any {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return strings.TrimSpace(*s)
}

func clamp(limit int) int {
	switch {
	case limit <= 0:
		return 50
	case limit > 200:
		return 200
	}
	return limit
}

// Haversine distance in metres from ($lat,$lng) to the row's coordinates.
func distanceExpr(latArg, lngArg int, latCol, lngCol string) string {
	return fmt.Sprintf(`2 * 6371000 * asin(sqrt(power(sin(radians(%[3]s - $%[1]d) / 2), 2) + cos(radians($%[1]d)) * cos(radians(%[3]s)) * power(sin(radians(%[4]s - $%[2]d) / 2), 2)))`, latArg, lngArg, latCol, lngCol)
}
