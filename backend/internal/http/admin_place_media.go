package httpapi

import (
	"invisible-museum/backend/internal/models"
	"net/http"
	"strings"
)

func (d Deps) adminSetPlaceMedia(w http.ResponseWriter, r *http.Request) {
	var req models.MediaListRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if fields := req.Validate(); len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	id, ok := d.adminPlaceID(w, r)
	if !ok {
		return
	}
	place, err := d.Store.GetPlace(r.Context(), id, "", true)
	if err != nil {
		d.fail(w, err)
		return
	}
	blockers, err := d.Store.MediaPublishBlockers(r.Context(), req.MediaIDs)
	if err != nil {
		d.fail(w, err)
		return
	}
	if place.Status == models.EntityStatusPublished && len(blockers) > 0 {
		writeError(w, http.StatusConflict, "conflict", strings.Join(blockers, "; "))
		return
	}
	if err := d.Store.ReplacePlaceMedia(r.Context(), id, req.MediaIDs); err != nil {
		d.fail(w, err)
		return
	}
	place, err = d.Store.GetPlace(r.Context(), id, "", true)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}
