package jobs

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"invisible-museum/backend/internal/repository"
)

func testdbPool(t *testing.T, s *repository.Store) *pgxpool.Pool {
	t.Helper()
	return s.Pool()
}
