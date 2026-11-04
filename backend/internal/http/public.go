package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

type listResponse struct {
	Items      any    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func queryInt(r *http.Request, key string, def int) int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func parseLatLng(s string) (*geo.Point, bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return nil, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return nil, false
	}
	return &geo.Point{Lat: lat, Lng: lng}, true
}

func parseBBox(s string) (*geo.BBox, bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return nil, false
	}
	vals := make([]float64, 4)
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, false
		}
		vals[i] = v
	}
	b := &geo.BBox{MinLng: vals[0], MinLat: vals[1], MaxLng: vals[2], MaxLat: vals[3]}
	if b.MinLat >= b.MaxLat || b.MinLng >= b.MaxLng {
		return nil, false
	}
	return b, true
}

func (d Deps) listCities(w http.ResponseWriter, r *http.Request) {
	cities, err := d.Store.ListCities(r.Context())
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: cities})
}

func (d Deps) getCity(w http.ResponseWriter, r *http.Request) {
	city, err := d.Store.GetCity(r.Context(), chi.URLParam(r, "city"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, city)
}

// resolveTopicParam turns ?topic= (id or slug) into a topic id, or "" when absent.
func (d Deps) resolveTopicParam(r *http.Request, cityID string) (string, error) {
	ref := strings.TrimSpace(r.URL.Query().Get("topic"))
	if ref == "" {
		return "", nil
	}
	t, err := d.Store.GetTopic(r.Context(), ref, cityID)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

func (d Deps) listPlaces(w http.ResponseWriter, r *http.Request) {
	cityID, err := d.Store.ResolveCityID(r.Context(), chi.URLParam(r, "city"))
	if err != nil {
		d.fail(w, err)
		return
	}
	q := r.URL.Query()
	fields := map[string]string{}
	f := repository.PlaceFilter{CityID: cityID, Limit: queryInt(r, "limit", 50), Cursor: q.Get("cursor")}
	if near := q.Get("near"); near != "" {
		pt, ok := parseLatLng(near)
		if !ok {
			fields["near"] = "must be lat,lng"
		}
		f.Near = pt
		f.RadiusM = float64(queryInt(r, "radius_m", 2000))
		if f.RadiusM <= 0 || f.RadiusM > 50000 {
			fields["radius_m"] = "must be between 1 and 50000"
		}
	}
	if bbox := q.Get("bbox"); bbox != "" {
		b, ok := parseBBox(bbox)
		if !ok {
			fields["bbox"] = "must be minLng,minLat,maxLng,maxLat"
		}
		f.BBox = b
	}
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	if f.TopicID, err = d.resolveTopicParam(r, cityID); err != nil {
		d.fail(w, err)
		return
	}
	places, next, err := d.Store.ListPlaces(r.Context(), f)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: places, NextCursor: next})
}

func (d Deps) listTours(w http.ResponseWriter, r *http.Request) {
	cityID, err := d.Store.ResolveCityID(r.Context(), chi.URLParam(r, "city"))
	if err != nil {
		d.fail(w, err)
		return
	}
	f := repository.TourFilter{CityID: cityID, PublicOnly: true, Limit: queryInt(r, "limit", 50), Cursor: r.URL.Query().Get("cursor")}
	if mode := r.URL.Query().Get("mode"); mode != "" {
		f.Mode = models.TravelMode(mode)
		if !f.Mode.Valid() {
			writeValidation(w, map[string]string{"mode": "must be walk or bike"})
			return
		}
	}
	if f.TopicID, err = d.resolveTopicParam(r, cityID); err != nil {
		d.fail(w, err)
		return
	}
	tours, next, err := d.Store.ListTours(r.Context(), f)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: tours, NextCursor: next})
}

func (d Deps) getPlace(w http.ResponseWriter, r *http.Request) {
	place, err := d.Store.GetPlace(r.Context(), chi.URLParam(r, "place"), r.URL.Query().Get("city"), false)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}

func (d Deps) listPlaceMoments(w http.ResponseWriter, r *http.Request) {
	place, err := d.Store.GetPlace(r.Context(), chi.URLParam(r, "place"), r.URL.Query().Get("city"), false)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: place.Moments})
}

func (d Deps) getMoment(w http.ResponseWriter, r *http.Request) {
	m, err := d.Store.GetMoment(r.Context(), chi.URLParam(r, "id"), false)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (d Deps) getTopic(w http.ResponseWriter, r *http.Request) {
	t, err := d.Store.GetTopic(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("city"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// visibleTour loads a tour and enforces visibility for the current account.
func (d Deps) visibleTour(w http.ResponseWriter, r *http.Request) (*models.TourDetail, bool) {
	tour, err := d.Store.GetTour(r.Context(), chi.URLParam(r, "tour"))
	if err != nil {
		d.fail(w, err)
		return nil, false
	}
	if !canSeeTour(&tour.TourSummary, accountFrom(r.Context())) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return nil, false
	}
	return tour, true
}

func (d Deps) getTour(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.visibleTour(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, tour)
}

func (d Deps) getTourRoute(w http.ResponseWriter, r *http.Request) {
	tour, ok := d.visibleTour(w, r)
	if !ok {
		return
	}
	mode := models.TravelMode(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = models.TravelModeWalk
	}
	if !mode.Valid() {
		writeValidation(w, map[string]string{"mode": "must be walk or bike"})
		return
	}
	route, err := d.Store.GetTourRoute(r.Context(), tour.ID, mode)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, route)
}

func (d Deps) getMedia(w http.ResponseWriter, r *http.Request) {
	m, err := d.Store.GetMedia(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	if m.Status == models.EntityStatusArchived {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}
