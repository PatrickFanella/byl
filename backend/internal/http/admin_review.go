package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func (d Deps) adminListCandidates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repository.CandidateFilter{Status: models.CandidateStatus(q.Get("status")), Kind: models.CandidateKind(q.Get("kind")), Limit: queryInt(r, "limit", 50), Cursor: q.Get("cursor")}
	fields := map[string]string{}
	if f.Status != "" && !f.Status.Valid() {
		fields["status"] = "must be pending, approved, or rejected"
	}
	if f.Kind != "" && !f.Kind.Valid() {
		fields["kind"] = "must be place, moment, or media"
	}
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	cands, next, err := d.Store.ListCandidates(r.Context(), f)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: cands, NextCursor: next})
}

func (d Deps) adminGetCandidate(w http.ResponseWriter, r *http.Request) {
	c, err := d.Store.GetCandidate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (d Deps) adminCreateCandidate(w http.ResponseWriter, r *http.Request) {
	var req models.CreateCandidateRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if f := req.Validate(); len(f) > 0 {
		writeValidation(w, f)
		return
	}
	a := accountFrom(r.Context())
	c, err := d.Store.CreateCandidate(r.Context(), models.Candidate{
		Kind: req.Kind, Origin: req.Origin, SourceName: req.SourceName, ExternalID: req.ExternalID,
		SubmittedByAccountID: &a.ID, TargetPlaceID: req.TargetPlaceID, TargetMomentID: req.TargetMomentID, Payload: req.Payload,
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// mergePayload overlays override's top-level keys onto base.
func mergePayload(base, override json.RawMessage) (json.RawMessage, error) {
	merged := map[string]json.RawMessage{}
	if len(base) > 0 {
		if err := json.Unmarshal(base, &merged); err != nil {
			return nil, err
		}
	}
	if len(override) > 0 {
		var over map[string]json.RawMessage
		if err := json.Unmarshal(override, &over); err != nil {
			return nil, err
		}
		for k, v := range over {
			merged[k] = v
		}
	}
	return json.Marshal(merged)
}

func (d Deps) adminApproveCandidate(w http.ResponseWriter, r *http.Request) {
	c, err := d.Store.GetCandidate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	var req models.ReviewRequest
	if r.ContentLength != 0 {
		if !decodeOrFail(w, r, &req) {
			return
		}
	}
	if c.Status != models.CandidateStatusPending {
		writeError(w, http.StatusConflict, "conflict", "candidate already reviewed")
		return
	}
	payload, err := mergePayload(c.Payload, req.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "payload must be a JSON object")
		return
	}
	entityID, fields, err := d.createFromCandidate(r, c, payload)
	if len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	if err != nil {
		d.fail(w, err)
		return
	}
	reviewed, err := d.Store.ReviewCandidate(r.Context(), c.ID, accountFrom(r.Context()).ID, models.CandidateStatusApproved, req.Note, &entityID)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reviewed)
}

// createFromCandidate materialises the candidate's payload as a draft entity.
func (d Deps) createFromCandidate(r *http.Request, c *models.Candidate, payload json.RawMessage) (string, map[string]string, error) {
	switch c.Kind {
	case models.CandidateKindPlace:
		var req models.CreatePlaceRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return "", map[string]string{"payload": "does not match a place"}, nil
		}
		if f := req.Validate(); len(f) > 0 {
			return "", f, nil
		}
		p, err := d.Store.CreatePlace(r.Context(), req)
		if err != nil {
			return "", nil, err
		}
		return p.ID, nil, nil
	case models.CandidateKindMoment:
		var req models.CreateMomentRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return "", map[string]string{"payload": "does not match a moment"}, nil
		}
		if req.PlaceID == "" && c.TargetPlaceID != nil {
			req.PlaceID = *c.TargetPlaceID
		}
		if f := req.Validate(); len(f) > 0 {
			return "", f, nil
		}
		m, err := d.Store.CreateMoment(r.Context(), req)
		if err != nil {
			return "", nil, err
		}
		return m.ID, nil, nil
	case models.CandidateKindMedia:
		var asset models.MediaAsset
		if err := json.Unmarshal(payload, &asset); err != nil || !asset.Kind.Valid() {
			return "", map[string]string{"payload": "does not match a media asset with a valid kind"}, nil
		}
		asset.ID = ""
		asset.Status = models.EntityStatusDraft
		m, err := d.Store.CreateMedia(r.Context(), asset)
		if err != nil {
			return "", nil, err
		}
		if c.TargetMomentID != nil {
			existing, err := d.Store.GetMoment(r.Context(), *c.TargetMomentID, true)
			if err == nil {
				ids := make([]string, 0, len(existing.Media)+1)
				for _, em := range existing.Media {
					ids = append(ids, em.ID)
				}
				_ = d.Store.ReplaceMomentMedia(r.Context(), existing.ID, append(ids, m.ID))
			}
		}
		return m.ID, nil, nil
	}
	return "", map[string]string{"kind": "unsupported"}, nil
}

func (d Deps) adminRejectCandidate(w http.ResponseWriter, r *http.Request) {
	var req models.ReviewRequest
	if r.ContentLength != 0 {
		if !decodeOrFail(w, r, &req) {
			return
		}
	}
	reviewed, err := d.Store.ReviewCandidate(r.Context(), chi.URLParam(r, "id"), accountFrom(r.Context()).ID, models.CandidateStatusRejected, req.Note, nil)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reviewed)
}
