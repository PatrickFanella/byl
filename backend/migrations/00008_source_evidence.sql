-- +goose Up
CREATE TABLE source_evidence (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  brief_id UUID NOT NULL REFERENCES route_briefs(id),
  brief_version INTEGER NOT NULL CHECK (brief_version > 0),
  job_id UUID UNIQUE REFERENCES jobs(id),
  source_key TEXT NOT NULL CHECK (length(source_key) = 64),
  url TEXT NOT NULL,
  reference TEXT NOT NULL DEFAULT '',
  source_kind TEXT NOT NULL CHECK (source_kind IN ('page','catalog')),
  request_key TEXT,
  request_hash TEXT NOT NULL,
  result JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (brief_id, request_key)
);
CREATE INDEX source_evidence_brief_idx ON source_evidence(brief_id,created_at DESC,id);
CREATE INDEX source_evidence_source_idx ON source_evidence(source_key);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  RAISE EXCEPTION 'Retain source evidence and review later curator work before destructive rollback';
END $$;
-- +goose StatementEnd
