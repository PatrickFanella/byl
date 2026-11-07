-- +goose Up
CREATE TABLE reconciliation_records (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 identity_key TEXT NOT NULL UNIQUE,
 proposal JSONB NOT NULL,
 place_candidate_id UUID REFERENCES candidates(id) ON DELETE RESTRICT,
 moment_candidate_id UUID REFERENCES candidates(id) ON DELETE RESTRICT,
 review_version INTEGER NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE reconciliation_sources (
 id BIGSERIAL PRIMARY KEY,
 record_id UUID NOT NULL REFERENCES reconciliation_records(id) ON DELETE RESTRICT,
 brief_id UUID NOT NULL REFERENCES route_briefs(id) ON DELETE RESTRICT,
 run_id UUID REFERENCES brief_research_runs(id) ON DELETE RESTRICT,
 source_key TEXT NOT NULL,
 source JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (record_id,brief_id,source_key)
);
CREATE INDEX reconciliation_sources_brief_idx ON reconciliation_sources (brief_id,record_id);
CREATE TABLE reconciliation_decisions (
 record_id UUID NOT NULL REFERENCES reconciliation_records(id) ON DELETE RESTRICT,
 version INTEGER NOT NULL,
 decision JSONB NOT NULL,
 reviewer_id UUID NOT NULL REFERENCES accounts(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (record_id,version)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Reconciliation contains retained curator decisions and provenance; keep this additive schema during image rollback';
END $$;
-- +goose StatementEnd
