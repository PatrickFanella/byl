package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/models"
)

func (d Deps) adminListJobs(w http.ResponseWriter, r *http.Request) {
	status := models.JobStatus(r.URL.Query().Get("status"))
	if status != "" && !status.Valid() {
		writeValidation(w, map[string]string{"status": "must be queued, running, done, or failed"})
		return
	}
	jobs, err := d.Store.ListJobs(r.Context(), status, queryInt(r, "limit", 50), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: jobs})
}

func (d Deps) adminRetryJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := d.Store.RetryJob(r.Context(), id); err != nil {
		d.fail(w, err)
		return
	}
	job, err := d.Store.GetJob(r.Context(), id)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (d Deps) adminListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := d.Store.ListAccounts(r.Context(), queryInt(r, "limit", 50), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: accounts})
}

func (d Deps) adminSetRole(w http.ResponseWriter, r *http.Request) {
	var req models.RoleRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	id := chi.URLParam(r, "id")
	if err := d.Store.SetRole(r.Context(), id, req.Role); err != nil {
		d.fail(w, err)
		return
	}
	acct, err := d.Store.GetAccount(r.Context(), id)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, acct)
}
