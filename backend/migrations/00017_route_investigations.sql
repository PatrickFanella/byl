-- +goose Up
CREATE TABLE route_investigations (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 report_id UUID NOT NULL REFERENCES route_reports(id),
 job_id UUID NOT NULL REFERENCES jobs(id),
 created_by UUID NOT NULL REFERENCES accounts(id),
 request_key TEXT,
 request_hash TEXT NOT NULL,
 input JSONB NOT NULL CHECK(jsonb_typeof(input)='object'),
 result JSONB CHECK(result IS NULL OR jsonb_typeof(result)='object'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(created_by,request_key)
);
CREATE INDEX route_investigations_report_created_idx ON route_investigations(report_id,created_at DESC,id);
CREATE INDEX route_reports_related_idx ON route_reports(revision_id,(submitted->>'mode'),(submitted->>'direction'),(submitted->'affected'));
-- +goose StatementBegin
CREATE FUNCTION retain_route_investigation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'Retain route investigation checkpoints';
 END IF;
 IF (NEW.id,NEW.report_id,NEW.job_id,NEW.created_by,NEW.request_key,NEW.request_hash,NEW.input,NEW.created_at)
 IS DISTINCT FROM (OLD.id,OLD.report_id,OLD.job_id,OLD.created_by,OLD.request_key,OLD.request_hash,OLD.input,OLD.created_at)
 OR (OLD.result IS NOT NULL AND NEW.result IS DISTINCT FROM OLD.result) THEN
  RAISE EXCEPTION 'Route investigation checkpoints are immutable';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_route_investigation BEFORE UPDATE OR DELETE ON route_investigations FOR EACH ROW EXECUTE FUNCTION retain_route_investigation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain private route investigations during application rollback';
END $$;
-- +goose StatementEnd
