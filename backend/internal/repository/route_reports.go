package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

const routeReportCols = `id,owner_account_id,submitted,status,reason,version,created_at,updated_at`

func scanRouteReport(row interface{ Scan(...any) error }) (*models.RouteReport, error) {
	r := &models.RouteReport{}
	var raw []byte
	if err := row.Scan(&r.ID, &r.OwnerAccountID, &raw, &r.Status, &r.Reason, &r.Version, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(raw, &r.Submitted); err != nil {
		return nil, err
	}
	return r, nil
}

// CreateRouteReport serializes retries/quotas without replacing the trip revision
// with today's draft. The stored observation is never used as public content.
func (s *Store) CreateRouteReport(ctx context.Context, owner string, in models.RouteReportInput, key string) (*models.RouteReport, bool, error) {
	in.Normalize()
	if len(in.Validate()) > 0 || len(key) > 128 {
		return nil, false, ErrInvalidReference
	}
	if !isUUID(in.TourID) || !isUUID(in.RevisionID) {
		return nil, false, ErrNotFound
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, false, err
	}
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	var out *models.RouteReport
	created := false
	err = s.Transaction(ctx, func(tx *Store) error {
		if _, err := tx.pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,2222))`, owner); err != nil {
			return err
		}
		var role models.AccountRole
		if err := tx.pool.QueryRow(ctx, `SELECT role FROM accounts WHERE id=$1 FOR SHARE`, owner).Scan(&role); err != nil {
			return mapErr(err)
		}
		if key != "" {
			var id, previous string
			err := tx.pool.QueryRow(ctx, `SELECT id,submission_hash FROM route_reports WHERE owner_account_id=$1 AND submission_key=$2`, owner, key).Scan(&id, &previous)
			if err == nil {
				if previous != digest {
					return fmt.Errorf("%w: idempotency key was used for a different report", ErrConflict)
				}
				out, err = tx.GetRouteReport(ctx, id, owner)
				return err
			}
			if !errors.Is(mapErr(err), ErrNotFound) {
				return err
			}
		}
		var active, recent int
		if err := tx.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status NOT IN ('accepted','declined')),count(*) FILTER(WHERE created_at>now()-interval '24 hours') FROM route_reports WHERE owner_account_id=$1`, owner).Scan(&active, &recent); err != nil {
			return err
		}
		if active >= 20 || recent >= 10 {
			return ErrSubmissionLimit
		}
		// Hold visibility/deletion stable through insertion, while permitting
		// independent readers and publisher account FK checks.
		var id string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR SHARE`, in.TourID).Scan(&id); err != nil {
			return mapErr(err)
		}
		var tourOwner *string
		var visibility models.TourVisibility
		var status models.EntityStatus
		if err := tx.pool.QueryRow(ctx, `SELECT t.owner_account_id,t.visibility,t.status FROM tours t JOIN tour_revision_state st ON st.tour_id=t.id WHERE t.id=$1 AND st.deleted_at IS NULL`, in.TourID).Scan(&tourOwner, &visibility, &status); err != nil {
			return mapErr(err)
		}
		isOwner := tourOwner != nil && *tourOwner == owner
		if !isOwner && (visibility == models.TourVisibilityPrivate || (status != models.EntityStatusPublished && !role.AtLeast(models.AccountRoleCurator))) {
			return ErrNotFound
		}
		revision, err := tx.GetTourRevision(ctx, in.TourID, in.RevisionID)
		if err != nil {
			return err
		}
		if !isOwner && revision.Snapshot.Tour.Visibility == models.TourVisibilityPrivate {
			return ErrNotFound
		}
		if err := validateReportLocation(revision, in); err != nil {
			return err
		}
		var submissionKey any
		if key != "" {
			submissionKey = key
		}
		out, err = scanRouteReport(tx.pool.QueryRow(ctx, `INSERT INTO route_reports(owner_account_id,tour_id,revision_id,submitted,submission_key,submission_hash) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+routeReportCols, owner, in.TourID, in.RevisionID, raw, submissionKey, digest))
		created = err == nil
		return err
	})
	return out, created, err
}

// GetRouteReport keeps the owner predicate in SQL. Only curator transports omit it.
func (s *Store) GetRouteReport(ctx context.Context, id, owner string) (*models.RouteReport, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanRouteReport(s.pool.QueryRow(ctx, `SELECT `+routeReportCols+` FROM route_reports WHERE id=$1 AND ($2='' OR owner_account_id::text=$2)`, id, owner))
}

// ListRouteReports returns private observations with bounded offset pagination.
func (s *Store) ListRouteReports(ctx context.Context, owner string, status models.BriefStatus, limit, offset int) ([]models.RouteReport, error) {
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT `+routeReportCols+` FROM route_reports WHERE ($1='' OR owner_account_id::text=$1) AND ($2='' OR status=$2) ORDER BY created_at DESC,id LIMIT $3 OFFSET $4`, owner, status, clamp(limit), offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RouteReport{}
	for rows.Next() {
		r, err := scanRouteReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// SetRouteReportState records a private disposition; it edits no tour or story.
func (s *Store) SetRouteReportState(ctx context.Context, id string, in models.BriefStateRequest) (*models.RouteReport, error) {
	if len(in.Validate()) > 0 {
		return nil, ErrInvalidReference
	}
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	var out *models.RouteReport
	err := s.Transaction(ctx, func(tx *Store) error {
		r, err := scanRouteReport(tx.pool.QueryRow(ctx, `SELECT `+routeReportCols+` FROM route_reports WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if r.Version != in.Version || !r.Status.CanTransitionTo(in.Status) {
			return fmt.Errorf("%w: report changed or disposition transition is invalid", ErrConflict)
		}
		out, err = scanRouteReport(tx.pool.QueryRow(ctx, `UPDATE route_reports SET status=$2,reason=$3,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+routeReportCols, id, in.Status, in.Reason))
		return err
	})
	return out, err
}

func validateReportLocation(revision *models.TourRevision, in models.RouteReportInput) error {
	bad := fmt.Errorf("%w: affected location or annotation does not match this revision", ErrInvalidReference)
	if !slices.Contains(revision.Snapshot.Tour.Modes, in.Mode) || revision.Snapshot.Routes[in.Mode].Status != models.RouteStatusReady {
		return bad
	}
	stops := map[string]models.TourStop{}
	for _, stop := range revision.Snapshot.Tour.Stops {
		stops[stop.ID] = stop
	}
	var path [][2]float64
	a := in.Affected
	if a.Kind == "segment" {
		from, fromOK := stops[a.FromStopID]
		to, toOK := stops[a.ToStopID]
		delta := to.Position - from.Position
		if !fromOK || !toOK || (delta != 1 && delta != -1) || (in.Direction == "forward" && delta != 1) || (in.Direction == "reverse" && delta != -1) {
			return bad
		}
		for _, leg := range revision.Snapshot.Routes[in.Mode].Legs {
			if leg.FromPosition == min(from.Position, to.Position) && leg.ToPosition == max(from.Position, to.Position) {
				path = slices.Clone(leg.Geometry)
				break
			}
		}
		if len(path) < 2 {
			path = [][2]float64{{from.Place.Longitude, from.Place.Latitude}, {to.Place.Longitude, to.Place.Latitude}}
		} else if delta == -1 {
			slices.Reverse(path)
		}
	} else {
		stop, ok := stops[a.StopID]
		if !ok {
			return bad
		}
		point := [2]float64{stop.Place.Longitude, stop.Place.Latitude}
		if a.Kind == "entrance" && stop.Place.ArrivalPoint != nil {
			arrival := stop.Place.ArrivalPoint
			if arrival.Point.Latitude == nil || arrival.Point.Longitude == nil {
				return bad
			}
			if a.ArrivalID != "" && a.ArrivalID != arrival.ID {
				return bad
			}
			point = [2]float64{*arrival.Point.Longitude, *arrival.Point.Latitude}
		} else if a.ArrivalID != "" {
			return bad
		}
		path = [][2]float64{point}
	}
	// One point marks the affected area; it cannot encode a background trip trace.
	// Alternatives stay within 1 km of that area and connect to its anchors.
	if in.Annotation != nil && !nearReportPath([2]float64{*in.Annotation.Lng, *in.Annotation.Lat}, path, 1000) {
		return bad
	}
	if in.Alternative != nil {
		points := in.Alternative.Coordinates
		for _, p := range points {
			if !nearReportPath([2]float64{p[0], p[1]}, path, 1000) {
				return bad
			}
		}
		first := [2]float64{points[0][0], points[0][1]}
		last := [2]float64{points[len(points)-1][0], points[len(points)-1][1]}
		near := func(a, b [2]float64) bool { return geo.DistanceMeters(a[1], a[0], b[1], b[0]) <= 100 }
		if a.Kind == "segment" {
			if !near(first, path[0]) || !near(last, path[len(path)-1]) {
				return bad
			}
		} else if !near(first, path[0]) && !near(last, path[0]) {
			return bad
		}
	}
	return nil
}

// nearReportPath uses a local projection only to bound unverified annotations.
// It does not assert traversability or create connections at visual crossings.
func nearReportPath(point [2]float64, path [][2]float64, radius float64) bool {
	for i, a := range path {
		if geo.DistanceMeters(point[1], point[0], a[1], a[0]) <= radius {
			return true
		}
		if i == 0 {
			continue
		}
		b := path[i-1]
		x := math.Remainder(a[0]-b[0], 360) * math.Cos((a[1]+b[1])*math.Pi/360)
		y := a[1] - b[1]
		px := math.Remainder(point[0]-b[0], 360) * math.Cos((a[1]+b[1])*math.Pi/360)
		py := point[1] - b[1]
		if x*x+y*y == 0 {
			continue
		}
		t := max(0, min(1, (px*x+py*y)/(x*x+y*y)))
		lng := math.Remainder(b[0]+t*math.Remainder(a[0]-b[0], 360), 360)
		lat := b[1] + t*y
		if geo.DistanceMeters(point[1], point[0], lat, lng) <= radius {
			return true
		}
	}
	return false
}
