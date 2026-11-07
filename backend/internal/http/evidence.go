package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/sources"
)

func (d Deps) adminQueueSourceEvidence(w http.ResponseWriter, r *http.Request) {
	var in models.EvidenceRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	fields := models.Fields{}
	if in.Version < 1 {
		fields["version"] = "must be a positive saved brief version"
	}
	if in.SourceKind == "" {
		in.SourceKind = "page"
	}
	if in.SourceKind != "page" && in.SourceKind != "catalog" {
		fields["source_kind"] = "must be page or catalog"
	}
	normalized, reference, key, err := sources.NormalizeEvidenceURL(in.URL)
	if err != nil {
		fields["url"] = "must be a public HTTP or HTTPS URL on its standard port, without credentials, at most 2000 bytes"
	}
	requestKey := r.Header.Get("Idempotency-Key")
	if len(requestKey) > 128 {
		fields["idempotency_key"] = "must be at most 128 bytes"
	}
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	in.URL = normalized
	evidence, err := d.Store.QueueSourceEvidence(r.Context(), chi.URLParam(r, "id"), in, reference, key, requestKey)
	if errors.Is(err, repository.ErrSubmissionLimit) {
		writeError(w, http.StatusTooManyRequests, "submission_limit", "at most 20 evidence requests per brief per rolling 24 hours")
		return
	}
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, evidence)
}

func (d Deps) adminListSourceEvidence(w http.ResponseWriter, r *http.Request) {
	evidence, err := d.Store.ListSourceEvidence(r.Context(), chi.URLParam(r, "id"), queryInt(r, "limit", 20), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, listResponse{Items: evidence})
}

func (d Deps) adminGetSourceEvidence(w http.ResponseWriter, r *http.Request) {
	e, err := d.Store.GetSourceEvidence(r.Context(), chi.URLParam(r, "evidence"))
	if err != nil {
		d.fail(w, err)
		return
	}
	if e.BriefID != chi.URLParam(r, "id") {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, e)
}
