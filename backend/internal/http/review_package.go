package httpapi

import (
	"encoding/hex"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
)

func (d Deps) adminPublishReviewPackage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")
	var in models.PublishPackageRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	fields := models.Fields{}
	if raw, err := hex.DecodeString(in.Version); err != nil || len(raw) != 32 {
		fields["version"] = "must be the exact 64-character SHA-256 review version"
	}
	if in.RunID == "" {
		fields["run_id"] = "must identify the reviewed research run"
	}
	if in.TourID == "" {
		fields["tour_id"] = "must identify the reviewed prepared tour"
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > 2000 {
		fields["note"] = "must contain at most 2000 characters"
	}
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	revision, err := d.Store.PublishCuratorPackage(r.Context(), chi.URLParam(r, "id"), accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, revision)
}

func (d Deps) adminReviewPackage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")
	q := r.URL.Query()
	expected := q.Get("expected_version")
	if expected != "" {
		raw, err := hex.DecodeString(expected)
		if err != nil || len(raw) != 32 {
			writeValidation(w, map[string]string{"expected_version": "must be a 64-character SHA-256 review version"})
			return
		}
	}
	out, err := d.Store.GetCuratorReviewPackage(r.Context(), chi.URLParam(r, "id"), q.Get("run_id"), q.Get("tour_id"), expected)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
