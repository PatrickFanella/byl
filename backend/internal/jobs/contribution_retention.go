package jobs

import (
	"context"
	"log/slog"
	"time"

	"invisible-museum/backend/internal/repository"
)

// RunContributionRetention remains active when collection or ordinary job workers
// are disabled. A standalone command provides the same cleanup during rollback.
func RunContributionRetention(ctx context.Context, store *repository.Store, log *slog.Logger) {
	prune := func() {
		work, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		count, err := store.PurgeRouteContributions(work, time.Now())
		if err != nil && ctx.Err() == nil {
			log.Error("route contribution retention failed")
			return
		}
		if count > 0 {
			log.Info("route contribution retention", "direct_rows_deleted", count)
		}
	}
	prune()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}
