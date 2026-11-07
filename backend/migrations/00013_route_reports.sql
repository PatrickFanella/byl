-- +goose Up
CREATE TABLE route_reports (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_account_id UUID NOT NULL REFERENCES accounts(id),
 tour_id UUID NOT NULL,
 revision_id UUID NOT NULL,
 submitted JSONB NOT NULL CHECK(jsonb_typeof(submitted)='object'),
 CHECK(submitted->>'tour_id'=tour_id::text AND submitted->>'revision_id'=revision_id::text),
 status TEXT NOT NULL DEFAULT 'review' CHECK(status IN ('draft','processing','needs_research','review','accepted','declined')),
 reason TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),
 submission_key TEXT,
 submission_hash TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tour_id,revision_id) REFERENCES tour_revisions(tour_id,id),
 UNIQUE(owner_account_id,submission_key)
);
CREATE INDEX route_reports_owner_created_idx ON route_reports(owner_account_id,created_at DESC,id);
CREATE INDEX route_reports_created_idx ON route_reports(created_at DESC,id);
-- +goose StatementBegin
CREATE FUNCTION retain_route_report_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.owner_account_id,NEW.tour_id,NEW.revision_id,NEW.submitted,NEW.submission_key,NEW.submission_hash,NEW.created_at)
 IS DISTINCT FROM (OLD.owner_account_id,OLD.tour_id,OLD.revision_id,OLD.submitted,OLD.submission_key,OLD.submission_hash,OLD.created_at) THEN
  RAISE EXCEPTION 'Route report submissions are immutable';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_route_report_input BEFORE UPDATE ON route_reports FOR EACH ROW EXECUTE FUNCTION retain_route_report_input();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain private route reports during application rollback';
END $$;
-- +goose StatementEnd
