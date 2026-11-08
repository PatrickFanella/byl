package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

func (d Deps) routeContributionIntake(w http.ResponseWriter, r *http.Request) {
	routeReportHeaders(w)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": d.contributionsAdmitted(r), "retention_days": 30, "detector_version": "uncalibrated-v1", "calibrated": false, "withdrawal_available": true})
}

func (d Deps) contributionsAdmitted(r *http.Request) bool {
	if !d.Contributions || len(d.ContributionKey) != 32 {
		return false
	}
	admitted, err := d.Store.ContributionsAdmitted(r.Context(), d.ContributionKey, time.Now())
	return err == nil && admitted
}

// Never echo unexpected field names, query strings, tokens or submitted values.
// Delete capabilities live only in bounded JSON bodies, never logged URL paths.
func decodeContribution(w http.ResponseWriter, r *http.Request, body any) bool {
	routeReportHeaders(w)
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "bad_request", "query parameters are not supported for private contribution requests")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "provide one bounded contribution object containing only the documented fields")
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "provide one bounded contribution object containing only the documented fields")
		return false
	}
	return true
}

func (d Deps) contributionOpen(w http.ResponseWriter, r *http.Request) bool {
	routeReportHeaders(w)
	if !d.contributionsAdmitted(r) {
		writeError(w, http.StatusForbidden, "contributions_disabled", "route improvement collection is disabled; navigation is unchanged")
		return false
	}
	return true
}

func contributionKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.Header.Get("Idempotency-Key")
	if !models.ValidContributionKey(key) {
		writeValidation(w, models.Fields{"idempotency_key": "provide 16 to 128 printable ASCII characters without spaces"})
		return "", false
	}
	return key, true
}

func (d Deps) contributionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrContributionsDisabled):
		writeError(w, http.StatusForbidden, "contributions_disabled", "route improvement collection is disabled; navigation is unchanged")
	case errors.Is(err, repository.ErrContributionConsent):
		writeError(w, http.StatusForbidden, "consent_required", "active consent for this retained route and mode is required; make a fresh choice after withdrawal or expiry")
	case errors.Is(err, repository.ErrContributionDuplicate):
		writeError(w, http.StatusConflict, "already_contributed", "this route-scoped owner already supplied a summary for this segment, direction and UTC day")
	case errors.Is(err, repository.ErrSubmissionLimit):
		writeError(w, http.StatusTooManyRequests, "submission_limit", "contribution limits have been reached; navigation remains available")
	default:
		d.fail(w, err)
	}
}

func (d Deps) grantRouteContributionConsent(w http.ResponseWriter, r *http.Request) {
	if !d.contributionOpen(w, r) {
		return
	}
	var in models.ContributionConsentRequest
	if !decodeContribution(w, r, &in) {
		return
	}
	in.Normalize()
	if fields := in.Validate(time.Now()); len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	key, ok := contributionKey(w, r)
	if !ok {
		return
	}
	consent, created, err := d.Store.GrantContributionConsent(r.Context(), d.ContributionKey, accountFrom(r.Context()).ID, in, key, time.Now())
	if err != nil {
		d.contributionError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, consent)
}

func (d Deps) createRouteContribution(w http.ResponseWriter, r *http.Request) {
	if !d.contributionOpen(w, r) {
		return
	}
	var in models.ContributionRequest
	if !decodeContribution(w, r, &in) {
		return
	}
	if fields := in.Summary.Validate(time.Now()); len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	key, ok := contributionKey(w, r)
	if !ok {
		return
	}
	receipt, created, err := d.Store.CreateRouteContribution(r.Context(), d.ContributionKey, accountFrom(r.Context()).ID, in, key, time.Now())
	if err != nil {
		d.contributionError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, receipt)
}

// The scoped capability permits erasure after sign-out and while collection is off.
func (d Deps) withdrawRouteContributions(w http.ResponseWriter, r *http.Request) {
	var in models.ContributionWithdrawal
	if !decodeContribution(w, r, &in) {
		return
	}
	if err := d.Store.WithdrawRouteContributions(r.Context(), in, time.Now()); err != nil {
		d.contributionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
