package httpapi

import (
	"bytes"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
)

func (d Deps) adminProposeCorridor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	const maxImportBody = 256 << 10
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxImportBody+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read reference import")
		return
	}
	if len(raw) > maxImportBody {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "corridor imports allow at most 256 KiB")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var in models.CorridorProposalRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	fields := models.Fields{}
	if in.Version < 0 || in.Remove == (in.Intent != nil) {
		fields["intent"] = "supply an intent or explicit removal, with the current approved version"
	}
	if in.Intent != nil {
		for k, v := range in.Intent.Validate() {
			fields[k] = v
		}
	}
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	p, err := d.Store.ProposeCorridor(r.Context(), tour.ID, accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (d Deps) adminListCorridors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	items, err := d.Store.ListCorridorProposals(r.Context(), tour.ID, queryInt(r, "limit", 20), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) adminGetCorridor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	p, err := d.Store.GetCorridorProposal(r.Context(), tour.ID, chi.URLParam(r, "proposal"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (d Deps) adminReviewCorridor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	var in models.ArrivalReviewRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if f := in.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	p, err := d.Store.ReviewCorridor(r.Context(), tour.ID, chi.URLParam(r, "proposal"), accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
