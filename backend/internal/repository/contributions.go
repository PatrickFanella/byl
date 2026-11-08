package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"invisible-museum/backend/internal/models"
)

var (
	ErrContributionsDisabled = errors.New("contribution custody has not been admitted")
	ErrContributionConsent   = errors.New("active contribution consent required")
	ErrContributionDuplicate = errors.New("this scope already supplied a segment summary for that UTC day")
)

// ContributionsAdmitted is deliberately checked in the database rather than
// cached in process configuration, so a restored database defaults to closed.
func (s *Store) ContributionsAdmitted(ctx context.Context, secret []byte, now time.Time) (bool, error) {
	if len(secret) != 32 {
		return false, nil
	}
	var admitted bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM route_contribution_custody WHERE policy_version=1 AND key_fingerprint=$1 AND admitted_on<=$2)`, contributionMAC(secret, "route-contribution-custody-v1"), models.ContributionDay(now).AddDate(0, 0, -2)).Scan(&admitted)
	return admitted, err
}

// AdmitContributionCustody is an operator action, never called by API startup.
// The two-day hold ages out pre-restore consent requests. Repeating admission for
// the same key retains its original date; rotating keys requires draining data.
func (s *Store) AdmitContributionCustody(ctx context.Context, secret []byte, now time.Time) error {
	if len(secret) != 32 {
		return ErrInvalidReference
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(232319,1)`); err != nil {
			return err
		}
		fingerprint := contributionMAC(secret, "route-contribution-custody-v1")
		var matches bool
		if err := tx.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM route_contribution_custody WHERE policy_version=1 AND key_fingerprint=$1)`, fingerprint).Scan(&matches); err != nil {
			return err
		}
		if matches {
			return nil
		}
		var remaining bool
		if err := tx.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM route_contribution_consents WHERE expires_on>$1)`, models.ContributionDay(now)).Scan(&remaining); err != nil {
			return err
		}
		if remaining {
			return ErrConflict
		}
		_, err := tx.pool.Exec(ctx, `INSERT INTO route_contribution_custody(singleton,policy_version,key_fingerprint,admitted_on) VALUES(true,1,$1,$2) ON CONFLICT(singleton) DO UPDATE SET policy_version=1,key_fingerprint=excluded.key_fingerprint,admitted_on=excluded.admitted_on`, fingerprint, models.ContributionDay(now))
		return err
	})
}

func (s *Store) lockContributionCustody(ctx context.Context, secret []byte, now time.Time) error {
	if _, err := s.pool.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(232319,1)`); err != nil {
		return err
	}
	admitted, err := s.ContributionsAdmitted(ctx, secret, now)
	if err != nil {
		return err
	}
	if !admitted {
		return ErrContributionsDisabled
	}
	return nil
}

func contributionMAC(secret []byte, domain string, parts ...string) string {
	mac := hmac.New(sha256.New, secret)
	raw, _ := json.Marshal(append([]string{domain}, parts...))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

func contributionHash(value any) string {
	raw, _ := json.Marshal(value)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func contributionScope(secret []byte, actor, tour, revision string, mode models.TravelMode) string {
	return contributionMAC(secret, "route-contribution-owner-v1", actor, tour, revision, string(mode))
}

func (s *Store) contributionLimit(ctx context.Context, secret []byte, actor, kind string, today time.Time, limit int) error {
	key := contributionMAC(secret, "route-contribution-limit-v1", actor, kind, today.Format("2006-01-02"))
	var count int
	err := s.pool.QueryRow(ctx, `INSERT INTO route_contribution_limits(scope,count,expires_on) VALUES($1,1,$2) ON CONFLICT(scope) DO UPDATE SET count=route_contribution_limits.count+1 WHERE route_contribution_limits.count<$3 RETURNING count`, key, today.AddDate(0, 0, 2), limit).Scan(&count)
	if errors.Is(mapErr(err), ErrNotFound) {
		return ErrSubmissionLimit
	}
	return err
}

// contributionRevision checks current visibility without storing an account FK.
// Eligibility and logical segment identities come from the immutable snapshot.
func (s *Store) contributionRevision(ctx context.Context, actor, tour, revision string, mode models.TravelMode) (*models.TourRevision, error) {
	if !isUUID(actor) || !isUUID(tour) || !isUUID(revision) {
		return nil, ErrNotFound
	}
	var owner *string
	var visibility models.TourVisibility
	var status models.EntityStatus
	err := s.pool.QueryRow(ctx, `SELECT t.owner_account_id,t.visibility,t.status FROM tours t JOIN tour_revision_state st ON st.tour_id=t.id WHERE t.id=$1 AND st.deleted_at IS NULL FOR SHARE OF t`, tour).Scan(&owner, &visibility, &status)
	if err != nil {
		return nil, mapErr(err)
	}
	if (owner == nil || *owner != actor) && (visibility == models.TourVisibilityPrivate || status != models.EntityStatusPublished) {
		return nil, ErrNotFound
	}
	r, err := s.GetTourRevision(ctx, tour, revision)
	if err != nil {
		return nil, err
	}
	if (owner == nil || *owner != actor) && r.Snapshot.Tour.Visibility == models.TourVisibilityPrivate {
		return nil, ErrNotFound
	}
	route, ok := r.Snapshot.Routes[mode]
	if !ok || route.Journey == nil || !route.Journey.Usable || len(route.Journey.Segments) == 0 {
		return nil, ErrInvalidReference
	}
	return r, nil
}

// GrantContributionConsent retains no account identifier or cross-route key.
// The delete capability is recoverable on exact retries, never stored in clear.
func (s *Store) GrantContributionConsent(ctx context.Context, secret []byte, actor string, in models.ContributionConsentRequest, key string, now time.Time) (*models.ContributionConsent, bool, error) {
	in.Normalize()
	if len(secret) != 32 || !models.ValidContributionKey(key) || len(in.Validate(now)) > 0 {
		return nil, false, ErrInvalidReference
	}
	today := models.ContributionDay(now)
	scope := contributionScope(secret, actor, in.TourID, in.RevisionID, in.Mode)
	keyHash, requestHash := contributionHash(key), contributionHash(in)
	var consent *models.ContributionConsent
	created := false
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockContributionCustody(ctx, secret, now); err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,2323))`, scope); err != nil {
			return err
		}
		var id, previous string
		var expires time.Time
		var revoked *time.Time
		err := tx.pool.QueryRow(ctx, `SELECT id,request_hash,expires_on,revoked_on FROM route_contribution_consents WHERE owner_scope=$1 AND grant_key_hash=$2 FOR UPDATE`, scope, keyHash).Scan(&id, &previous, &expires, &revoked)
		if err == nil {
			if previous != requestHash {
				return ErrConflict
			}
			if revoked != nil || !expires.After(today) {
				return ErrContributionConsent
			}
		} else if !errors.Is(mapErr(err), ErrNotFound) {
			return err
		} else {
			if in.RequestedOn.Before(today.AddDate(0, 0, -1)) {
				return ErrContributionConsent
			}
			if _, err = tx.contributionRevision(ctx, actor, in.TourID, in.RevisionID, in.Mode); err != nil {
				return err
			}
			var account string
			if err = tx.pool.QueryRow(ctx, `SELECT id FROM accounts WHERE id=$1 FOR SHARE`, actor).Scan(&account); err != nil {
				return mapErr(err)
			}
			if err = tx.contributionLimit(ctx, secret, actor, "consent", today, 10); err != nil {
				return err
			}
			expires = today.AddDate(0, 0, 30)
			if err = tx.pool.QueryRow(ctx, `INSERT INTO route_contribution_consents(owner_scope,tour_id,revision_id,mode,grant_key_hash,request_hash,deletion_hash,requested_on,created_on,expires_on) VALUES($1,$2,$3,$4,$5,$6,repeat('0',64),$7,$8,$9) RETURNING id`, scope, in.TourID, in.RevisionID, in.Mode, keyHash, requestHash, in.RequestedOn.Time, today, expires).Scan(&id); err != nil {
				return err
			}
			created = true
		}
		handle := contributionMAC(secret, "route-contribution-delete-v1", id)
		if created {
			if _, err := tx.pool.Exec(ctx, `UPDATE route_contribution_consents SET deletion_hash=$2 WHERE id=$1`, id, contributionHash(handle)); err != nil {
				return err
			}
		}
		consent = &models.ContributionConsent{ID: id, DeletionHandle: handle, ExpiresOn: models.Date{Time: expires}}
		return nil
	})
	return consent, created, err
}

func contributionSegment(revision *models.TourRevision, in models.ContributionSummary) bool {
	route := revision.Snapshot.Routes[in.Mode]
	if route.Journey == nil || !route.Journey.Usable {
		return false
	}
	for _, segment := range route.Journey.Segments {
		if segment.ID == in.SegmentID && segment.Access == "allowed" && segment.Reverse == (in.Direction == "reverse") {
			return true
		}
	}
	return false
}

// CreateRouteContribution permits one summary per route-scoped owner, logical
// segment, direction and UTC day. An exact key replay consumes no new quota.
func (s *Store) CreateRouteContribution(ctx context.Context, secret []byte, actor string, in models.ContributionRequest, key string, now time.Time) (*models.ContributionReceipt, bool, error) {
	if len(secret) != 32 || !isUUID(actor) || !isUUID(in.ConsentID) || !models.ValidContributionKey(key) || len(in.Summary.Validate(now)) > 0 {
		return nil, false, ErrInvalidReference
	}
	today := models.ContributionDay(now)
	var receipt *models.ContributionReceipt
	created := false
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockContributionCustody(ctx, secret, now); err != nil {
			return err
		}
		var scope, tour, revision string
		var mode models.TravelMode
		var createdOn, expires time.Time
		var revoked *time.Time
		// Read scope first, then take the same lock used by grant/withdrawal.
		if err := tx.pool.QueryRow(ctx, `SELECT owner_scope FROM route_contribution_consents WHERE id=$1`, in.ConsentID).Scan(&scope); err != nil {
			return mapErr(err)
		}
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,2323))`, scope); err != nil {
			return err
		}
		if err := tx.pool.QueryRow(ctx, `SELECT owner_scope,tour_id,revision_id,mode,created_on,expires_on,revoked_on FROM route_contribution_consents WHERE id=$1 FOR UPDATE`, in.ConsentID).Scan(&scope, &tour, &revision, &mode, &createdOn, &expires, &revoked); err != nil {
			return mapErr(err)
		}
		if !hmac.Equal([]byte(scope), []byte(contributionScope(secret, actor, tour, revision, mode))) {
			return ErrNotFound
		}
		if revoked != nil || !expires.After(today) || in.Summary.ObservationPeriod.Before(createdOn) {
			return ErrContributionConsent
		}
		if in.Summary.RevisionID != revision || in.Summary.Mode != mode {
			return ErrInvalidReference
		}
		keyHash, requestHash := contributionHash(key), contributionHash(in.Summary)
		var id, previous string
		var retainedExpiry time.Time
		err := tx.pool.QueryRow(ctx, `SELECT id,request_hash,expires_on FROM route_contributions WHERE owner_scope=$1 AND submission_key_hash=$2`, scope, keyHash).Scan(&id, &previous, &retainedExpiry)
		if err == nil {
			if !retainedExpiry.After(today) {
				return ErrConflict
			}
			if previous != requestHash {
				return ErrConflict
			}
			receipt = &models.ContributionReceipt{ID: id, ExpiresOn: models.Date{Time: retainedExpiry}}
			return nil
		} else if !errors.Is(mapErr(err), ErrNotFound) {
			return err
		}
		var duplicate bool
		if err = tx.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM route_contributions WHERE owner_scope=$1 AND segment_id=$2 AND direction=$3 AND observation_period=$4)`, scope, in.Summary.SegmentID, in.Summary.Direction, in.Summary.ObservationPeriod.Time).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate {
			return ErrContributionDuplicate
		}
		r, err := tx.contributionRevision(ctx, actor, tour, revision, mode)
		if err != nil {
			return err
		}
		if !contributionSegment(r, in.Summary) {
			return ErrInvalidReference
		}
		if err = tx.contributionLimit(ctx, secret, actor, "summary", today, 50); err != nil {
			return err
		}
		if periodExpiry := in.Summary.ObservationPeriod.AddDate(0, 0, 30); periodExpiry.Before(expires) {
			expires = periodExpiry
		}
		if err = tx.pool.QueryRow(ctx, `INSERT INTO route_contributions(consent_id,owner_scope,segment_id,direction,observation_period,schema_version,detector_version,calibrated,qualified_passages,sustained_departure_rejoins,submission_key_hash,request_hash,expires_on) VALUES($1,$2,$3,$4,$5,1,'uncalibrated-v1',false,1,$6,$7,$8,$9) RETURNING id`, in.ConsentID, scope, in.Summary.SegmentID, in.Summary.Direction, in.Summary.ObservationPeriod.Time, in.Summary.SustainedDepartureRejoins, keyHash, requestHash, expires).Scan(&id); err != nil {
			return err
		}
		receipt = &models.ContributionReceipt{ID: id, ExpiresOn: models.Date{Time: expires}}
		created = true
		return nil
	})
	return receipt, created, err
}

// WithdrawRouteContributions consumes a scoped deletion capability even after
// sign-out or collection shutdown. A replay never revokes a later fresh grant.
func (s *Store) WithdrawRouteContributions(ctx context.Context, in models.ContributionWithdrawal, now time.Time) error {
	if !isUUID(in.ConsentID) || len(in.DeletionHandle) != 64 {
		return ErrNotFound
	}
	return s.Transaction(ctx, func(tx *Store) error {
		var scope string
		if err := tx.pool.QueryRow(ctx, `SELECT owner_scope FROM route_contribution_consents WHERE id=$1`, in.ConsentID).Scan(&scope); err != nil {
			return mapErr(err)
		}
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,2323))`, scope); err != nil {
			return err
		}
		var expected string
		var revoked *time.Time
		if err := tx.pool.QueryRow(ctx, `SELECT deletion_hash,revoked_on FROM route_contribution_consents WHERE id=$1 FOR UPDATE`, in.ConsentID).Scan(&expected, &revoked); err != nil {
			return mapErr(err)
		}
		if !hmac.Equal([]byte(expected), []byte(contributionHash(in.DeletionHandle))) {
			return ErrNotFound
		}
		if revoked != nil {
			return nil
		}
		// Lock every currently live grant before touching child rows, matching
		// parent-first expiry cleanup. Same-scope grants are serialized above.
		if _, err := tx.pool.Exec(ctx, `SELECT id FROM route_contribution_consents WHERE owner_scope=$1 ORDER BY id FOR UPDATE`, scope); err != nil {
			return err
		}
		if _, err := tx.pool.Exec(ctx, `DELETE FROM route_contributions WHERE owner_scope=$1`, scope); err != nil {
			return err
		}
		_, err := tx.pool.Exec(ctx, `UPDATE route_contribution_consents SET revoked_on=$2 WHERE owner_scope=$1 AND revoked_on IS NULL`, scope, models.ContributionDay(now))
		return err
	})
}

// PurgeRouteContributions is also callable by the standalone retention command.
// It runs regardless of the collection gate; the counts view filters expiry even
// if a stopped process delays physical deletion until the next cleanup.
func (s *Store) PurgeRouteContributions(ctx context.Context, now time.Time) (int64, error) {
	today := models.ContributionDay(now)
	var count int64
	err := s.Transaction(ctx, func(tx *Store) error {
		for _, query := range []string{
			`WITH expired AS (SELECT id FROM route_contribution_consents WHERE expires_on<=$1 ORDER BY id FOR UPDATE SKIP LOCKED) DELETE FROM route_contribution_consents c USING expired e WHERE c.id=e.id`,
			`WITH expired AS (SELECT id FROM route_contributions WHERE expires_on<=$1 ORDER BY id FOR UPDATE SKIP LOCKED) DELETE FROM route_contributions c USING expired e WHERE c.id=e.id`,
			`WITH expired AS (SELECT scope FROM route_contribution_limits WHERE expires_on<=$1 ORDER BY scope FOR UPDATE SKIP LOCKED) DELETE FROM route_contribution_limits c USING expired e WHERE c.scope=e.scope`,
		} {
			r, err := tx.pool.Exec(ctx, query, today)
			if err != nil {
				return err
			}
			count += r.RowsAffected()
		}
		return nil
	})
	return count, err
}
