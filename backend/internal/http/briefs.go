package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func briefHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Authorization")
}

func (d Deps) routeBriefIntakeStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": d.BriefIntake})
}

func (d Deps) createMyRouteBrief(w http.ResponseWriter, r *http.Request) {
	if !d.BriefIntake {
		d.routeBriefIntakeDisabled(w, r)
		return
	}
	if strings.IndexFunc(r.Header.Get("Idempotency-Key"), func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		briefHeaders(w)
		writeValidation(w, map[string]string{"idempotency_key": "must contain printable ASCII characters without spaces"})
		return
	}
	// The shared creator stores leads only, with existing owner quotas and retry
	// keys. It grants no research, review, route mutation or publication authority.
	d.adminCreateRouteBrief(w, r)
}

func (d Deps) listRouteBriefs(curator bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		briefHeaders(w)
		owner := accountFrom(r.Context()).ID
		if curator {
			owner = ""
		}
		status := models.BriefStatus(r.URL.Query().Get("status"))
		if status != "" && !status.Valid() {
			writeValidation(w, map[string]string{"status": "unknown brief state"})
			return
		}
		briefs, err := d.Store.ListRouteBriefs(r.Context(), owner, status, queryInt(r, "limit", 50), queryOffset(r))
		if err != nil {
			d.fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, listResponse{Items: briefs})
	}
}

func (d Deps) getRouteBrief(curator bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		briefHeaders(w)
		owner := accountFrom(r.Context()).ID
		if curator {
			owner = ""
		}
		brief, err := d.Store.GetRouteBrief(r.Context(), chi.URLParam(r, "id"), owner)
		if err != nil {
			d.fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, brief)
	}
}

func (d Deps) routeBriefIntakeDisabled(w http.ResponseWriter, r *http.Request) {
	briefHeaders(w)
	message := "public route brief submissions are not open yet"
	if r.Method == http.MethodPut {
		message = "submitted route suggestions cannot be edited here"
	}
	writeError(w, http.StatusForbidden, "intake_disabled", message)
}

func (d Deps) adminCreateRouteBrief(w http.ResponseWriter, r *http.Request) {
	briefHeaders(w)
	var req models.RouteBriefInput
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 128 {
		writeValidation(w, map[string]string{"idempotency_key": "must be at most 128 bytes"})
		return
	}
	brief, created, err := d.Store.CreateRouteBrief(r.Context(), accountFrom(r.Context()).ID, req, key)
	if errors.Is(err, repository.ErrSubmissionLimit) {
		writeError(w, http.StatusTooManyRequests, "submission_limit", "at most 10 new briefs per 24 hours and 20 unresolved briefs per account")
		return
	}
	if err != nil {
		d.fail(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, brief)
}

func (d Deps) adminUpdateRouteBrief(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateRouteBriefRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	f := req.Submitted.Validate()
	if req.Version < 1 {
		f["version"] = "must be a positive version from the latest brief"
	}
	if len(f) > 0 {
		writeValidation(w, f)
		return
	}
	brief, err := d.Store.UpdateRouteBrief(r.Context(), chi.URLParam(r, "id"), "", req)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, brief)
}

func (d Deps) adminSetRouteBriefState(w http.ResponseWriter, r *http.Request) {
	var req models.BriefStateRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	brief, err := d.Store.SetRouteBriefState(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, brief)
}
