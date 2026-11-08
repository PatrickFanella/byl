package httpapi

import (
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"net/http"
	"time"
)

func (d Deps) adminContributionReviews(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	q := r.URL.Query()
	f := repository.ContributionReviewFilter{RevisionID: q.Get("revision_id"), SegmentID: q.Get("segment_id"), Mode: models.TravelMode(q.Get("mode")), Direction: q.Get("direction"), Limit: queryInt(r, "limit", 50), Offset: queryOffset(r)}
	allowed := map[string]bool{"revision_id": true, "segment_id": true, "mode": true, "direction": true, "window_start": true, "limit": true, "offset": true}
	for key, values := range q {
		if !allowed[key] || len(values) != 1 {
			writeError(w, http.StatusBadRequest, "bad_request", "unexpected or repeated review query parameter")
			return
		}
	}
	if f.RevisionID == "" && (f.SegmentID != "" || f.Mode != "" || f.Direction != "" || q.Get("window_start") != "") {
		writeValidation(w, models.Fields{"revision_id": "select a complete revision, segment, mode, direction and UTC week"})
		return
	}
	if f.RevisionID != "" {
		var err error
		f.WindowStart, err = time.Parse("2006-01-02", q.Get("window_start"))
		if err != nil {
			writeValidation(w, models.Fields{"window_start": "use a UTC Monday within the current and preceding three weeks"})
			return
		}
		f.Offset = 0
		f.Limit = 1
	}
	items, err := d.Store.ListContributionReviews(r.Context(), f, time.Now())
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: items})
}
