-- +goose Up
CREATE TABLE tour_transit_choices (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 tour_id UUID NOT NULL REFERENCES tours(id),
 version BIGINT NOT NULL CHECK(version>0),
 draft_version BIGINT NOT NULL CHECK(draft_version>=0),
 choice JSONB,
 reviewed_by UUID NOT NULL REFERENCES accounts(id),
 review_note TEXT NOT NULL CHECK(length(trim(review_note)) BETWEEN 1 AND 4000),
 reviewed_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tour_id,version),
 UNIQUE(tour_id,id,version)
);
CREATE TABLE tour_transit_current (
 tour_id UUID PRIMARY KEY REFERENCES tours(id),
 choice_id UUID NOT NULL UNIQUE,
 version BIGINT NOT NULL,
 FOREIGN KEY(tour_id,choice_id,version) REFERENCES tour_transit_choices(tour_id,id,version)
);
-- +goose StatementBegin
CREATE FUNCTION retain_transit_choice() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Reviewed transit selections are immutable; make a new version'; END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_transit_choice BEFORE UPDATE OR DELETE ON tour_transit_choices
 FOR EACH ROW EXECUTE FUNCTION retain_transit_choice();

-- A mixed revision never presents a continuous walking line for its rail leg.
-- Ordinary qualified walking legs are retained separately, excluding that leg.
-- The schedule is a later explicit departure-context plan, not a live join in
-- the published historical snapshot.
ALTER FUNCTION byl_tour_snapshot(UUID) RENAME TO byl_tour_snapshot_v19;
-- +goose StatementBegin
CREATE FUNCTION byl_tour_snapshot(tour UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 WITH base AS (SELECT byl_tour_snapshot_v19(tour) AS value), selected AS (
  SELECT c.choice || jsonb_build_object('version',c.version,'walking_legs',
   COALESCE((SELECT jsonb_agg(leg ORDER BY position) FROM base,
    jsonb_array_elements(base.value#>'{routes,walk,legs}') WITH ORDINALITY AS legs(leg,position)
    WHERE (leg->>'from_position')::integer<>(SELECT position FROM tour_stops WHERE tour_id=tour AND id=(c.choice#>>'{intent,from_stop_id}')::uuid)
       OR (leg->>'to_position')::integer<>(SELECT position FROM tour_stops WHERE tour_id=tour AND id=(c.choice#>>'{intent,to_stop_id}')::uuid)), '[]'::jsonb)) AS choice
  FROM tour_transit_current p JOIN tour_transit_choices c ON c.id=p.choice_id AND c.tour_id=p.tour_id
  WHERE p.tour_id=tour AND c.choice IS NOT NULL
 )
 SELECT CASE WHEN selected.choice IS NULL THEN base.value ELSE
  jsonb_set(jsonb_set(base.value,'{tour,transit}',selected.choice),
   '{routes}',(base.value->'routes')-'walk') END
 FROM base LEFT JOIN selected ON true
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Retain reviewed transit selections and immutable mixed revisions during image rollback'; END $$;
-- +goose StatementEnd
