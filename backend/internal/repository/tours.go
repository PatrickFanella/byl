package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

// TourFilter narrows ListTours.
type TourFilter struct {
	CityID             string
	TopicID            string
	Mode               models.TravelMode
	OwnerID            string
	SavedByAccountID   string
	PublicOnly         bool
	IncludeUnpublished bool
	Limit              int
	Cursor             string
}

// CreateTourInput creates a tour with optional stops.
type CreateTourInput struct {
	CityID         string
	Slug           string
	RandomSlug     bool
	Title          string
	Summary        string
	TopicID        *string
	Modes          []models.TravelMode
	HeroMediaID    *string
	Status         models.EntityStatus
	Visibility     models.TourVisibility
	OwnerAccountID *string
	Stops          []models.StopInput
}

// UpdateTourInput patches a tour. Nil pointers and nil slices leave fields unchanged.
type UpdateTourInput struct {
	ExpectedVersion    *int64
	DeferLegacyRelease bool
	Title              *string
	Summary            *string
	TopicID            *string
	ClearTopic         bool
	Modes              []models.TravelMode
	HeroMediaID        *string
	Visibility         *models.TourVisibility
}

const tourCols = `t.id, t.city_id,
	ARRAY(SELECT city_id::text FROM (SELECT t.city_id UNION SELECT p.city_id FROM tour_stops ts JOIN places p ON p.id=ts.place_id WHERE ts.tour_id=t.id) coverage ORDER BY city_id),
	t.slug, t.title, t.summary, t.topic_id, t.modes::text[], t.hero_media_id, t.status, t.visibility, t.owner_account_id,
	(SELECT count(*) FROM tour_stops ts WHERE ts.tour_id = t.id), t.created_at, t.updated_at,
 COALESCE((SELECT draft_version FROM tour_revision_state WHERE tour_id=t.id),0),
 COALESCE((SELECT route_version FROM tour_revision_state WHERE tour_id=t.id),0),
 (SELECT current_revision_id FROM tour_revision_state WHERE tour_id=t.id),
 COALESCE((SELECT r.number FROM tour_revision_state st JOIN tour_revisions r ON r.id=st.current_revision_id WHERE st.tour_id=t.id),0)`

type tourRow struct {
	models.TourSummary
	heroMediaID *string
}

func scanTour(row interface{ Scan(...any) error }) (tourRow, error) {
	var r tourRow
	var modes []string
	err := row.Scan(&r.ID, &r.CityID, &r.CityIDs, &r.Slug, &r.Title, &r.Summary, &r.TopicID, &modes, &r.heroMediaID, &r.Status, &r.Visibility, &r.OwnerAccountID, &r.StopCount, &r.CreatedAt, &r.UpdatedAt, &r.DraftVersion, &r.RouteVersion, &r.RevisionID, &r.RevisionNumber)
	if err != nil {
		return r, err
	}
	r.Modes = make([]models.TravelMode, 0, len(modes))
	for _, m := range modes {
		r.Modes = append(r.Modes, models.TravelMode(m))
	}
	r.Routes = map[models.TravelMode]models.RouteSummary{}
	return r, nil
}

func modesArg(modes []models.TravelMode) []string {
	if len(modes) == 0 {
		modes = models.AllTravelModes
	}
	seen := map[models.TravelMode]bool{}
	out := make([]string, 0, len(modes))
	for _, m := range models.AllTravelModes {
		for _, given := range modes {
			if given == m && !seen[m] {
				seen[m] = true
				out = append(out, string(m))
			}
		}
	}
	return out
}

// queryTours loads tour summaries with topics, hero media, and route summaries.
func (s *Store) queryTours(ctx context.Context, q dbtx, where string, args ...any) ([]models.TourSummary, error) {
	rows, err := q.Query(ctx, `SELECT `+tourCols+` FROM tours t `+strings.Replace(where, "WHERE ", "WHERE NOT EXISTS(SELECT 1 FROM tour_revision_state ds WHERE ds.tour_id=t.id AND ds.deleted_at IS NOT NULL) AND ", 1), args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var tours []tourRow
	var ids, topicIDs, mediaIDs []string
	for rows.Next() {
		r, err := scanTour(rows)
		if err != nil {
			return nil, err
		}
		tours = append(tours, r)
		ids = append(ids, r.ID)
		if r.TopicID != nil {
			topicIDs = append(topicIDs, *r.TopicID)
		}
		if r.heroMediaID != nil {
			mediaIDs = append(mediaIDs, *r.heroMediaID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []models.TourSummary{}
	if len(tours) == 0 {
		return out, nil
	}
	topics := map[string]models.Topic{}
	if len(topicIDs) > 0 {
		trows, err := q.Query(ctx, topicSelect+` WHERE t.id = ANY($1)`, topicIDs)
		if err != nil {
			return nil, err
		}
		for trows.Next() {
			tp, err := scanTopic(trows)
			if err != nil {
				trows.Close()
				return nil, err
			}
			topics[tp.ID] = tp
		}
		trows.Close()
	}
	media, err := s.mediaByIDs(ctx, q, mediaIDs)
	if err != nil {
		return nil, err
	}
	routes, err := s.routeSummaries(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range tours {
		ts := r.TourSummary
		if r.TopicID != nil {
			if tp, ok := topics[*r.TopicID]; ok {
				ts.Topic = &tp
			}
		}
		if r.heroMediaID != nil {
			if m, ok := media[*r.heroMediaID]; ok {
				ts.HeroMedia = &m
			}
		}
		if rs, ok := routes[r.ID]; ok {
			ts.Routes = rs
		}
		out = append(out, ts)
	}
	return out, nil
}

func (s *Store) routeSummaries(ctx context.Context, q dbtx, tourIDs []string) (map[string]map[models.TravelMode]models.RouteSummary, error) {
	out := map[string]map[models.TravelMode]models.RouteSummary{}
	rows, err := q.Query(ctx, `SELECT tour_id, mode, status, distance_m, duration_s, error, computed_at,byl_path_quality_summary(tour_id,mode) FROM tour_routes WHERE tour_id = ANY($1)`, tourIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tourID string
		var r models.TourRoute
		var quality []byte
		if err := rows.Scan(&tourID, &r.Mode, &r.Status, &r.DistanceM, &r.DurationS, &r.Error, &r.ComputedAt, &quality); err != nil {
			return nil, err
		}
		if len(quality) > 0 {
			if err := json.Unmarshal(quality, &r.Quality); err != nil {
				return nil, err
			}
		}
		if out[tourID] == nil {
			out[tourID] = map[models.TravelMode]models.RouteSummary{}
		}
		out[tourID][r.Mode] = r.Summary()
	}
	return out, rows.Err()
}

// ListTours returns tour summaries and the next cursor.
func (s *Store) ListTours(ctx context.Context, f TourFilter) ([]models.TourSummary, string, error) {
	if f.PublicOnly {
		return s.listPublishedTours(ctx, f)
	}
	where := []string{"TRUE"}
	args := []any{}
	add := func(v any) int { args = append(args, v); return len(args) }
	if f.CityID != "" {
		n := add(f.CityID)
		where = append(where, fmt.Sprintf("(t.city_id=$%d OR EXISTS (SELECT 1 FROM tour_stops ts JOIN places p ON p.id=ts.place_id WHERE ts.tour_id=t.id AND p.city_id=$%d))", n, n))
	}
	if f.TopicID != "" {
		where = append(where, fmt.Sprintf("t.topic_id = $%d", add(f.TopicID)))
	}
	if f.Mode != "" {
		where = append(where, fmt.Sprintf("$%d::travel_mode = ANY(t.modes)", add(string(f.Mode))))
	}
	if f.OwnerID != "" {
		where = append(where, fmt.Sprintf("t.owner_account_id = $%d", add(f.OwnerID)))
	}
	if f.SavedByAccountID != "" {
		n := add(f.SavedByAccountID)
		where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM saved_tours st WHERE st.tour_id = t.id AND st.account_id = $%d)", n))
		// A saved tour is only listed while the saver could still open it.
		where = append(where, fmt.Sprintf("(t.owner_account_id = $%d OR (t.visibility <> 'private' AND t.status = 'published'))", n))
	}
	if f.PublicOnly {
		where = append(where, "t.visibility = 'public' AND t.status = 'published'")
	} else if !f.IncludeUnpublished {
		where = append(where, "t.status = 'published'")
	}
	limit := clamp(f.Limit)
	offset, _ := strconv.Atoi(f.Cursor)
	if offset < 0 {
		offset = 0
	}
	tours, err := s.queryTours(ctx, s.pool, fmt.Sprintf("WHERE %s ORDER BY t.title,t.id LIMIT $%d OFFSET $%d", strings.Join(where, " AND "), add(limit+1), add(offset)), args...)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(tours) > limit {
		tours = tours[:limit]
		next = strconv.Itoa(offset + limit)
	}
	return tours, next, nil
}

// ResolveTourID turns an id or slug into an id.
func (s *Store) ResolveTourID(ctx context.Context, ref string) (string, error) {
	var id string
	var err error
	if isUUID(ref) {
		err = s.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id = $1 AND NOT EXISTS(SELECT 1 FROM tour_revision_state ds WHERE ds.tour_id=tours.id AND ds.deleted_at IS NOT NULL)`, ref).Scan(&id)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT id FROM tours WHERE slug = $1 AND NOT EXISTS(SELECT 1 FROM tour_revision_state ds WHERE ds.tour_id=tours.id AND ds.deleted_at IS NOT NULL)`, ref).Scan(&id)
	}
	return id, mapErr(err)
}

// GetTour returns a tour with stops regardless of status or visibility; callers apply access rules.
// With publicOnly, unpublished featured moments are replaced by the place's primary moment when that is published.
func (s *Store) GetTour(ctx context.Context, ref string, publicOnly bool) (*models.TourDetail, error) {
	id, err := s.ResolveTourID(ctx, ref)
	if err != nil {
		return nil, err
	}
	tours, err := s.queryTours(ctx, s.pool, `WHERE t.id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(tours) == 0 {
		return nil, ErrNotFound
	}
	stops, err := s.tourStops(ctx, id, publicOnly)
	if err != nil {
		return nil, err
	}
	corridor, err := s.GetApprovedCorridor(ctx, id)
	if err != nil {
		return nil, err
	}
	tour := &models.TourDetail{TourSummary: tours[0], Stops: stops, Corridor: corridor, WorkingCopy: true, Journeys: map[models.TravelMode]*models.RouteJourney{}}
	for _, mode := range tour.Modes {
		route, err := s.GetTourRoute(ctx, id, mode)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		tour.Journeys[mode] = models.DescribeJourney(tour, route)
	}
	return tour, nil
}

func (s *Store) tourStops(ctx context.Context, tourID string, publicOnly bool) ([]models.TourStop, error) {
	rows, err := s.pool.Query(ctx, `SELECT ts.id, ts.position, ts.place_id, ts.featured_moment_id, ts.note,
		CASE WHEN $2 THEN COALESCE(
			(SELECT fm.id FROM moments fm WHERE fm.id = ts.featured_moment_id AND fm.status = 'published'),
			(SELECT pm.id FROM moments pm WHERE pm.id = p.primary_moment_id AND pm.status = 'published'))
		ELSE COALESCE(ts.featured_moment_id, p.primary_moment_id) END
		FROM tour_stops ts JOIN places p ON p.id = ts.place_id WHERE ts.tour_id = $1 ORDER BY ts.position`, tourID, publicOnly)
	if err != nil {
		return nil, err
	}
	type stopRow struct {
		stop     models.TourStop
		placeID  string
		momentID *string
	}
	var stopRows []stopRow
	var placeIDs, momentIDs []string
	for rows.Next() {
		var sr stopRow
		if err := rows.Scan(&sr.stop.ID, &sr.stop.Position, &sr.placeID, &sr.stop.FeaturedMomentID, &sr.stop.Note, &sr.momentID); err != nil {
			rows.Close()
			return nil, err
		}
		stopRows = append(stopRows, sr)
		placeIDs = append(placeIDs, sr.placeID)
		if sr.momentID != nil {
			momentIDs = append(momentIDs, *sr.momentID)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []models.TourStop{}
	if len(stopRows) == 0 {
		return out, nil
	}
	places := map[string]models.PlaceSummary{}
	prows, err := s.pool.Query(ctx, `SELECT `+placeCols+` FROM places p WHERE p.id = ANY($1)`, placeIDs)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		p, err := scanPlace(prows, false)
		if err != nil {
			prows.Close()
			return nil, err
		}
		places[p.ID] = p
	}
	prows.Close()
	moments := map[string]models.MomentSummary{}
	if len(momentIDs) > 0 {
		mrows, err := s.queryMoments(ctx, s.pool, `WHERE m.id = ANY($1)`, momentIDs)
		if err != nil {
			return nil, err
		}
		for _, m := range mrows {
			moments[m.ID] = m.MomentSummary
		}
	}
	for _, sr := range stopRows {
		st := sr.stop
		st.Place = places[sr.placeID]
		if sr.momentID != nil {
			if m, ok := moments[*sr.momentID]; ok {
				st.FeaturedMoment = &m
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// CreateTour inserts a tour, its stops, pending route rows, and route jobs in one transaction.
func (s *Store) CreateTour(ctx context.Context, in CreateTourInput) (*models.TourDetail, error) {
	status := in.Status
	if status == "" {
		status = models.EntityStatusDraft
	}
	visibility := in.Visibility
	if visibility == "" {
		visibility = models.TourVisibilityPublic
	}
	modes := modesArg(in.Modes)
	var id string
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var slug string
		var err error
		if in.RandomSlug {
			slug, err = uniqueSlug(ctx, tx, "tours", "", "", geo.RandomSlug(10))
		} else {
			slug, err = uniqueSlug(ctx, tx, "tours", "", "", slugFor(in.Slug, in.Title))
		}
		if err != nil {
			return err
		}
		// Lock coverage before the FK takes a key-share lock on the topic row.
		if in.TopicID != nil && *in.TopicID != "" {
			if err := requireTopicCities(ctx, tx, *in.TopicID, nil); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `INSERT INTO tours (city_id, slug, title, summary, topic_id, modes, hero_media_id, status, visibility, owner_account_id)
			VALUES ($1,$2,$3,$4,$5,$6::travel_mode[],$7,$8,$9,$10) RETURNING id`,
			in.CityID, slug, strings.TrimSpace(in.Title), in.Summary, nullable(in.TopicID), modes, nullable(in.HeroMediaID), status, visibility, nullable(in.OwnerAccountID)).Scan(&id); err != nil {
			return err
		}
		if err := replaceStopsTx(ctx, tx, id, in.Stops); err != nil {
			return err
		}
		if err := syncRoutesTx(ctx, tx, id, modes, true); err != nil {
			return err
		}
		return (&Store{pool: transactionDB{tx}, rawPool: s.rawPool}).publishRequestedRevisionTx(ctx, id)
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetTour(ctx, id, false)
}

// UpdateTour patches a tour; changing modes adds or removes route rows and jobs.
func (s *Store) UpdateTour(ctx context.Context, id string, in UpdateTourInput) (*models.TourDetail, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		// Existing tour writes take the tour lock before any topic lock, like ReplaceStops.
		var lockedID string
		if err := tx.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
			return err
		}
		if err := bumpTourDraftTx(ctx, tx, id, in.Modes != nil, in.ExpectedVersion); err != nil {
			return err
		}
		if !in.ClearTopic && in.TopicID != nil && *in.TopicID != "" {
			if err := requireTopicCities(ctx, tx, *in.TopicID, nil); err != nil {
				return err
			}
		}
		var modesArgValue any
		if in.Modes != nil {
			modesArgValue = modesArg(in.Modes)
		}
		tag, err := tx.Exec(ctx, `UPDATE tours SET title = COALESCE($2, title), summary = COALESCE($3, summary),
			topic_id = CASE WHEN $4 THEN NULL WHEN $5::uuid IS NOT NULL THEN $5::uuid ELSE topic_id END,
			modes = COALESCE($6::travel_mode[], modes), hero_media_id = CASE WHEN $7 THEN $8::uuid ELSE hero_media_id END,
			visibility = COALESCE($9, visibility), updated_at = $10 WHERE id = $1`,
			id, in.Title, in.Summary, in.ClearTopic, nullable(in.TopicID), modesArgValue, in.HeroMediaID != nil, nullable(in.HeroMediaID), in.Visibility, time.Now())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := requireTourTopicCities(ctx, tx, id); err != nil {
			return err
		}
		if in.Modes != nil {
			if err := syncRoutesTx(ctx, tx, id, modesArg(in.Modes), false); err != nil {
				return err
			}
		}
		if in.DeferLegacyRelease {
			return nil
		}
		return (&Store{pool: transactionDB{tx}, rawPool: s.rawPool}).publishRequestedRevisionTx(ctx, id)
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return s.GetTour(ctx, id, false)
}

// ReplaceStops sets the stop list and requeues every route.
func (s *Store) ReplaceStops(ctx context.Context, tourID string, stops []models.StopInput) error {
	return s.ReplaceStopsVersioned(ctx, tourID, stops, nil)
}

// ReplaceStopsVersioned rejects stale editors and retains stable stop identities.
func (s *Store) ReplaceStopsVersioned(ctx context.Context, tourID string, stops []models.StopInput, expected *int64) error {
	return mapErr(s.withTx(ctx, func(tx pgx.Tx) error {
		var modes []string
		if err := tx.QueryRow(ctx, `SELECT modes::text[] FROM tours WHERE id = $1 FOR UPDATE`, tourID).Scan(&modes); err != nil {
			return err
		}
		if err := bumpTourDraftTx(ctx, tx, tourID, true, expected); err != nil {
			return err
		}
		if err := replaceStopsTx(ctx, tx, tourID, stops); err != nil {
			return err
		}
		return syncRoutesTx(ctx, tx, tourID, modes, true)
	}))
}

func replaceStopsTx(ctx context.Context, tx pgx.Tx, tourID string, stops []models.StopInput) error {
	var cityID string
	var ownerID *string
	if err := tx.QueryRow(ctx, `SELECT city_id,owner_account_id FROM tours WHERE id = $1 FOR UPDATE`, tourID).Scan(&cityID, &ownerID); err != nil {
		return err
	}
	type previousStop struct {
		id, place string
		moment    *string
	}
	previous := []previousStop{}
	rows, err := tx.Query(ctx, `SELECT id,place_id,featured_moment_id FROM tour_stops WHERE tour_id=$1 ORDER BY position`, tourID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p previousStop
		if err := rows.Scan(&p.id, &p.place, &p.moment); err != nil {
			rows.Close()
			return err
		}
		previous = append(previous, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	used := map[string]bool{}
	if _, err := tx.Exec(ctx, `DELETE FROM tour_stops WHERE tour_id = $1`, tourID); err != nil {
		return err
	}
	for i, st := range stops {
		if !isUUID(st.PlaceID) {
			return fmt.Errorf("%w: stop %d: unknown place", ErrInvalidReference, i+1)
		}
		var placeCity string
		if err := tx.QueryRow(ctx, `SELECT city_id FROM places WHERE id = $1`, st.PlaceID).Scan(&placeCity); err != nil {
			if mapErr(err) == ErrNotFound {
				return fmt.Errorf("%w: stop %d: unknown place", ErrInvalidReference, i+1)
			}
			return err
		}
		if ownerID != nil && placeCity != cityID {
			return fmt.Errorf("%w: stop %d: place is in a different city", ErrInvalidReference, i+1)
		}
		var featured any
		if st.FeaturedMomentID != nil && *st.FeaturedMomentID != "" {
			if !isUUID(*st.FeaturedMomentID) {
				return fmt.Errorf("%w: stop %d: unknown featured moment", ErrInvalidReference, i+1)
			}
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM moments WHERE id = $1 AND place_id = $2)`, *st.FeaturedMomentID, st.PlaceID).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%w: stop %d: featured moment does not belong to the place", ErrInvalidReference, i+1)
			}
			featured = *st.FeaturedMomentID
		}
		stable := ""
		for _, p := range previous {
			if !used[p.id] && p.place == st.PlaceID && sameOptionalID(p.moment, st.FeaturedMomentID) && (st.ID == "" || st.ID == p.id) {
				stable = p.id
				break
			}
		}
		if st.ID != "" && stable == "" {
			return fmt.Errorf("%w: stop ID does not match this tour/place/story", ErrInvalidReference)
		}
		if stable != "" {
			used[stable] = true
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tour_stops(id,tour_id,position,place_id,featured_moment_id,note) VALUES(COALESCE(NULLIF($1,'')::uuid,gen_random_uuid()),$2,$3,$4,$5,$6)`, stable, tourID, i+1, st.PlaceID, featured, st.Note); err != nil {
			return err
		}
	}
	return requireTourTopicCities(ctx, tx, tourID)
}

func requireTourTopicCities(ctx context.Context, q dbtx, tourID string) error {
	var topicID *string
	var cities []string
	if err := q.QueryRow(ctx, `SELECT t.topic_id,ARRAY(SELECT city_id::text FROM (SELECT t.city_id UNION SELECT p.city_id FROM tour_stops ts JOIN places p ON p.id=ts.place_id WHERE ts.tour_id=t.id) coverage ORDER BY city_id) FROM tours t WHERE t.id=$1`, tourID).Scan(&topicID, &cities); err != nil {
		return err
	}
	if topicID == nil {
		return nil
	}
	return requireTopicCities(ctx, q, *topicID, cities)
}

// syncRoutesTx makes tour_routes match modes. New modes get a pending row and a job; when
// requeueAll is set every mode is reset to pending and re-enqueued.
func syncRoutesTx(ctx context.Context, tx pgx.Tx, tourID string, modes []string, requeueAll bool) error {
	var version int64
	if err := tx.QueryRow(ctx, `SELECT route_version FROM tour_revision_state WHERE tour_id=$1`, tourID).Scan(&version); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tour_routes WHERE tour_id = $1 AND NOT (mode::text = ANY($2))`, tourID, modes); err != nil {
		return err
	}
	for _, m := range modes {
		tag, err := tx.Exec(ctx, `INSERT INTO tour_routes (tour_id, mode, status) VALUES ($1, $2, 'pending') ON CONFLICT (tour_id, mode) DO NOTHING`, tourID, m)
		if err != nil {
			return err
		}
		inserted := tag.RowsAffected() == 1
		requeue := requeueAll
		if !inserted && !requeue {
			var reusable bool
			if err := tx.QueryRow(ctx, `SELECT r.status='ready' AND COALESCE(i.input_hash=byl_route_input_hash($1,$2),false) FROM tour_routes r LEFT JOIN tour_route_inputs i ON i.tour_id=r.tour_id AND i.mode=r.mode WHERE r.tour_id=$1 AND r.mode=$2`, tourID, m).Scan(&reusable); err != nil {
				return err
			}
			requeue = !reusable
		}
		if requeue && !inserted {
			if _, err := tx.Exec(ctx, `UPDATE tour_routes SET status = 'pending', error = '', updated_at = now() WHERE tour_id = $1 AND mode = $2`, tourID, m); err != nil {
				return err
			}
		}
		if inserted || requeue {
			if err := enqueueTx(ctx, tx, models.JobKindComputeRoute, models.ComputeRoutePayload{TourID: tourID, Mode: models.TravelMode(m), RouteVersion: version}); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteTour hides retained releases; only never-published tours are removed.
func (s *Store) DeleteTour(ctx context.Context, id string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		var found string
		if err := tx.pool.QueryRow(ctx, `SELECT id FROM tours WHERE id=$1 FOR UPDATE`, id).Scan(&found); err != nil {
			return mapErr(err)
		}
		var retained bool
		if err := tx.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tour_revisions WHERE tour_id=$1) OR EXISTS(SELECT 1 FROM tour_corridor_proposals WHERE tour_id=$1) OR EXISTS(SELECT 1 FROM tour_path_quality_reports WHERE tour_id=$1)`, id).Scan(&retained); err != nil {
			return err
		}
		if !retained {
			_, err := tx.pool.Exec(ctx, `DELETE FROM tours WHERE id=$1`, id)
			return mapErr(err)
		}
		if _, err := tx.pool.Exec(ctx, `UPDATE tour_revision_state SET deleted_at=now(),release_requested_version=NULL WHERE tour_id=$1`, id); err != nil {
			return err
		}
		_, err := tx.pool.Exec(ctx, `UPDATE tours SET status='draft',visibility='private',updated_at=now() WHERE id=$1`, id)
		return err
	})
}

// SetTourStatus changes a tour's status.
func (s *Store) SetTourStatus(ctx context.Context, id string, status models.EntityStatus) error {
	tag, err := s.pool.Exec(ctx, `UPDATE tours SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TourPublishBlockers lists why a tour cannot be published: unpublished places, missing or unready routes.
func (s *Store) TourPublishBlockers(ctx context.Context, id string) ([]string, error) {
	var out []string
	rows, err := s.pool.Query(ctx, `SELECT ts.position, p.slug, p.status FROM tour_stops ts JOIN places p ON p.id = ts.place_id WHERE ts.tour_id = $1 AND p.status <> 'published' ORDER BY ts.position`, id)
	if err != nil {
		return nil, mapErr(err)
	}
	for rows.Next() {
		var pos int
		var slug, status string
		if err := rows.Scan(&pos, &slug, &status); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, fmt.Sprintf("stop %d: place %s is %s, not published", pos, slug, status))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mrows, err := s.pool.Query(ctx, `SELECT ts.position, m.slug, m.status FROM tour_stops ts JOIN moments m ON m.id = ts.featured_moment_id WHERE ts.tour_id = $1 AND m.status <> 'published' ORDER BY ts.position`, id)
	if err != nil {
		return nil, mapErr(err)
	}
	for mrows.Next() {
		var pos int
		var slug, status string
		if err := mrows.Scan(&pos, &slug, &status); err != nil {
			mrows.Close()
			return nil, err
		}
		out = append(out, fmt.Sprintf("stop %d: featured moment %s is %s, not published", pos, slug, status))
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	var stopCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM tour_stops WHERE tour_id = $1`, id).Scan(&stopCount); err != nil {
		return nil, err
	}
	if stopCount == 0 {
		out = append(out, "tour has no stops")
	}
	rrows, err := s.pool.Query(ctx, `SELECT m::text, COALESCE(r.status::text, 'missing'), COALESCE(r.error, ''), COALESCE(i.input_hash=byl_route_input_hash($1,m),false),byl_path_quality_passes($1,m),byl_path_quality_summary($1,m) FROM unnest((SELECT modes FROM tours WHERE id = $1)) AS m
 LEFT JOIN tour_routes r ON r.tour_id=$1 AND r.mode=m LEFT JOIN tour_route_inputs i ON i.tour_id=$1 AND i.mode=m ORDER BY m`, id)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rrows.Close()
	for rrows.Next() {
		var mode, status, errText string
		var fresh bool
		var qualityPass bool
		var qualityRaw []byte
		if err := rrows.Scan(&mode, &status, &errText, &fresh, &qualityPass, &qualityRaw); err != nil {
			return nil, err
		}
		if !qualityPass {
			state := "missing"
			if len(qualityRaw) > 0 {
				var q models.PathQualitySummary
				if err := json.Unmarshal(qualityRaw, &q); err != nil {
					return nil, err
				}
				state = q.Status
				if !q.InputCurrent {
					state = "stale"
				}
				for i, issue := range q.Issues {
					if i >= 3 {
						break
					}
					state += " " + issue.Code
				}
			}
			out = append(out, "route for "+mode+" path quality is "+state)
		}
		if status == string(models.RouteStatusReady) && !fresh {
			out = append(out, "route for "+mode+" inputs changed; recompute before publication")
		}
		if status != string(models.RouteStatusReady) {
			msg := fmt.Sprintf("route for %s is %s", mode, status)
			if errText != "" {
				msg += ": " + errText
			}
			out = append(out, msg)
		}
	}
	if err := rrows.Err(); err != nil {
		return nil, err
	}
	rrows.Close()
	working, err := s.GetTour(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if working.Corridor != nil && working.Corridor.Intent != nil {
		if err := working.Corridor.Intent.ValidateStops(working); err != nil {
			out = append(out, "reviewed corridor intent needs replacement: "+err.Error())
		}
	}
	return out, nil
}

// GetTourRoute returns the full route for a mode.
func (s *Store) GetTourRoute(ctx context.Context, tourID string, mode models.TravelMode) (*models.TourRoute, error) {
	var r models.TourRoute
	var geom, legs []byte
	var quality []byte
	err := s.pool.QueryRow(ctx, `SELECT id, tour_id, mode, status, provider, distance_m, duration_s, geometry, legs, error, computed_at,byl_path_quality_summary(tour_id,mode) FROM tour_routes WHERE tour_id = $1 AND mode = $2`, tourID, mode).
		Scan(&r.ID, &r.TourID, &r.Mode, &r.Status, &r.Provider, &r.DistanceM, &r.DurationS, &geom, &legs, &r.Error, &r.ComputedAt, &quality)
	if err != nil {
		return nil, mapErr(err)
	}
	if len(quality) > 0 {
		if err := json.Unmarshal(quality, &r.Quality); err != nil {
			return nil, err
		}
	}
	if len(geom) > 0 {
		r.Geometry = &models.LineString{}
		if err := json.Unmarshal(geom, r.Geometry); err != nil {
			return nil, err
		}
	}
	r.Legs = []models.RouteLeg{}
	if len(legs) > 0 {
		if err := json.Unmarshal(legs, &r.Legs); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// UpsertRoute writes a computed (or failed) route for a tour and mode.
func (s *Store) UpsertRoute(ctx context.Context, r models.TourRoute) error {
	var geom, legs any
	if r.Geometry != nil {
		b, err := json.Marshal(r.Geometry)
		if err != nil {
			return err
		}
		geom = b
	}
	if r.Legs != nil {
		b, err := json.Marshal(r.Legs)
		if err != nil {
			return err
		}
		legs = b
	}
	computedAt := r.ComputedAt
	if computedAt == nil && r.Status == models.RouteStatusReady {
		now := time.Now()
		computedAt = &now
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO tour_routes (tour_id, mode, status, provider, distance_m, duration_s, geometry, legs, error, computed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (tour_id, mode) DO UPDATE SET status = EXCLUDED.status, provider = EXCLUDED.provider, distance_m = EXCLUDED.distance_m,
			duration_s = EXCLUDED.duration_s, geometry = EXCLUDED.geometry, legs = EXCLUDED.legs, error = EXCLUDED.error, computed_at = EXCLUDED.computed_at, updated_at = now()`,
		r.TourID, r.Mode, r.Status, r.Provider, r.DistanceM, r.DurationS, geom, legs, r.Error, computedAt)
	if err != nil {
		return mapErr(err)
	}
	if r.Status == models.RouteStatusReady {
		_, err = s.pool.Exec(ctx, `INSERT INTO tour_route_inputs(tour_id,mode,input_hash) VALUES($1,$2,byl_route_input_hash($1,$2)) ON CONFLICT(tour_id,mode) DO UPDATE SET input_hash=excluded.input_hash`, r.TourID, r.Mode)
	}
	return mapErr(err)
}

// TourStopPoints returns the ordered coordinates of a tour's stops.
func (s *Store) TourStopPoints(ctx context.Context, tourID string) ([]geo.Point, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.latitude, p.longitude FROM tour_stops ts JOIN places p ON p.id = ts.place_id WHERE ts.tour_id = $1 ORDER BY ts.position`, tourID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []geo.Point
	for rows.Next() {
		var pt geo.Point
		if err := rows.Scan(&pt.Lat, &pt.Lng); err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	return out, rows.Err()
}

// RecomputeRoutes marks every route pending and enqueues a job per mode.
func (s *Store) RecomputeRoutes(ctx context.Context, tourID string) error {
	return mapErr(s.withTx(ctx, func(tx pgx.Tx) error {
		var modes []string
		if err := tx.QueryRow(ctx, `SELECT modes::text[] FROM tours WHERE id = $1 FOR UPDATE`, tourID).Scan(&modes); err != nil {
			return err
		}
		if err := bumpTourDraftTx(ctx, tx, tourID, true, nil); err != nil {
			return err
		}
		return syncRoutesTx(ctx, tx, tourID, modes, true)
	}))
}

// PublishedCuratedToursUsingPlace returns slugs of published curated (ownerless) tours that stop at the place.
func (s *Store) PublishedCuratedToursUsingPlace(ctx context.Context, placeID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT t.slug FROM tours t JOIN tour_revision_state st ON st.tour_id=t.id LEFT JOIN tour_revisions r ON r.id=st.current_revision_id
 WHERE t.status='published' AND t.owner_account_id IS NULL AND st.deleted_at IS NULL AND
 ((r.id IS NOT NULL AND EXISTS(SELECT 1 FROM jsonb_array_elements(r.snapshot->'tour'->'stops') stop WHERE stop->'place'->>'id'=($1::uuid)::text)) OR
 (r.id IS NULL AND EXISTS(SELECT 1 FROM tour_stops ts WHERE ts.tour_id=t.id AND ts.place_id=$1::uuid))) ORDER BY t.slug`, placeID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}
