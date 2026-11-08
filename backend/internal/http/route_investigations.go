package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func (d Deps) adminRelatedRouteReports(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	items, err := d.Store.RelatedRouteReports(r.Context(), chi.URLParam(r, "id"), queryInt(r, "limit", 50), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) adminQueueRouteInvestigation(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var in models.RouteInvestigationRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if in.Research != nil {
		in.Research.Normalize()
	}
	if f := in.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 128 || strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		writeValidation(w, models.Fields{"idempotency_key": "must contain at most 128 printable ASCII characters without spaces"})
		return
	}
	i, err := d.Store.QueueRouteInvestigation(r.Context(), chi.URLParam(r, "id"), accountFrom(r.Context()).ID, in, key)
	if errors.Is(err, repository.ErrSubmissionLimit) {
		writeError(w, http.StatusTooManyRequests, "submission_limit", "investigations use the sourcing brief and evidence quotas")
		return
	}
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, i)
}

func (d Deps) adminListRouteInvestigations(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	items, err := d.Store.ListRouteInvestigations(r.Context(), chi.URLParam(r, "id"), queryInt(r, "limit", 50), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) adminGetRouteInvestigation(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	i, err := d.Store.GetRouteInvestigation(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "investigation"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, i)
}
