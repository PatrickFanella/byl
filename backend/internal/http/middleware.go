package httpapi

import (
	"context"
	"net/http"
	"strings"

	"invisible-museum/backend/internal/models"
)

// SessionResolver turns a bearer token into an account.
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (*models.Account, error)
}

type ctxKey int

const accountKey ctxKey = iota

func withAccount(ctx context.Context, a *models.Account) context.Context {
	return context.WithValue(ctx, accountKey, a)
}

// accountFrom returns the signed-in account, or nil.
func accountFrom(ctx context.Context) *models.Account {
	a, _ := ctx.Value(accountKey).(*models.Account)
	return a
}

func bearerToken(r *http.Request) (string, bool) {
	raw := r.Header.Get("Authorization")
	scheme, token, ok := strings.Cut(raw, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// optionalAuth attaches the account when a valid token is present and passes anonymous requests through.
func (d Deps) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := bearerToken(r); ok && d.Sessions != nil {
			if a, err := d.Sessions.Resolve(r.Context(), token); err == nil && a != nil {
				r = r.WithContext(withAccount(r.Context(), a))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireAuth rejects requests without a valid session.
func (d Deps) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
			return
		}
		if d.Sessions == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sessions are not configured")
			return
		}
		a, err := d.Sessions.Resolve(r.Context(), token)
		if err != nil || a == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session")
			return
		}
		next.ServeHTTP(w, r.WithContext(withAccount(r.Context(), a)))
	})
}

// requireRole rejects accounts below the given role. Must run after requireAuth.
func requireRole(min models.AccountRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := accountFrom(r.Context())
			if a == nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
				return
			}
			if !a.Role.AtLeast(min) {
				writeError(w, http.StatusForbidden, "forbidden", "this action requires the "+string(min)+" role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
