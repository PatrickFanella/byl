package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
)

func (d Deps) getPlaceArrival(w http.ResponseWriter, r *http.Request) {
	mode := models.TravelMode(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = models.TravelModeWalk
	}
	if !mode.Valid() {
		writeValidation(w, models.Fields{"mode": "must be walk or bike"})
		return
	}
	p, err := d.Store.GetPlace(r.Context(), chi.URLParam(r, "place"), r.URL.Query().Get("city"), false)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, p.SelectArrival(mode))
}

func (d Deps) adminListArrivals(w http.ResponseWriter, r *http.Request) {
	items, err := d.Store.ListArrivalProposals(r.Context(), chi.URLParam(r, "id"), queryInt(r, "limit", 20), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) adminProposeArrival(w http.ResponseWriter, r *http.Request) {
	var in models.ArrivalProposalRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if fields := in.Validate(); len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	a, err := d.Store.ProposeArrival(r.Context(), chi.URLParam(r, "id"), accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, a)
}

func (d Deps) adminReviewArrival(w http.ResponseWriter, r *http.Request) {
	var in models.ArrivalReviewRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if fields := in.Validate(); len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	a, err := d.Store.ReviewArrival(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "arrival"), accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, a)
}
