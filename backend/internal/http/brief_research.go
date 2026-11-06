package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
)

func (d Deps) getBriefResearch(curator bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner := accountFrom(r.Context()).ID
		if curator {
			owner = ""
		}
		run, err := d.Store.LatestBriefResearch(r.Context(), chi.URLParam(r, "id"), owner)
		if err != nil {
			d.fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, run)
	}
}

func (d Deps) adminBriefResearch(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in models.ResearchActionRequest
		if !decodeOrFail(w, r, &in) {
			return
		}
		fields := models.Fields{}
		if in.Version < 1 {
			fields["version"] = "must be a positive version from the latest brief"
		}
		if action != "start" && in.RunID == "" {
			fields["run_id"] = "is required"
		}
		if action == "start" && in.RunID != "" {
			fields["run_id"] = "omit when starting a new run"
		}
		if len(fields) > 0 {
			writeValidation(w, fields)
			return
		}
		var run *models.ResearchRun
		var err error
		if action == "start" {
			run, err = d.Store.StartBriefResearch(r.Context(), chi.URLParam(r, "id"), in.Version)
		} else {
			run, err = d.Store.ChangeBriefResearch(r.Context(), chi.URLParam(r, "id"), in, action == "resume")
		}
		if err != nil {
			d.fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusAccepted, run)
	}
}
