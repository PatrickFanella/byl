package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"invisible-museum/backend/internal/models"
)

func (d Deps) publishTourRevision(w http.ResponseWriter, r *http.Request, tour *models.TourDetail) {
	in := models.PublishRevisionRequest{DraftVersion: tour.DraftVersion, CurrentRevisionID: tour.RevisionID}
	if r.ContentLength != 0 {
		in = models.PublishRevisionRequest{}
		if !decodeOrFail(w, r, &in) {
			return
		}
	}
	if in.DraftVersion < 0 {
		writeValidation(w, models.Fields{"draft_version": "must be the current draft version"})
		return
	}
	revision, err := d.Store.PublishTourRevision(r.Context(), tour.ID, accountFrom(r.Context()).ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, revision.Snapshot.Tour)
}

func (d Deps) publishMyTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.myTour(w, r)
	if !ok {
		return
	}
	d.publishTourRevision(w, r, tour)
}

func (d Deps) listTourRevisions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")
	tour, ok := d.visibleTourBase(w, r)
	if !ok {
		return
	}
	a := accountFrom(r.Context())
	private := a != nil && (a.Role.AtLeast(models.AccountRoleCurator) || (tour.OwnerAccountID != nil && *tour.OwnerAccountID == a.ID))
	items, err := d.Store.ListTourRevisions(r.Context(), tour.ID, queryInt(r, "limit", 20), queryOffset(r), private)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}

func (d Deps) getTourRevision(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")
	tour, ok := d.visibleTourBase(w, r)
	if !ok {
		return
	}
	revision, err := d.Store.GetTourRevision(r.Context(), tour.ID, chi.URLParam(r, "revision"))
	if err != nil {
		d.fail(w, err)
		return
	}
	if !canSeeTour(&revision.Snapshot.Tour.TourSummary, accountFrom(r.Context())) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	writeJSON(w, http.StatusOK, revision)
}

func (d Deps) revisionProgress(put bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tour, ok := d.visibleTourBase(w, r)
		if !ok {
			return
		}
		revision, err := d.Store.GetTourRevision(r.Context(), tour.ID, chi.URLParam(r, "revision"))
		if err != nil {
			d.fail(w, err)
			return
		}
		if !canSeeTour(&revision.Snapshot.Tour.TourSummary, accountFrom(r.Context())) {
			writeError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		account := accountFrom(r.Context()).ID
		var progress *models.RevisionProgress
		if put {
			var in models.RevisionProgressRequest
			if !decodeOrFail(w, r, &in) {
				return
			}
			if fields := in.Validate(); len(fields) > 0 {
				writeValidation(w, fields)
				return
			}
			p := models.RevisionProgress{AccountID: account, TourID: tour.ID, RevisionID: revision.ID, Mode: in.Mode, VisitedStopIDs: in.VisitedStopIDs}
			if in.Completed {
				now := time.Now()
				p.CompletedAt = &now
			}
			progress, err = d.Store.PutRevisionProgress(r.Context(), p, in.Reset)
		} else {
			progress, err = d.Store.GetRevisionProgress(r.Context(), account, revision.ID)
		}
		if err != nil {
			d.fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, progress)
	}
}
