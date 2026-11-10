package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

type fakeResolver struct{ accounts map[string]*models.Account }

func (f fakeResolver) Resolve(_ context.Context, token string) (*models.Account, error) {
	if a, ok := f.accounts[token]; ok {
		return a, nil
	}
	if token == "outage" {
		return nil, errors.New("connection refused")
	}
	return nil, repository.ErrNotFound
}

func protectedRouter(resolver SessionResolver) http.Handler {
	deps := Deps{Sessions: resolver}
	mux := http.NewServeMux()
	mux.Handle("/auth", deps.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(accountFrom(r.Context()).ID))
	})))
	mux.Handle("/curator", deps.requireAuth(requireRole(models.AccountRoleCurator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))))
	return mux
}

func TestRequireAuth(t *testing.T) {
	r := protectedRouter(fakeResolver{accounts: map[string]*models.Account{
		"good": {ID: "acct-1", Role: models.AccountRoleVisitor},
	}})

	cases := []struct {
		name   string
		header string
		path   string
		want   int
	}{
		{"missing header", "", "/auth", 401},
		{"wrong scheme", "Basic abc", "/auth", 401},
		{"unknown token", "Bearer nope", "/auth", 401},
		{"session store outage", "Bearer outage", "/auth", 503},
		{"valid", "Bearer good", "/auth", 200},
		{"visitor on curator route", "Bearer good", "/curator", 403},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestOptionalAuthPassesAnonymous(t *testing.T) {
	deps := Deps{Sessions: fakeResolver{}}
	h := deps.optionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accountFrom(r.Context()) != nil {
			t.Fatal("expected no account")
		}
		w.WriteHeader(204)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 204 {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestOptionalAuthReportsSessionOutage(t *testing.T) {
	deps := Deps{Sessions: fakeResolver{}}
	h := deps.optionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run anonymously when the session lookup failed")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer outage")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}
