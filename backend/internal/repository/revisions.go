package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

// ErrRouteSuperseded fences route results after stop/mode/recompute inputs change.
var ErrRouteSuperseded = errors.New("route inputs superseded")

// TourNavigationInputs captures approved inputs and their fence in one database
// statement. Separate reads could pair an old arrival with a new input hash.
func (s *Store) TourNavigationInputs(ctx context.Context, tourID string, mode models.TravelMode, version int64) (*models.TourDetail, string, error) {
	var raw []byte
	var digest string
	err := s.pool.QueryRow(ctx, `SELECT byl_tour_snapshot(t.id)->'tour',byl_route_input_hash(t.id,$3) FROM tours t JOIN tour_revision_state st ON st.tour_id=t.id WHERE t.id=$1 AND st.route_version=$2 AND st.deleted_at IS NULL`, tourID, version, mode).Scan(&raw, &digest)
	if errors.Is(mapErr(err), ErrNotFound) {
		return nil, "", ErrRouteSuperseded
	}
	if err != nil {
		return nil, "", err
	}
	var tour models.TourDetail
	err = json.Unmarshal(raw, &tour)
	return &tour, digest, err
}

// TourRouteInputs takes one consistent view of routing version and ordered sites.
func (s *Store) TourRouteInputs(ctx context.Context, tourID string, mode models.TravelMode, version int64) ([]geo.Point, string, error) {
	var raw []byte
	var digest string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE((SELECT jsonb_agg(jsonb_build_object('lat',p.latitude,'lng',p.longitude) ORDER BY ts.position) FROM tour_stops ts JOIN places p ON p.id=ts.place_id WHERE ts.tour_id=t.id),'[]'::jsonb),byl_route_input_hash(t.id,$3) FROM tours t JOIN tour_revision_state st ON st.tour_id=t.id WHERE t.id=$1 AND st.route_version=$2 AND st.deleted_at IS NULL`, tourID, version, mode).Scan(&raw, &digest)
	if errors.Is(mapErr(err), ErrNotFound) {
		return nil, "", ErrRouteSuperseded
	}
	if err != nil {
		return nil, "", err
	}
	var points []geo.Point
	err = json.Unmarshal(raw, &points)
	return points, digest, err
}

const revisionCols = `id,tour_id,number,draft_version,published_at,snapshot`

func scanRevision(row interface{ Scan(...any) error }) (*models.TourRevision, error) {
	r := &models.TourRevision{}
	var raw []byte
	if err := row.Scan(&r.ID, &r.TourID, &r.Number, &r.DraftVersion, &r.PublishedAt, &raw); err != nil {
		return nil, mapErr(err)
	}
	if err := json.Unmarshal(raw, &r.Snapshot); err != nil {
		return nil, err
	}
	if r.Snapshot == nil {
		return nil, ErrInvalidReference
	}
	r.Snapshot.Tour.RevisionID = &r.ID
	r.Snapshot.Tour.RevisionNumber = r.Number
	r.Snapshot.Tour.DraftVersion = r.DraftVersion
	r.Snapshot.Tour.Routes = map[models.TravelMode]models.RouteSummary{}
	for mode, route := range r.Snapshot.Routes {
		r.Snapshot.Tour.Routes[mode] = route.Summary()
	}
	for id, m := range r.Snapshot.Moments {
		m.Excerpt = models.Excerpt(m.Body)
		r.Snapshot.Moments[id] = m
	}
	for i := range r.Snapshot.Tour.Stops {
		stop := &r.Snapshot.Tour.Stops[i]
		if stop.FeaturedMoment != nil {
			if m, ok := r.Snapshot.Moments[stop.FeaturedMoment.ID]; ok {
				summary := m.MomentSummary
				stop.FeaturedMoment = &summary
			}
		}
	}
	return r, nil
}

// GetTourRevision reads retained content; the transport enforces current visibility.
func (s *Store) GetTourRevision(ctx context.Context, tourID, id string) (*models.TourRevision, error) {
	if !isUUID(tourID) || !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanRevision(s.pool.QueryRow(ctx, `SELECT `+revisionCols+` FROM tour_revisions WHERE id=$1 AND tour_id=$2`, id, tourID))
}

// ListTourRevisions pages release metadata; complete snapshots are fetched by ID.
func (s *Store) ListTourRevisions(ctx context.Context, tourID string, limit, offset int, includePrivate bool) ([]models.TourRevision, error) {
	if !isUUID(tourID) {
		return nil, ErrNotFound
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT `+revisionCols+` FROM tour_revisions WHERE tour_id=$1 AND ($4 OR snapshot->'tour'->>'visibility'<>'private') ORDER BY number DESC LIMIT $2 OFFSET $3`, tourID, clamp(limit), offset, includePrivate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TourRevision{}
	for rows.Next() {
		r, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		r.Snapshot = nil
		out = append(out, *r)
	}
	return out, rows.Err()
}

func bumpTourDraftTx(ctx context.Context, tx pgx.Tx, id string, routeChanged bool, expected *int64) error {
	var version int64
	if err := tx.QueryRow(ctx, `SELECT draft_version FROM tour_revision_state WHERE tour_id=$1 AND deleted_at IS NULL`, id).Scan(&version); err != nil {
		return mapErr(err)
	}
	if expected != nil && version != *expected {
		return fmt.Errorf("%w: tour draft changed; reload before editing", ErrConflict)
	}
	_, err := tx.Exec(ctx, `UPDATE tour_revision_state SET draft_version=draft_version+1,route_version=route_version+CASE WHEN $2 THEN 1 ELSE 0 END,
	 release_requested_version=CASE WHEN EXISTS(SELECT 1 FROM tours WHERE id=$1 AND owner_account_id IS NOT NULL AND status='published') THEN draft_version+1 END WHERE tour_id=$1`, id, routeChanged)
	return err
}

func (s *Store) publishRevisionTx(ctx context.Context, tourID, author string, in models.PublishRevisionRequest, explicit bool) (*models.TourRevision, error) {
	var version int64
	var current *string
	if err := s.pool.QueryRow(ctx, `SELECT draft_version,current_revision_id FROM tour_revision_state WHERE tour_id=$1 AND deleted_at IS NULL`, tourID).Scan(&version, &current); err != nil {
		return nil, mapErr(err)
	}
	if version != in.DraftVersion || (explicit && !sameOptionalID(current, in.CurrentRevisionID)) {
		return nil, fmt.Errorf("%w: tour draft or published revision changed", ErrConflict)
	}
	// Lock content until the single-statement snapshot is durable. Tour edits and
	// fenced route commits already use the tour-row lock held by the caller.
	for _, query := range []string{
		`SELECT p.id FROM places p WHERE p.id IN (SELECT place_id FROM tour_stops WHERE tour_id=$1) ORDER BY p.id FOR SHARE`,
		`SELECT m.id FROM moments m WHERE m.place_id IN (SELECT place_id FROM tour_stops WHERE tour_id=$1) ORDER BY m.id FOR SHARE`,
	} {
		rows, err := s.pool.Query(ctx, query, tourID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	blockers, err := s.TourPublishBlockers(ctx, tourID)
	if err != nil {
		return nil, err
	}
	if len(blockers) > 0 {
		return nil, fmt.Errorf("%w: cannot publish: %s", ErrConflict, strings.Join(blockers, "; "))
	}
	if _, err := s.pool.Exec(ctx, `UPDATE tours SET status='published',updated_at=now() WHERE id=$1`, tourID); err != nil {
		return nil, err
	}
	var id string
	err = s.pool.QueryRow(ctx, `INSERT INTO tour_revisions(tour_id,number,draft_version,snapshot,published_by)
	 SELECT $1,COALESCE(max(number),0)+1,$2,jsonb_set(byl_tour_snapshot($1),'{tour,route_version}',to_jsonb((SELECT route_version FROM tour_revision_state WHERE tour_id=$1))),NULLIF($3,'')::uuid FROM tour_revisions WHERE tour_id=$1 RETURNING id`, tourID, version, author).Scan(&id)
	if err != nil {
		return nil, mapErr(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE tour_revision_state SET current_revision_id=$2,release_requested_version=NULL WHERE tour_id=$1`, tourID, id); err != nil {
		return nil, err
	}
	return s.GetTourRevision(ctx, tourID, id)
}

func sameOptionalID(a, b *string) bool {
	value := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}
	return value(a) == value(b)
}

// PublishTourRevision releases an expected draft without allowing a stale publish
// to replace a newer pointer. Curator/owner authorization belongs to the transport.
func (s *Store) PublishTourRevision(ctx context.Context, tourID, author string, in models.PublishRevisionRequest) (*models.TourRevision, error) {
	var revision *models.TourRevision
	err := s.Transaction(ctx, func(tx *Store) error {
		var id string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, tourID).Scan(&id); err != nil {
			return mapErr(err)
		}
		var err error
		revision, err = tx.publishRevisionTx(ctx, tourID, author, in, true)
		return err
	})
	return revision, err
}

// publishRequestedRevisionTx fulfills only durable legacy save/share or migration
// release intent, for its exact current draft, after all routes are ready.
func (s *Store) publishRequestedRevisionTx(ctx context.Context, tourID string) error {
	var version int64
	var requested *int64
	var owner *string
	var status string
	if err := s.pool.QueryRow(ctx, `SELECT st.draft_version,st.release_requested_version,t.owner_account_id,t.status FROM tour_revision_state st JOIN tours t ON t.id=st.tour_id WHERE st.tour_id=$1 AND st.deleted_at IS NULL`, tourID).Scan(&version, &requested, &owner, &status); err != nil {
		return mapErr(err)
	}
	if requested == nil || *requested != version || status != "published" {
		return nil
	}
	var corridorReleased bool
	if err := s.pool.QueryRow(ctx, `SELECT byl_corridor_is_released($1)`, tourID).Scan(&corridorReleased); err != nil {
		return err
	}
	if !corridorReleased {
		return nil
	}
	// Provider success is not publication approval. Controlled navigation always
	// waits for the explicit curator/owner publication endpoint.
	var controlled bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tour_routes r CROSS JOIN LATERAL jsonb_array_elements(r.legs) leg WHERE r.tour_id=$1 AND leg->>'navigation_source'='approved_navigation')`, tourID).Scan(&controlled); err != nil {
		return err
	}
	if controlled {
		return nil
	}
	blockers, err := s.TourPublishBlockers(ctx, tourID)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return nil
	}
	author := ""
	if owner != nil {
		author = *owner
	}
	_, err = s.publishRevisionTx(ctx, tourID, author, models.PublishRevisionRequest{DraftVersion: version}, false)
	return err
}

// ServedTour returns the published revision, or the legacy pending initial view.
// An authenticated legacy trip keeps its pin until an explicit progress reset.
func (s *Store) ServedTour(ctx context.Context, working *models.TourDetail, account string) (*models.TourDetail, error) {
	id := working.RevisionID
	if account != "" {
		var pin string
		err := s.pool.QueryRow(ctx, `SELECT pin.revision_id FROM legacy_revision_pins pin JOIN tour_revision_progress progress ON progress.account_id=pin.account_id AND progress.revision_id=pin.revision_id WHERE pin.account_id=$1 AND pin.tour_id=$2`, account, working.ID).Scan(&pin)
		if err == nil {
			id = &pin
		} else if !errors.Is(mapErr(err), ErrNotFound) {
			return nil, err
		}
	}
	if id == nil {
		// The legacy pending initial view must not expose a newly reviewed
		// corridor before the owner/curator explicitly publishes its first release.
		pending := *working
		pending.Corridor = nil
		return &pending, nil
	}
	r, err := s.GetTourRevision(ctx, working.ID, *id)
	if err != nil {
		return nil, err
	}
	return &r.Snapshot.Tour, nil
}

// listPublishedTours filters and orders discovery by published metadata, not edits.
func (s *Store) listPublishedTours(ctx context.Context, f TourFilter) ([]models.TourSummary, string, error) {
	var offset int
	offset, _ = strconv.Atoi(f.Cursor)
	if offset < 0 {
		offset = 0
	}
	limit := clamp(f.Limit)
	rows, err := s.pool.Query(ctx, `SELECT t.id,r.id FROM tours t JOIN tour_revision_state st ON st.tour_id=t.id LEFT JOIN tour_revisions r ON r.id=st.current_revision_id
	 WHERE t.status='published' AND t.visibility='public' AND st.deleted_at IS NULL AND (r.id IS NULL OR r.snapshot->'tour'->>'visibility'='public')
	 AND ($1='' OR (r.id IS NOT NULL AND (r.snapshot->'tour'->'city_ids') ? $1) OR (r.id IS NULL AND (t.city_id::text=$1 OR EXISTS(SELECT 1 FROM tour_stops ts JOIN places p ON p.id=ts.place_id WHERE ts.tour_id=t.id AND p.city_id::text=$1))))
	 AND ($2='' OR (CASE WHEN r.id IS NULL THEN t.topic_id::text ELSE r.snapshot->'tour'->>'topic_id' END)=$2)
	 AND ($3='' OR COALESCE(r.snapshot->'tour'->'modes',to_jsonb(t.modes)) ? $3)
	 ORDER BY COALESCE(r.snapshot->'tour'->>'title',t.title),t.id LIMIT $4 OFFSET $5`, f.CityID, f.TopicID, string(f.Mode), limit+1, offset)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	type selected struct {
		id       string
		revision *string
	}
	ids := []selected{}
	for rows.Next() {
		var x selected
		if err := rows.Scan(&x.id, &x.revision); err != nil {
			return nil, "", err
		}
		ids = append(ids, x)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	rows.Close()
	out := []models.TourSummary{}
	next := ""
	if len(ids) > limit {
		ids = ids[:limit]
		next = strconv.Itoa(offset + limit)
	}
	for _, x := range ids {
		if x.revision != nil {
			r, err := s.GetTourRevision(ctx, x.id, *x.revision)
			if err != nil {
				return nil, "", err
			}
			out = append(out, r.Snapshot.Tour.TourSummary)
		} else {
			t, err := s.GetTour(ctx, x.id, true)
			if err != nil {
				return nil, "", err
			}
			out = append(out, t.TourSummary)
		}
	}
	return out, next, nil
}

// UpsertRouteVersioned rejects worker output after routing inputs change.
func (s *Store) UpsertRouteVersioned(ctx context.Context, r models.TourRoute, version int64, expectedHash ...string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		var id string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, r.TourID).Scan(&id); err != nil {
			return mapErr(err)
		}
		var current int64
		if err := tx.pool.QueryRow(ctx, `SELECT route_version FROM tour_revision_state WHERE tour_id=$1 AND deleted_at IS NULL`, r.TourID).Scan(&current); err != nil {
			return mapErr(err)
		}
		if current != version {
			return ErrRouteSuperseded
		}
		// Place coordinates are independent source records. Lock them while checking
		// the actual input digest, even when the tour version itself did not change.
		rows, err := tx.pool.Query(ctx, `SELECT p.id FROM places p WHERE p.id IN (SELECT place_id FROM tour_stops WHERE tour_id=$1) ORDER BY p.id FOR SHARE`, r.TourID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(expectedHash) > 0 {
			var actual string
			if err := tx.pool.QueryRow(ctx, `SELECT byl_route_input_hash($1,$2)`, r.TourID, r.Mode).Scan(&actual); err != nil {
				return err
			}
			if actual != expectedHash[0] {
				return ErrRouteSuperseded
			}
		}
		if err := tx.UpsertRoute(ctx, r); err != nil {
			return err
		}
		return tx.publishRequestedRevisionTx(ctx, r.TourID)
	})
}

// ValidateRevisionVisits checks stable IDs against the frozen itinerary.
func ValidateRevisionVisits(revision *models.TourRevision, mode models.TravelMode, ids []string) error {
	if !mode.Valid() || !slices.Contains(revision.Snapshot.Tour.Modes, mode) || len(ids) > 30 {
		return ErrInvalidReference
	}
	for _, id := range ids {
		if !isUUID(id) {
			return ErrInvalidReference
		}
		found := false
		for _, stop := range revision.Snapshot.Tour.Stops {
			if stop.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: stop does not belong to this revision", ErrInvalidReference)
		}
	}
	return nil
}
