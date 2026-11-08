package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
)

func (d Deps) adminRouteCorrectionComparison(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	p, err := d.Store.RouteCorrectionComparison(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("investigation_id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (d Deps) adminApproveRouteCorrection(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in models.ApproveRouteCorrectionRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if f := in.Validate(time.Time{}); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	c, err := d.Store.ApproveRouteCorrection(r.Context(), chi.URLParam(r, "id"), accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (d Deps) adminListRouteCorrections(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	if value := r.URL.Query().Get("due"); value != "" && value != "true" && value != "false" {
		writeValidation(w, models.Fields{"due": "must be true or false"})
		return
	}
	items, err := d.Store.ListRouteCorrections(r.Context(), r.URL.Query().Get("due") == "true", queryInt(r, "limit", 50), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) routeCorrectionOutcome(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	// Check ownership before checking for a release, including unknown report IDs.
	report, err := d.Store.GetRouteReport(r.Context(), chi.URLParam(r, "id"), accountFrom(r.Context()).ID)
	if err != nil {
		d.fail(w, err)
		return
	}
	c, err := d.Store.GetRouteCorrection(r.Context(), report.ID)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, models.RouteCorrectionOutcome{RevisionID: c.RevisionID, PreviousRevisionID: c.PreviousRevisionID, Temporary: c.Temporary, ReviewOn: c.ReviewOn, ReviewDue: c.ReviewDue})
}
