package repository

import (
	"context"
	"strings"
	"time"

	"invisible-museum/backend/internal/models"
)

const accountCols = `id, provider, provider_subject, email, display_name, role, last_sign_in_at, created_at`

func scanAccount(row interface{ Scan(...any) error }) (models.Account, error) {
	var a models.Account
	err := row.Scan(&a.ID, &a.Provider, &a.ProviderSubject, &a.Email, &a.DisplayName, &a.Role, &a.LastSignInAt, &a.CreatedAt)
	return a, err
}

// UpsertAccount creates or refreshes the account for a verified identity. A bootstrap admin gets the
// admin role only when the account is created, so a later demotion survives the next sign-in.
func (s *Store) UpsertAccount(ctx context.Context, provider models.AuthProvider, subject, email, displayName string, bootstrapAdmin bool) (*models.Account, error) {
	role := models.AccountRoleVisitor
	if bootstrapAdmin {
		role = models.AccountRoleAdmin
	}
	email = strings.ToLower(strings.TrimSpace(email))
	a, err := scanAccount(s.pool.QueryRow(ctx, `INSERT INTO accounts (provider, provider_subject, email, display_name, role, last_sign_in_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (provider, provider_subject) DO UPDATE SET
			email = CASE WHEN EXCLUDED.email = '' THEN accounts.email ELSE EXCLUDED.email END,
			display_name = CASE WHEN EXCLUDED.display_name = '' THEN accounts.display_name ELSE EXCLUDED.display_name END,
			last_sign_in_at = now(), updated_at = now()
		RETURNING `+accountCols, provider, subject, email, strings.TrimSpace(displayName), role))
	if err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

// GetAccount returns one account.
func (s *Store) GetAccount(ctx context.Context, id string) (*models.Account, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	a, err := scanAccount(s.pool.QueryRow(ctx, `SELECT `+accountCols+` FROM accounts WHERE id = $1`, id))
	if err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

// ListAccounts returns accounts newest first.
func (s *Store) ListAccounts(ctx context.Context, limit, offset int) ([]models.Account, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+accountCols+` FROM accounts ORDER BY created_at DESC LIMIT $1 OFFSET $2`, clamp(limit), offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetRole changes an account's role.
func (s *Store) SetRole(ctx context.Context, id string, role models.AccountRole) error {
	tag, err := s.pool.Exec(ctx, `UPDATE accounts SET role = $2, updated_at = now() WHERE id = $1`, id, role)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSession stores a hashed session token.
func (s *Store) CreateSession(ctx context.Context, accountID, tokenHash string, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_hash, account_id, expires_at) VALUES ($1, $2, $3)`, tokenHash, accountID, expires)
	return mapErr(err)
}

// AccountByToken resolves a live session and records its use. Expired or unknown tokens are ErrNotFound.
func (s *Store) AccountByToken(ctx context.Context, tokenHash string, now time.Time) (*models.Account, error) {
	a, err := scanAccount(s.pool.QueryRow(ctx, `WITH touched AS (
			UPDATE sessions SET last_used_at = $2 WHERE token_hash = $1 AND expires_at > $2 RETURNING account_id
		) SELECT `+accountCols+` FROM accounts WHERE id = (SELECT account_id FROM touched)`, tokenHash, now))
	if err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

// RevokeSession deletes a session.
func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return mapErr(err)
}

// Save bookmarks a tour for an account.
func (s *Store) Save(ctx context.Context, accountID, tourID string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO saved_tours (account_id, tour_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, accountID, tourID)
	return mapErr(err)
}

// Unsave removes a bookmark.
func (s *Store) Unsave(ctx context.Context, accountID, tourID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM saved_tours WHERE account_id = $1 AND tour_id = $2`, accountID, tourID)
	return mapErr(err)
}

// ListSaved returns an account's bookmarked tours.
func (s *Store) ListSaved(ctx context.Context, accountID string) ([]models.TourSummary, error) {
	tours, _, err := s.ListTours(ctx, TourFilter{SavedByAccountID: accountID, IncludeUnpublished: true})
	if err != nil {
		return nil, err
	}
	out := []models.TourSummary{}
	for _, tour := range tours {
		working, err := s.GetTour(ctx, tour.ID, false)
		if err != nil {
			return nil, err
		}
		served, err := s.ServedTour(ctx, working, accountID)
		if err != nil {
			return nil, err
		}
		if served.Visibility == models.TourVisibilityPrivate && (served.OwnerAccountID == nil || *served.OwnerAccountID != accountID) {
			continue
		}
		out = append(out, served.TourSummary)
	}
	return out, nil
}
