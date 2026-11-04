package jobs

import (
	"context"
	"fmt"

	"invisible-museum/backend/internal/models"
)

// draftCandidate is implemented in the next task; until then the job fails without retry.
func (w *Worker) draftCandidate(ctx context.Context, p models.DraftCandidatePayload) error {
	return fmt.Errorf("%w: drafting is not implemented yet", ErrNoRetry)
}
