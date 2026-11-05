package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

// Cities

func (d Deps) adminListCities(w http.ResponseWriter, r *http.Request) {
	cities, err := d.Store.ListCities(r.Context())
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: cities})
}

func (d Deps) adminCreateCity(w http.ResponseWriter, r *http.Request) {
	var req models.CreateCityRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	city, err := d.Store.CreateCity(r.Context(), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, city)
}

func (d Deps) adminUpdateCity(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateCityRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	city, err := d.Store.UpdateCity(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, city)
}

// Topics

func (d Deps) adminListTopics(w http.ResponseWriter, r *http.Request) {
	cityID, err := d.Store.ResolveCityID(r.Context(), r.URL.Query().Get("city"))
	if err != nil {
		d.fail(w, err)
		return
	}
	topics, err := d.Store.ListTopics(r.Context(), cityID)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: topics})
}

func (d Deps) adminCreateTopic(w http.ResponseWriter, r *http.Request) {
	var req models.CreateTopicRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	topic, err := d.Store.CreateTopic(r.Context(), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, topic)
}

func (d Deps) adminUpdateTopic(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateTopicRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	topic, err := d.Store.UpdateTopic(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, topic)
}

// Places

func (d Deps) adminListPlaces(w http.ResponseWriter, r *http.Request) {
	f := repository.PlaceFilter{IncludeUnpublished: true, Limit: queryInt(r, "limit", 50), Cursor: r.URL.Query().Get("cursor")}
	if city := r.URL.Query().Get("city"); city != "" {
		id, err := d.Store.ResolveCityID(r.Context(), city)
		if err != nil {
			d.fail(w, err)
			return
		}
		f.CityID = id
	}
	places, next, err := d.Store.ListPlaces(r.Context(), f)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: places, NextCursor: next})
}

// adminPlaceID resolves {id} (id or slug) or writes the error.
func (d Deps) adminPlaceID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := d.Store.ResolvePlaceID(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("city"))
	if err != nil {
		d.fail(w, err)
		return "", false
	}
	return id, true
}

func (d Deps) adminGetPlace(w http.ResponseWriter, r *http.Request) {
	place, err := d.Store.GetPlace(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("city"), true)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}

func (d Deps) adminCreatePlace(w http.ResponseWriter, r *http.Request) {
	var req models.CreatePlaceRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	place, err := d.Store.CreatePlace(r.Context(), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, place)
}

func (d Deps) adminUpdatePlace(w http.ResponseWriter, r *http.Request) {
	var req models.UpdatePlaceRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	id, ok := d.adminPlaceID(w, r)
	if !ok {
		return
	}
	place, err := d.Store.UpdatePlace(r.Context(), id, req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}

func (d Deps) adminSetPlaceStatus(status models.EntityStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := d.adminPlaceID(w, r)
		if !ok {
			return
		}
		if status != models.EntityStatusPublished {
			using, err := d.Store.PublishedCuratedToursUsingPlace(r.Context(), id)
			if err != nil {
				d.fail(w, err)
				return
			}
			if len(using) > 0 {
				writeError(w, http.StatusConflict, "conflict", "place is a stop on published tours: "+strings.Join(using, ", ")+"; unpublish them first")
				return
			}
		}
		if err := d.Store.SetPlaceStatus(r.Context(), id, status); err != nil {
			d.fail(w, err)
			return
		}
		place, err := d.Store.GetPlace(r.Context(), id, "", true)
		if err != nil {
			d.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, place)
	}
}

// Moments

func (d Deps) adminGetMoment(w http.ResponseWriter, r *http.Request) {
	m, err := d.Store.GetMoment(r.Context(), chi.URLParam(r, "id"), true)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (d Deps) adminListMoments(w http.ResponseWriter, r *http.Request) {
	placeID, err := d.Store.ResolvePlaceID(r.Context(), r.URL.Query().Get("place"), r.URL.Query().Get("city"))
	if err != nil {
		d.fail(w, err)
		return
	}
	moments, err := d.Store.ListMoments(r.Context(), placeID, true)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: moments})
}

func (d Deps) adminCreateMoment(w http.ResponseWriter, r *http.Request) {
	var req models.CreateMomentRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	m, err := d.Store.CreateMoment(r.Context(), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (d Deps) adminUpdateMoment(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateMomentRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	m, err := d.Store.UpdateMoment(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (d Deps) adminPublishMoment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := d.Store.GetMoment(r.Context(), id, true); err != nil {
		d.fail(w, err)
		return
	}
	blockers, err := d.Store.MomentPublishBlockers(r.Context(), id)
	if err != nil {
		d.fail(w, err)
		return
	}
	if len(blockers) > 0 {
		writeError(w, http.StatusConflict, "conflict", "cannot publish: "+strings.Join(blockers, "; "))
		return
	}
	err = d.Store.Transaction(r.Context(), func(tx *repository.Store) error {
		if err := tx.SetMomentStatus(r.Context(), id, models.EntityStatusPublished); err != nil {
			return err
		}
		return tx.Enqueue(r.Context(), models.JobKindGenerateNarration, models.GenerateNarrationPayload{MomentID: id})
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	d.respondMoment(w, r, id)
}

func (d Deps) adminSetMomentStatus(status models.EntityStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := d.Store.SetMomentStatus(r.Context(), id, status); err != nil {
			d.fail(w, err)
			return
		}
		d.respondMoment(w, r, id)
	}
}

func (d Deps) respondMoment(w http.ResponseWriter, r *http.Request, id string) {
	m, err := d.Store.GetMoment(r.Context(), id, true)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (d Deps) adminSetMomentMedia(w http.ResponseWriter, r *http.Request) {
	var req models.MediaListRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := d.Store.GetMoment(r.Context(), id, true)
	if err != nil {
		d.fail(w, err)
		return
	}
	if existing.Status == models.EntityStatusPublished {
		unlicensed, err := d.Store.UnlicensedMedia(r.Context(), req.MediaIDs)
		if err != nil {
			d.fail(w, err)
			return
		}
		if len(unlicensed) > 0 {
			writeError(w, http.StatusConflict, "conflict", "moment is published; these assets have no license: "+strings.Join(unlicensed, ", "))
			return
		}
	}
	if err := d.Store.ReplaceMomentMedia(r.Context(), id, req.MediaIDs); err != nil {
		d.fail(w, err)
		return
	}
	d.respondMoment(w, r, id)
}

func (d Deps) adminSetMomentSources(w http.ResponseWriter, r *http.Request) {
	var req models.SourcesRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	id := chi.URLParam(r, "id")
	if err := d.Store.ReplaceMomentSources(r.Context(), id, req.Sources); err != nil {
		d.fail(w, err)
		return
	}
	d.respondMoment(w, r, id)
}

func (d Deps) adminSetMomentTopics(w http.ResponseWriter, r *http.Request) {
	var req models.TopicsRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	id := chi.URLParam(r, "id")
	if err := d.Store.ReplaceMomentTopics(r.Context(), id, req.TopicIDs); err != nil {
		d.fail(w, err)
		return
	}
	d.respondMoment(w, r, id)
}
