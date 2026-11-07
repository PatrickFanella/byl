package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

// adminPathQuality exposes retained evidence only within the curator workspace.
func (d Deps) adminPathQuality(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	mode := models.TravelMode(r.URL.Query().Get("mode"))
	if !mode.Valid() {
		writeValidation(w, map[string]string{"mode": "must be walk or bike"})
		return
	}
	id := r.URL.Query().Get("report_id")
	report, err := d.Store.GetPathQualityReport(r.Context(), tour.ID, mode, id)
	if errors.Is(err, repository.ErrNotFound) && id == "" {
		writeJSON(w, http.StatusOK, map[string]any{"mode": mode, "report": nil})
		return
	}
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": mode, "report": report})
}

func (d Deps) adminListTours(w http.ResponseWriter, r *http.Request) {
	f := repository.TourFilter{IncludeUnpublished: true, Limit: queryInt(r, "limit", 50), Cursor: r.URL.Query().Get("cursor")}
	if city := r.URL.Query().Get("city"); city != "" {
		id, err := d.Store.ResolveCityID(r.Context(), city)
		if err != nil {
			d.fail(w, err)
			return
		}
		f.CityID = id
	}
	tours, next, err := d.Store.ListTours(r.Context(), f)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: tours, NextCursor: next})
}

// adminTour resolves {id} (id or slug) to a tour, or writes the error.
func (d Deps) adminTour(w http.ResponseWriter, r *http.Request) (*models.TourDetail, bool) {
	tour, err := d.Store.GetTour(r.Context(), chi.URLParam(r, "id"), false)
	if err != nil {
		d.fail(w, err)
		return nil, false
	}
	return tour, true
}

func (d Deps) adminGetTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, tour)
}

func (d Deps) adminCreateTour(w http.ResponseWriter, r *http.Request) {
	var req models.CreateTourRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	tour, err := d.Store.CreateTour(r.Context(), repository.CreateTourInput{
		CityID: req.CityID, Slug: req.Slug, Title: req.Title, Summary: req.Summary, TopicID: req.TopicID, Modes: req.Modes,
		HeroMediaID: req.HeroMediaID, Visibility: req.Visibility, Stops: req.Stops,
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tour)
}

func (d Deps) adminUpdateTour(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateTourRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	existing, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	expected := req.DraftVersion
	if expected == nil {
		expected = &existing.DraftVersion
	}
	in := repository.UpdateTourInput{ExpectedVersion: expected, Title: req.Title, Summary: req.Summary, Modes: req.Modes, HeroMediaID: req.HeroMediaID, Visibility: req.Visibility}
	if req.TopicID != nil {
		if strings.TrimSpace(*req.TopicID) == "" {
			in.ClearTopic = true
		} else {
			in.TopicID = req.TopicID
		}
	}
	tour, err := d.Store.UpdateTour(r.Context(), existing.ID, in)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tour)
}

func (d Deps) adminSetTourStops(w http.ResponseWriter, r *http.Request) {
	var req models.StopsRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	existing, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	expected := req.DraftVersion
	if expected == nil {
		expected = &existing.DraftVersion
	}
	if err := d.Store.ReplaceStopsVersioned(r.Context(), existing.ID, req.Stops, expected); err != nil {
		d.fail(w, err)
		return
	}
	d.respondTour(w, r, existing.ID)
}

func (d Deps) respondTour(w http.ResponseWriter, r *http.Request, id string) {
	tour, err := d.Store.GetTour(r.Context(), id, false)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tour)
}

func (d Deps) adminPublishTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	d.publishTourRevision(w, r, tour)
}

func (d Deps) adminUnpublishTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	id := tour.ID
	if err := d.Store.SetTourStatus(r.Context(), id, models.EntityStatusDraft); err != nil {
		d.fail(w, err)
		return
	}
	d.respondTour(w, r, id)
}

func (d Deps) adminDeleteTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	if err := d.Store.DeleteTour(r.Context(), tour.ID); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) adminRecomputeRoutes(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	if err := d.Store.RecomputeRoutes(r.Context(), tour.ID); err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}
