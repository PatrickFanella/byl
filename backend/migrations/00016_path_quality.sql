-- +goose Up
-- Private, immutable inspection input belongs to a particular corridor proposal.
-- Public intent and snapshots contain neither source excerpts nor private IDs.
CREATE TABLE corridor_quality_evidence (
 proposal_id UUID PRIMARY KEY REFERENCES tour_corridor_proposals(id),
 refs JSONB NOT NULL CHECK(jsonb_typeof(refs)='array'),
 inspections JSONB NOT NULL CHECK(jsonb_typeof(inspections)='array'),
 content_sha256 TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(octet_length(refs::text)<=262144 AND octet_length(inspections::text)<=524288)
);
-- +goose StatementBegin
CREATE FUNCTION retain_corridor_quality_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN
  RAISE EXCEPTION 'Retain immutable corridor inspection input';
 END IF;
 PERFORM 1 FROM tour_corridor_proposals WHERE id=NEW.proposal_id AND status='pending' FOR UPDATE;
 IF NOT FOUND THEN
  RAISE EXCEPTION 'Inspection input must be captured before corridor review';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_corridor_quality_evidence BEFORE INSERT OR UPDATE OR DELETE ON corridor_quality_evidence
 FOR EACH ROW EXECUTE FUNCTION retain_corridor_quality_evidence();

CREATE TABLE tour_path_quality_reports (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 tour_id UUID NOT NULL REFERENCES tours(id),
 mode travel_mode NOT NULL,
 route_version BIGINT NOT NULL CHECK(route_version>=0),
 input_hash TEXT NOT NULL,
 report JSONB NOT NULL CHECK(jsonb_typeof(report)='object' AND octet_length(report::text)<=1048576),
 content_sha256 TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(tour_id,mode,id),
 CHECK(report->>'mode'=mode::text AND report->>'input_hash'=input_hash)
);
CREATE INDEX tour_path_quality_history_idx ON tour_path_quality_reports(tour_id,mode,created_at DESC,id);
-- +goose StatementBegin
CREATE FUNCTION retain_path_quality_report() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'Retain immutable path-quality evidence';
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_path_quality_report BEFORE UPDATE OR DELETE ON tour_path_quality_reports
 FOR EACH ROW EXECUTE FUNCTION retain_path_quality_report();

CREATE TABLE tour_path_quality_current (
 tour_id UUID NOT NULL,
 mode travel_mode NOT NULL,
 report_id UUID NOT NULL,
 PRIMARY KEY(tour_id,mode),
 FOREIGN KEY(tour_id,mode) REFERENCES tour_routes(tour_id,mode) ON DELETE CASCADE,
 FOREIGN KEY(tour_id,mode,report_id) REFERENCES tour_path_quality_reports(tour_id,mode,id)
);
-- Any route writer, including an older application, invalidates its quality
-- pointer. A qualified worker attaches a new checkpoint in the same transaction.
-- +goose StatementBegin
CREATE FUNCTION invalidate_path_quality() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 DELETE FROM tour_path_quality_current WHERE tour_id=NEW.tour_id AND mode=NEW.mode;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER invalidate_path_quality BEFORE UPDATE ON tour_routes FOR EACH ROW EXECUTE FUNCTION invalidate_path_quality();

-- +goose StatementBegin
CREATE FUNCTION byl_path_quality_passes(tour UUID,requested travel_mode) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM tour_path_quality_current c
 JOIN tour_path_quality_reports q ON q.id=c.report_id AND q.tour_id=c.tour_id AND q.mode=c.mode
 JOIN tour_revision_state st ON st.tour_id=c.tour_id
 JOIN tour_routes r ON r.tour_id=c.tour_id AND r.mode=c.mode
 WHERE c.tour_id=tour AND c.mode=requested AND st.deleted_at IS NULL
 AND q.route_version=st.route_version AND q.input_hash=byl_route_input_hash(tour,requested)
 AND q.report->>'status'='pass' AND q.report->>'version'='1'
 AND jsonb_typeof(q.report->'issues')='array'
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(q.report->'issues')='array' THEN q.report->'issues' ELSE '[]'::jsonb END) issue WHERE issue->>'severity' IN ('blocked','needs_research'))
 AND r.status='ready' AND r.provider='valhalla' AND q.report->>'provider'=r.provider);
$$;
-- +goose StatementEnd
-- Public/working summaries exclude private inspection excerpts and identifiers.
-- Input freshness is evaluated on read; captured reports remain immutable.
-- +goose StatementBegin
CREATE FUNCTION byl_path_quality_summary(tour UUID,requested travel_mode) RETURNS JSONB LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('report_id',q.id,'version',q.report->'version','status',q.report->'status',
  'issues',q.report->'issues','checked_at',q.report->'checked_at',
  'input_current',st.deleted_at IS NULL AND q.route_version=st.route_version AND q.input_hash=byl_route_input_hash(tour,requested))
 FROM tour_path_quality_current c JOIN tour_path_quality_reports q ON q.id=c.report_id
 JOIN tour_revision_state st ON st.tour_id=c.tour_id
 WHERE c.tour_id=tour AND c.mode=requested;
$$;
-- +goose StatementEnd
-- Existing published revisions are untouched. New inserts require quality even
-- when attempted by a previous API that does not implement the new gate.
-- +goose StatementBegin
CREATE FUNCTION require_revision_path_quality() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM tours t CROSS JOIN LATERAL unnest(t.modes) m
  WHERE t.id=NEW.tour_id AND NOT byl_path_quality_passes(t.id,m)) THEN
  RAISE EXCEPTION 'New tour publication requires current passing path quality for every mode';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER require_revision_path_quality BEFORE INSERT ON tour_revisions
 FOR EACH ROW EXECUTE FUNCTION require_revision_path_quality();

ALTER FUNCTION byl_tour_snapshot(UUID) RENAME TO byl_tour_snapshot_v15;
-- +goose StatementBegin
CREATE FUNCTION byl_tour_snapshot(tour UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 WITH base AS (SELECT byl_tour_snapshot_v15(tour) AS value), quality AS (
  SELECT c.mode::text AS mode,byl_path_quality_summary(tour,c.mode) AS summary
  FROM tour_path_quality_current c
  WHERE c.tour_id=tour
 )
 SELECT CASE WHEN EXISTS(SELECT 1 FROM quality) THEN jsonb_set(base.value,'{routes}',
  (SELECT jsonb_object_agg(entry.key,CASE WHEN quality.summary IS NOT NULL
    THEN entry.value||jsonb_build_object('quality',quality.summary) ELSE entry.value END)
   FROM jsonb_each(base.value->'routes') entry LEFT JOIN quality ON quality.mode=entry.key))
 ELSE base.value END FROM base;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain inspected path evidence, quality history and publication gates during image recovery';
END $$;
-- +goose StatementEnd
