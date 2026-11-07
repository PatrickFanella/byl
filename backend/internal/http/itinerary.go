package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (d Deps) adminBriefItinerary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	out, err := d.Store.GetBriefItinerary(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("run_id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
