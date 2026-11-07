package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/sources"
)

func (d Deps) adminListReconciliation(w http.ResponseWriter, r *http.Request) {
	items, err := d.Store.ListReconciliation(r.Context(), chi.URLParam(r, "id"), queryInt(r, "limit", 20), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) adminListReconciliationSources(w http.ResponseWriter, r *http.Request) {
	items, err := d.Store.ListReconciliationSources(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "record"), queryInt(r, "limit", 20), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) adminReconcileBrief(w http.ResponseWriter, r *http.Request) {
	var in models.ReconciliationRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	fields := in.Validate()
	if in.Item.URL != "" {
		if _, _, _, err := sources.NormalizeEvidenceURL(in.Item.URL); err != nil {
			fields["item.url"] = "must be a public HTTP or HTTPS source URL"
		}
	}
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	result, err := d.Store.ReconcileBrief(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (d Deps) adminDecideReconciliation(w http.ResponseWriter, r *http.Request) {
	var in models.IdentityDecisionRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if fields := in.Validate(); len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	result, err := d.Store.DecideReconciliation(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "record"), accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}
