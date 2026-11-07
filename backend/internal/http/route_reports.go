package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func routeReportHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Authorization")
}

func (d Deps) createRouteReport(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	const maxReportBody = 32 << 10
	body, err := io.ReadAll(io.LimitReader(r.Body, maxReportBody+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read report")
		return
	}
	if len(body) > maxReportBody {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "route reports allow at most 32 KiB")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var in models.RouteReportInput
	if !decodeOrFail(w, r, &in) {
		return
	}
	in.Normalize()
	if f := in.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 128 || strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		writeValidation(w, models.Fields{"idempotency_key": "must contain at most 128 printable ASCII characters without spaces"})
		return
	}
	report, created, err := d.Store.CreateRouteReport(r.Context(), accountFrom(r.Context()).ID, in, key)
	if errors.Is(err, repository.ErrSubmissionLimit) {
		writeError(w, http.StatusTooManyRequests, "submission_limit", "at most 10 new reports per 24 hours and 20 unresolved reports per account")
		return
	}
	if err != nil {
		d.fail(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, report)
}

func (d Deps) listRouteReports(curator bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routeReportHeaders(w)
		owner := accountFrom(r.Context()).ID
		if curator {
			owner = ""
		}
		status := models.BriefStatus(r.URL.Query().Get("status"))
		if status != "" && !status.Valid() {
			writeValidation(w, models.Fields{"status": "unknown report state"})
			return
		}
		items, err := d.Store.ListRouteReports(r.Context(), owner, status, queryInt(r, "limit", 50), queryOffset(r))
		if err != nil {
			d.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse{Items: items})
	}
}

func (d Deps) getRouteReport(curator bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routeReportHeaders(w)
		owner := accountFrom(r.Context()).ID
		if curator {
			owner = ""
		}
		report, err := d.Store.GetRouteReport(r.Context(), chi.URLParam(r, "id"), owner)
		if err != nil {
			d.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func (d Deps) adminSetRouteReportState(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	var in models.BriefStateRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if f := in.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	report, err := d.Store.SetRouteReportState(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
