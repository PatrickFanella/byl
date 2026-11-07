-- +goose Up
-- Retain the exact admitted hash when no approved arrival exists. Existing
-- caches/snapshots are not rewritten. Corridor history is already fenced by v14.
ALTER FUNCTION byl_route_input_hash(UUID,travel_mode) RENAME TO byl_route_input_hash_v14;
-- +goose StatementBegin
CREATE FUNCTION byl_route_input_hash(tour UUID,mode travel_mode) RETURNS TEXT LANGUAGE sql STABLE AS $$
 WITH arrivals AS (
  SELECT jsonb_agg(jsonb_build_object('stop_id',ts.id,'arrival_id',ap.id,'version',ac.version,'point',ap.point) ORDER BY ts.position) AS value
  FROM tour_stops ts JOIN place_arrival_current ac ON ac.place_id=ts.place_id
  JOIN place_arrival_proposals ap ON ap.id=ac.proposal_id AND ap.status='approved'
  WHERE ts.tour_id=tour
 )
 SELECT CASE WHEN arrivals.value IS NULL THEN byl_route_input_hash_v14(tour,mode)
  ELSE md5(byl_route_input_hash_v14(tour,mode)||':arrivals:'||arrivals.value::text) END FROM arrivals;
$$;
-- +goose StatementEnd

-- Record the actual coordinate source only for newly computed controlled legs.
-- Stable story identities and returns are retained in each mode's route JSON.
ALTER FUNCTION byl_tour_snapshot(UUID) RENAME TO byl_tour_snapshot_v14;
-- +goose StatementBegin
CREATE FUNCTION byl_tour_snapshot(tour UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 WITH base AS (SELECT byl_tour_snapshot_v14(tour) AS value), controlled AS (
  SELECT EXISTS(SELECT 1 FROM tour_routes r CROSS JOIN LATERAL jsonb_array_elements(r.legs) leg
   WHERE r.tour_id=tour AND leg->>'navigation_source'='approved_navigation') AS value
 )
 SELECT CASE WHEN controlled.value THEN jsonb_set(jsonb_set(base.value,
  '{routing_intent,coordinate_source}','"approved_navigation"'::jsonb),'{routing_intent,stops}',
  (SELECT jsonb_agg(s || jsonb_build_object('site_latitude',s->'latitude','site_longitude',s->'longitude',
    'latitude',s#>'{arrival_point,point,latitude}','longitude',s#>'{arrival_point,point,longitude}') ORDER BY ordinal)
   FROM jsonb_array_elements(base.value#>'{routing_intent,stops}') WITH ORDINALITY entry(s,ordinal)))
 ELSE base.value END FROM base,controlled;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain approved navigation input fences and published route history during image recovery';
END $$;
-- +goose StatementEnd
