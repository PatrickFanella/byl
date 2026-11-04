package httpapi

import (
	"net/http"
	"strings"

	"invisible-museum/backend/internal/models"
)

// devSignIn exists only so the dev auth test helper and later tasks share one path; Task 10 fills in the rest.
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
