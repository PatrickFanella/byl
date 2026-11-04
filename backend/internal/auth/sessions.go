package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

// Sessions issues and resolves opaque bearer tokens.
type Sessions struct {
	store *repository.Store
	ttl   time.Duration
}

// NewSessions builds a session manager.
func NewSessions(store *repository.Store, ttl time.Duration) *Sessions {
	return &Sessions{store: store, ttl: ttl}
}

// HashToken returns the stored form of a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Issue creates a new session token for the account.
func (s *Sessions) Issue(ctx context.Context, accountID string) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(s.ttl)
	if err := s.store.CreateSession(ctx, accountID, HashToken(token), expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Resolve returns the account for a live token.
func (s *Sessions) Resolve(ctx context.Context, token string) (*models.Account, error) {
	return s.store.AccountByToken(ctx, HashToken(token), time.Now())
}

// Revoke ends a session.
func (s *Sessions) Revoke(ctx context.Context, token string) error {
	return s.store.RevokeSession(ctx, HashToken(token))
}
