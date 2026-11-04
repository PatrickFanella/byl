package httpapi

import (
	"net/http"
	"strings"

	"invisible-museum/backend/internal/models"
)

func (d Deps) devSignIn(w http.ResponseWriter, r *http.Request) {
	if !d.DevAuth {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req models.DevSignInRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	d.issueSession(w, r, models.AuthProviderDev, email, email, req.DisplayName)
}

func (d Deps) appleSignIn(w http.ResponseWriter, r *http.Request) {
	if d.Apple == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Sign in with Apple is not configured")
		return
	}
	var req models.AppleSignInRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.IdentityToken) == "" {
		writeValidation(w, map[string]string{"identity_token": "is required"})
		return
	}
	id, err := d.Apple.Verify(r.Context(), req.IdentityToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "identity token was rejected")
		return
	}
	// Apple only sends the name on first sign-in, from the client, so prefer the request's value.
	name := req.DisplayName
	if name == "" {
		name = id.Name
	}
	d.issueSession(w, r, models.AuthProviderApple, id.Subject, id.Email, name)
}

func (d Deps) googleSignIn(w http.ResponseWriter, r *http.Request) {
	if d.Google == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Google sign-in is not configured")
		return
	}
	var req models.GoogleSignInRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.IDToken) == "" {
		writeValidation(w, map[string]string{"id_token": "is required"})
		return
	}
	id, err := d.Google.Verify(r.Context(), req.IDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "identity token was rejected")
		return
	}
	d.issueSession(w, r, models.AuthProviderGoogle, id.Subject, id.Email, id.Name)
}

func (d Deps) issueSession(w http.ResponseWriter, r *http.Request, provider models.AuthProvider, subject, email, name string) {
	bootstrap := d.IsBootstrap != nil && d.IsBootstrap(email)
	acct, err := d.Store.UpsertAccount(r.Context(), provider, subject, email, name, bootstrap)
	if err != nil {
		d.fail(w, err)
		return
	}
	token, expires, err := d.SessionManager.Issue(r.Context(), acct.ID)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, models.Session{Token: token, ExpiresAt: expires, Account: *acct})
}

func (d Deps) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, accountFrom(r.Context()))
}

func (d Deps) logout(w http.ResponseWriter, r *http.Request) {
	token, _ := bearerToken(r)
	if err := d.SessionManager.Revoke(r.Context(), token); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
