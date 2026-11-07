-- +goose Up
CREATE TABLE tour_corridor_proposals (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 tour_id UUID NOT NULL REFERENCES tours(id),
 base_version BIGINT NOT NULL CHECK(base_version>=0),
 draft_version BIGINT NOT NULL CHECK(draft_version>=0),
 intent JSONB,
 remove BOOLEAN NOT NULL DEFAULT false,
 content_sha256 TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 created_by UUID NOT NULL REFERENCES accounts(id),
 reviewed_by UUID REFERENCES accounts(id),
 review_note TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 reviewed_at TIMESTAMPTZ,
 CHECK((remove AND intent IS NULL) OR (NOT remove AND intent IS NOT NULL AND jsonb_typeof(intent)='object')),
 UNIQUE(tour_id,id)
);
CREATE INDEX tour_corridor_proposals_tour_idx ON tour_corridor_proposals(tour_id,created_at DESC,id);
CREATE TABLE tour_corridor_current (
 tour_id UUID PRIMARY KEY REFERENCES tours(id),
 proposal_id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>0),
 FOREIGN KEY(tour_id,proposal_id) REFERENCES tour_corridor_proposals(tour_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION retain_corridor_proposal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.tour_id,NEW.base_version,NEW.draft_version,NEW.intent,NEW.remove,NEW.content_sha256,NEW.created_by,NEW.created_at)
 IS DISTINCT FROM (OLD.tour_id,OLD.base_version,OLD.draft_version,OLD.intent,OLD.remove,OLD.content_sha256,OLD.created_by,OLD.created_at)
 OR OLD.status<>'pending' THEN
  RAISE EXCEPTION 'Corridor proposal inputs and completed reviews are immutable';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_corridor_proposal BEFORE UPDATE ON tour_corridor_proposals FOR EACH ROW EXECUTE FUNCTION retain_corridor_proposal();

-- Recompute or legacy metadata saves must not implicitly release a newly
-- reviewed corridor. Only an explicit publication can first release that choice.
-- +goose StatementBegin
CREATE FUNCTION byl_corridor_is_released(tour UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(
  SELECT 1 FROM tour_corridor_current c
  LEFT JOIN tour_revision_state st ON st.tour_id=c.tour_id
  LEFT JOIN tour_revisions r ON r.id=st.current_revision_id
  WHERE c.tour_id=tour AND (
   (r.snapshot#>>'{tour,corridor,id}') IS DISTINCT FROM c.proposal_id::text
   OR COALESCE((r.snapshot#>>'{tour,corridor,version}')::bigint,0)<>c.version)
 );
$$;
-- +goose StatementEnd

-- Wrap the admitted snapshot function without changing existing snapshot rows.
ALTER FUNCTION byl_tour_snapshot(UUID) RENAME TO byl_tour_snapshot_v12;
-- +goose StatementBegin
CREATE FUNCTION byl_tour_snapshot(tour UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 WITH approved AS (
  SELECT jsonb_build_object('id',p.id,'version',c.version,'intent',p.intent,'content_sha256',p.content_sha256,'reviewed_at',p.reviewed_at) AS value
  FROM tour_corridor_current c JOIN tour_corridor_proposals p ON p.id=c.proposal_id AND p.tour_id=c.tour_id
  WHERE c.tour_id=tour AND p.status='approved'
 ), base AS (SELECT byl_tour_snapshot_v12(tour) AS value)
 SELECT CASE WHEN EXISTS(SELECT 1 FROM approved)
  THEN jsonb_set(jsonb_set(base.value,'{tour,corridor}',(SELECT value FROM approved)),'{routing_intent,corridor}',
   (SELECT jsonb_build_object('id',value->'id','version',value->'version','content_sha256',value->'content_sha256') FROM approved))
  ELSE base.value END FROM base;
$$;
-- +goose StatementEnd

-- No corridor history preserves the exact old hash and all ready cache rows.
ALTER FUNCTION byl_route_input_hash(UUID,travel_mode) RENAME TO byl_route_input_hash_v12;
-- +goose StatementBegin
CREATE FUNCTION byl_route_input_hash(tour UUID,mode travel_mode) RETURNS TEXT LANGUAGE sql STABLE AS $$
 SELECT CASE WHEN EXISTS(SELECT 1 FROM tour_corridor_current WHERE tour_id=tour)
  THEN md5(byl_route_input_hash_v12(tour,mode)||':corridor:'||(SELECT version::text FROM tour_corridor_current WHERE tour_id=tour))
  ELSE byl_route_input_hash_v12(tour,mode) END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain reviewed corridor intent and published snapshots during image recovery';
END $$;
-- +goose StatementEnd
