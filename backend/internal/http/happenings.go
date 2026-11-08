package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func happeningHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")
}

func decodeHappening(w http.ResponseWriter, r *http.Request, out any) bool {
	happeningHeaders(w)
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "bad_request", "write requests do not accept query parameters")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "provide one bounded documented happening object")
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "provide one bounded documented happening object")
		return false
	}
	return true
}

func (d Deps) happeningFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrSubmissionLimit) {
		writeError(w, http.StatusTooManyRequests, "submission_limit", "happening import limits reached; review existing drafts first")
		return
	}
	d.fail(w, err)
}

func (d Deps) happeningCapabilities(w http.ResponseWriter, r *http.Request) {
	happeningHeaders(w)
	writeJSON(w, http.StatusOK, map[string]any{"import_enabled": d.HappeningsImport, "automatic_discovery": false, "occurrence_precision": "date", "max_evidence": 5, "max_drafts_per_day": 40})
}

func (d Deps) adminSaveHappening(update bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		happeningHeaders(w)
		if !update && !d.HappeningsImport {
			writeError(w, http.StatusForbidden, "happenings_import_disabled", "new current-happening import is disabled; existing review and notices remain available")
			return
		}
		var in models.HappeningWriteRequest
		if !decodeHappening(w, r, &in) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if !models.ValidContributionKey(key) {
			writeValidation(w, models.Fields{"idempotency_key": "retain 16–128 printable non-space ASCII characters before sending"})
			return
		}
		id := ""
		if update {
			id = chi.URLParam(r, "id")
		}
		draft, err := d.Store.SaveHappeningDraft(r.Context(), id, accountFrom(r.Context()).ID, key, in, time.Now())
		if err != nil {
			d.happeningFailure(w, err)
			return
		}
		status := http.StatusCreated
		if update {
			status = http.StatusOK
		}
		writeJSON(w, status, draft)
	}
}

func (d Deps) adminListHappenings(w http.ResponseWriter, r *http.Request) {
	happeningHeaders(w)
	offset := queryOffset(r)
	if offset < 0 || offset > 10000 {
		writeValidation(w, models.Fields{"offset": "use a bounded nonnegative offset"})
		return
	}
	rows, err := d.Store.ListHappeningDrafts(r.Context(), queryInt(r, "limit", 20), offset)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: rows})
}

func (d Deps) adminGetHappening(w http.ResponseWriter, r *http.Request) {
	happeningHeaders(w)
	row, err := d.Store.GetHappeningDraft(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (d Deps) adminHappeningReviewPackage(w http.ResponseWriter, r *http.Request) {
	happeningHeaders(w)
	p, err := d.Store.GetHappeningReviewPackage(r.Context(), chi.URLParam(r, "id"), time.Now())
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (d Deps) adminReviewHappening(w http.ResponseWriter, r *http.Request) {
	var in models.HappeningReviewRequest
	if !decodeHappening(w, r, &in) {
		return
	}
	decision, err := d.Store.ReviewHappening(r.Context(), chi.URLParam(r, "id"), accountFrom(r.Context()).ID, in, time.Now())
	if err != nil {
		d.happeningFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

func (d Deps) listPlaceHappenings(w http.ResponseWriter, r *http.Request) {
	happeningHeaders(w)
	offset := 0
	var err error
	if value := r.URL.Query().Get("cursor"); value != "" {
		offset, err = strconv.Atoi(value)
	}
	if err != nil || offset < 0 || offset > 10000 {
		writeValidation(w, models.Fields{"cursor": "use a bounded cursor returned by this endpoint"})
		return
	}
	limit := queryInt(r, "limit", 20)
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 50)
	rows, err := d.Store.PublicHappenings(r.Context(), chi.URLParam(r, "place"), limit+1, offset, time.Now())
	if err != nil {
		d.fail(w, err)
		return
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		if offset+limit <= 10000 {
			next = strconv.Itoa(offset + limit)
		}
	}
	writeJSON(w, http.StatusOK, listResponse{Items: rows, NextCursor: next})
}
