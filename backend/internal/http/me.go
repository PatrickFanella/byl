package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func (d Deps) listMyTours(w http.ResponseWriter, r *http.Request) {
	a := accountFrom(r.Context())
	tours, next, err := d.Store.ListTours(r.Context(), repository.TourFilter{OwnerID: a.ID, IncludeUnpublished: true, Limit: queryInt(r, "limit", 50), Cursor: r.URL.Query().Get("cursor")})
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: tours, NextCursor: next})
}

// checkStopsPublished writes a 422 and returns false when any stop is not a published place in the city.
func (d Deps) checkStopsPublished(w http.ResponseWriter, r *http.Request, cityID string, stops []models.StopInput) bool {
	ids := make([]string, 0, len(stops))
	for _, s := range stops {
		ids = append(ids, s.PlaceID)
	}
	bad, err := d.Store.UnpublishedOrForeign(r.Context(), cityID, ids)
	if err != nil {
		d.fail(w, err)
		return false
	}
	if len(bad) > 0 {
		writeValidation(w, map[string]string{"stops": fmt.Sprintf("stops %v must be published places in the route's city", bad)})
		return false
	}
	return true
}

func (d Deps) createMyTour(w http.ResponseWriter, r *http.Request) {
	a := accountFrom(r.Context())
	var req models.CreateUserTourRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	cityID, err := d.Store.ResolveCityID(r.Context(), req.City)
	if err != nil {
		d.fail(w, err)
		return
	}
	if !d.checkStopsPublished(w, r, cityID, req.Stops) {
		return
	}
	// User routes carry no curator-chosen moments; the place's primary moment is featured.
	for i := range req.Stops {
		req.Stops[i].FeaturedMomentID = nil
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = models.TourVisibilityUnlisted
	}
	tour, err := d.Store.CreateTour(r.Context(), repository.CreateTourInput{
		CityID: cityID, RandomSlug: true, Title: req.Title, Summary: req.Summary, Modes: req.Modes,
		Status: models.EntityStatusDraft, Visibility: visibility, OwnerAccountID: &a.ID, Stops: req.Stops,
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tour)
}

// myTour loads a tour the current account owns, or writes 404.
func (d Deps) myTour(w http.ResponseWriter, r *http.Request) (*models.TourDetail, bool) {
	a := accountFrom(r.Context())
	tour, err := d.Store.GetTour(r.Context(), chi.URLParam(r, "id"), false)
	if err != nil {
		d.fail(w, err)
		return nil, false
	}
	if tour.OwnerAccountID == nil || *tour.OwnerAccountID != a.ID {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return nil, false
	}
	return tour, true
}

func (d Deps) getMyTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.myTour(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, tour)
}

func (d Deps) updateMyTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.myTour(w, r)
	if !ok {
		return
	}
	var req models.UpdateUserTourRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	if req.Stops != nil {
		if !d.checkStopsPublished(w, r, tour.CityID, req.Stops) {
			return
		}
		for i := range req.Stops {
			req.Stops[i].FeaturedMomentID = nil
		}
	}
	expected := req.DraftVersion
	if expected == nil {
		expected = &tour.DraftVersion
	}
	err := d.Store.Transaction(r.Context(), func(tx *repository.Store) error {
		updated, err := tx.UpdateTour(r.Context(), tour.ID, repository.UpdateTourInput{Title: req.Title, Summary: req.Summary, Modes: req.Modes, Visibility: req.Visibility, ExpectedVersion: expected, DeferLegacyRelease: req.Stops != nil})
		if err != nil {
			return err
		}
		if req.Stops != nil {
			return tx.ReplaceStopsVersioned(r.Context(), tour.ID, req.Stops, &updated.DraftVersion)
		}
		return nil
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	updated, err := d.Store.GetTour(r.Context(), tour.ID, false)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (d Deps) deleteMyTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.myTour(w, r)
	if !ok {
		return
	}
	if err := d.Store.DeleteTour(r.Context(), tour.ID); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) listSaved(w http.ResponseWriter, r *http.Request) {
	tours, err := d.Store.ListSaved(r.Context(), accountFrom(r.Context()).ID)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: tours})
}

func (d Deps) saveTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.visibleTour(w, r)
	if !ok {
		return
	}
	if err := d.Store.Save(r.Context(), accountFrom(r.Context()).ID, tour.ID); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) unsaveTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.visibleTour(w, r)
	if !ok {
		return
	}
	if err := d.Store.Unsave(r.Context(), accountFrom(r.Context()).ID, tour.ID); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) listProgress(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Store.ListProgress(r.Context(), accountFrom(r.Context()).ID)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: rows})
}

func (d Deps) getProgress(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.visibleTour(w, r)
	if !ok {
		return
	}
	if tour.RevisionID == nil {
		writeError(w, http.StatusConflict, "conflict", "a published revision is not ready")
		return
	}
	var p *models.TourProgress
	err := d.Store.Transaction(r.Context(), func(tx *repository.Store) error {
		var err error
		p, err = tx.GetProgress(r.Context(), accountFrom(r.Context()).ID, tour.ID)
		if err == nil && p.RevisionID != *tour.RevisionID {
			return repository.ErrConflict
		}
		return err
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (d Deps) putProgress(w http.ResponseWriter, r *http.Request) {
	var req models.ProgressRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	var tour *models.TourDetail
	if req.Reset {
		// Reset targets the current release, not the old served legacy pin. Check
		// that exact snapshot before the transaction is allowed to change the pin.
		working, ok := d.visibleTourBase(w, r)
		if !ok {
			return
		}
		if working.RevisionID == nil {
			writeError(w, http.StatusConflict, "conflict", "a published revision is not ready")
			return
		}
		revision, err := d.Store.GetTourRevision(r.Context(), working.ID, *working.RevisionID)
		if err != nil {
			d.fail(w, err)
			return
		}
		tour = &revision.Snapshot.Tour
		if !canSeeTour(&tour.TourSummary, accountFrom(r.Context())) {
			writeError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if tour.Transit != nil && !d.requireTransitClient(w, r) {
			return
		}
	} else {
		served, ok := d.visibleTour(w, r)
		if !ok {
			return
		}
		tour = served
	}
	if tour.RevisionID == nil {
		writeError(w, http.StatusConflict, "conflict", "a published revision is not ready")
		return
	}
	a := accountFrom(r.Context())
	in := models.TourProgress{AccountID: a.ID, TourID: tour.ID, Mode: req.Mode, VisitedPositions: req.VisitedPositions}
	in.RevisionID = *tour.RevisionID
	if req.Completed {
		now := time.Now()
		in.CompletedAt = &now
	}
	p, err := d.Store.PutProgress(r.Context(), in, req.Reset)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
