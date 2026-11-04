package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

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

func (d Deps) adminGetTour(w http.ResponseWriter, r *http.Request) {
	tour, err := d.Store.GetTour(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		d.fail(w, err)
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
	in := repository.UpdateTourInput{Title: req.Title, Summary: req.Summary, Modes: req.Modes, HeroMediaID: req.HeroMediaID, Visibility: req.Visibility}
	if req.TopicID != nil {
		if strings.TrimSpace(*req.TopicID) == "" {
			in.ClearTopic = true
		} else {
			in.TopicID = req.TopicID
		}
	}
	tour, err := d.Store.UpdateTour(r.Context(), chi.URLParam(r, "id"), in)
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
	id := chi.URLParam(r, "id")
	if err := d.Store.ReplaceStops(r.Context(), id, req.Stops); err != nil {
		d.fail(w, err)
		return
	}
	d.respondTour(w, r, id)
}

func (d Deps) respondTour(w http.ResponseWriter, r *http.Request, id string) {
	tour, err := d.Store.GetTour(r.Context(), id)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tour)
}

func (d Deps) adminPublishTour(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := d.Store.GetTour(r.Context(), id); err != nil {
		d.fail(w, err)
		return
	}
	blockers, err := d.Store.TourPublishBlockers(r.Context(), id)
	if err != nil {
		d.fail(w, err)
		return
	}
	if len(blockers) > 0 {
		writeError(w, http.StatusConflict, "conflict", "cannot publish: "+strings.Join(blockers, "; "))
		return
	}
	if err := d.Store.SetTourStatus(r.Context(), id, models.EntityStatusPublished); err != nil {
		d.fail(w, err)
		return
	}
	d.respondTour(w, r, id)
}

func (d Deps) adminUnpublishTour(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := d.Store.SetTourStatus(r.Context(), id, models.EntityStatusDraft); err != nil {
		d.fail(w, err)
		return
	}
	d.respondTour(w, r, id)
}

func (d Deps) adminDeleteTour(w http.ResponseWriter, r *http.Request) {
	if err := d.Store.DeleteTour(r.Context(), chi.URLParam(r, "id")); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) adminRecomputeRoutes(w http.ResponseWriter, r *http.Request) {
	if err := d.Store.RecomputeRoutes(r.Context(), chi.URLParam(r, "id")); err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}
