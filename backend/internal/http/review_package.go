package httpapi

import (
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"
)

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
