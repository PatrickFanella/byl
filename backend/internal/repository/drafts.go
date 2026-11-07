package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"invisible-museum/backend/internal/models"
)

// CandidatePayloadHash is stable across JSONB formatting and captures editor state.
func CandidatePayloadHash(raw json.RawMessage) string {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	canonical, _ := json.Marshal(value)
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:])
}

// DraftEvidence accepts only retained inspected text; snippets and anecdotes are not evidence.
func (s *Store) DraftEvidence(ctx context.Context, ids []string) ([]models.SourceEvidence, error) {
	if len(ids) < 1 || len(ids) > 5 {
		return nil, fmt.Errorf("%w: choose 1–5 inspected evidence records", ErrInvalidReference)
	}
	seen := map[string]bool{}
	out := []models.SourceEvidence{}
	for _, id := range ids {
		if !isUUID(id) || seen[id] {
			return nil, ErrInvalidReference
		}
		seen[id] = true
		e, err := s.GetSourceEvidence(ctx, id)
		if err != nil {
			return nil, err
		}
		r := e.Result
		if r == nil {
			return nil, ErrInvalidReference
		}
		digest, digestErr := hex.DecodeString(r.ContentSHA256)
		if !r.Valid() || (r.Outcome != "inspected_page" && r.Outcome != "catalog_only") || len(r.Passages) == 0 || digestErr != nil || len(digest) != 32 || r.RetrievedAt.IsZero() {
			return nil, fmt.Errorf("%w: evidence must contain an inspected immutable passage snapshot", ErrInvalidReference)
		}
		refs := map[string]bool{}
		for _, p := range r.Passages {
			if p.Reference == "" || refs[p.Reference] || p.Text == "" {
				return nil, ErrInvalidReference
			}
			refs[p.Reference] = true
		}
		out = append(out, *e)
	}
	return out, nil
}

// SaveCandidateDraft fences both curator edits and approval/rejection races.
// Generated proposals never overwrite the editable title, body, dates or sources.
func (s *Store) SaveCandidateDraft(ctx context.Context, c *models.Candidate, proposal json.RawMessage) error {
	var payload map[string]json.RawMessage
	if json.Unmarshal(c.Payload, &payload) != nil || payload == nil {
		return ErrInvalidReference
	}
	var history []json.RawMessage
	if raw := payload["generated_drafts"]; len(raw) > 0 {
		if json.Unmarshal(raw, &history) != nil {
			return ErrInvalidReference
		}
	}
	if len(history) >= 5 {
		return fmt.Errorf("%w: review the existing five draft proposals before creating another candidate", ErrConflict)
	}
	history = append(history, proposal)
	payload["generated_drafts"], _ = json.Marshal(history)
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if len(raw) > 262144 {
		return fmt.Errorf("%w: draft proposal history exceeds its storage limit", ErrConflict)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE candidates SET payload=$3,origin='draft',updated_at=now() WHERE id=$1 AND status='pending' AND payload=$2::jsonb`, c.ID, c.Payload, raw)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: candidate changed while drafting; reload before retrying", ErrConflict)
	}
	return nil
}
