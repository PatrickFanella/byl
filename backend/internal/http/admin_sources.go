package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func (d Deps) adminListSources(w http.ResponseWriter, r *http.Request) {
	if d.Sources == nil {
		writeJSON(w, http.StatusOK, listResponse{Items: []any{}})
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: d.Sources.List()})
}

func (d Deps) adminCreateImport(w http.ResponseWriter, r *http.Request) {
	var req models.ImportRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	fields := map[string]string{}
	if strings.TrimSpace(req.Source) == "" {
		fields["source"] = "is required"
	} else if d.Sources == nil {
		fields["source"] = "no sources are configured"
	} else if _, ok := d.Sources.Get(req.Source); !ok {
		fields["source"] = "unknown source"
	}
	if strings.TrimSpace(req.City) == "" {
		fields["city"] = "is required"
	}
	if req.Limit < 0 || req.Limit > 100 {
		fields["limit"] = "must be between 0 and 100"
	}
	if req.RadiusM < 0 || req.RadiusM > 20000 {
		fields["radius_m"] = "must be between 0 and 20000"
	}
	payload := models.ImportSourcePayload{Source: req.Source, RadiusM: req.RadiusM, Query: strings.TrimSpace(req.Query), Limit: req.Limit}
	if req.Near != "" {
		pt, ok := parseLatLng(req.Near)
		if !ok {
			fields["near"] = "must be lat,lng"
		} else {
			payload.Lat, payload.Lng = &pt.Lat, &pt.Lng
		}
	}
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	cityID, err := d.Store.ResolveCityID(r.Context(), req.City)
	if err != nil {
		d.fail(w, err)
		return
	}
	payload.CityID = cityID
	if req.Place != "" {
		placeID, err := d.Store.ResolvePlaceID(r.Context(), req.Place, req.City)
		if err != nil {
			d.fail(w, err)
			return
		}
		payload.PlaceID = &placeID
	}
	job, err := d.Store.EnqueueReturning(r.Context(), models.JobKindImportSource, payload)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (d Deps) adminDraftCandidate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !d.drafterAvailable() {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "no LLM provider is configured (set LLM_PROVIDER)")
		return
	}
	c, err := d.Store.GetCandidate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	if c.Kind != models.CandidateKindMoment {
		writeError(w, http.StatusConflict, "conflict", "only moment candidates can be drafted")
		return
	}
	if c.Status != models.CandidateStatusPending {
		writeError(w, http.StatusConflict, "conflict", "candidate is already reviewed")
		return
	}
	var in models.DraftCandidateRequest
	if !decodeOrFail(w, r, &in) {
		return
	}
	if _, err := d.Store.DraftEvidence(r.Context(), in.EvidenceIDs); err != nil {
		d.fail(w, err)
		return
	}
	job, err := d.Store.EnqueueReturning(r.Context(), models.JobKindDraftCandidate, models.DraftCandidatePayload{CandidateID: c.ID, EvidenceIDs: in.EvidenceIDs, PayloadSHA256: repository.CandidatePayloadHash(c.Payload)})
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

type suggestion struct {
	Moment models.MomentSummary `json:"moment"`
	Reason string               `json:"reason"`
}

// adminSuggestFeatured ranks a stop's published moments against the tour's topic.
func (d Deps) adminSuggestFeatured(w http.ResponseWriter, r *http.Request) {
	if !d.drafterAvailable() {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "no LLM provider is configured (set LLM_PROVIDER)")
		return
	}
	tour, ok := d.adminTour(w, r)
	if !ok {
		return
	}
	position, err := strconv.Atoi(chi.URLParam(r, "position"))
	if err != nil || position < 1 || position > len(tour.Stops) {
		writeError(w, http.StatusNotFound, "not_found", "no stop at that position")
		return
	}
	if tour.Topic == nil {
		writeError(w, http.StatusConflict, "conflict", "tour has no topic to rank against")
		return
	}
	stop := tour.Stops[position-1]
	moments, err := d.Store.ListMoments(r.Context(), stop.Place.ID, false)
	if err != nil {
		d.fail(w, err)
		return
	}
	if len(moments) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ordered": []suggestion{}})
		return
	}
	req := llm.RankRequest{TopicName: tour.Topic.Name, TopicDescription: tour.Topic.Description, TourTitle: tour.Title, TourSummary: tour.Summary, PlaceName: stop.Place.Name}
	byID := map[string]models.MomentSummary{}
	for _, m := range moments {
		byID[m.ID] = m
		req.Moments = append(req.Moments, llm.MomentOption{ID: m.ID, Title: m.Title, Excerpt: m.Excerpt, DateDisplay: m.DateDisplay})
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	ranking, err := d.Drafter.RankMoments(ctx, req)
	if err != nil {
		d.logger().Error("rank moments", "error", err)
		writeError(w, http.StatusBadGateway, "unavailable", "the LLM did not return a ranking")
		return
	}
	out := make([]suggestion, 0, len(moments))
	seen := map[string]bool{}
	for _, rm := range ranking.Ordered {
		if m, ok := byID[rm.ID]; ok && !seen[rm.ID] {
			seen[rm.ID] = true
			out = append(out, suggestion{Moment: m, Reason: rm.Reason})
		}
	}
	for _, m := range moments {
		if !seen[m.ID] {
			out = append(out, suggestion{Moment: m, Reason: "not ranked by the model"})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ordered": out})
}

func (d Deps) adminStats(w http.ResponseWriter, r *http.Request) {
	st, err := d.Store.Stats(r.Context())
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
