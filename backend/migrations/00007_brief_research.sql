-- +goose Up
CREATE TABLE brief_research_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  brief_id UUID NOT NULL REFERENCES route_briefs(id),
  job_id UUID UNIQUE REFERENCES jobs(id),
  submitted JSONB NOT NULL,
  source_version INTEGER NOT NULL,
  brief_version INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','completed','needs_research','cancelled')),
  reason TEXT NOT NULL DEFAULT '',
  requests_used INTEGER NOT NULL DEFAULT 0 CHECK (requests_used >= 0),
  tokens_reserved INTEGER NOT NULL DEFAULT 0 CHECK (tokens_reserved >= 0),
  request_limit INTEGER NOT NULL DEFAULT 12 CHECK (request_limit BETWEEN 1 AND 12),
  token_limit INTEGER NOT NULL DEFAULT 20000 CHECK (token_limit BETWEEN 0 AND 20000),
  deadline_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '15 minutes',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (brief_id, source_version)
);
CREATE UNIQUE INDEX brief_research_active_idx ON brief_research_runs(brief_id) WHERE status = 'active';
CREATE INDEX brief_research_brief_idx ON brief_research_runs(brief_id,created_at DESC);
CREATE TABLE brief_research_stages (
  run_id UUID NOT NULL REFERENCES brief_research_runs(id),
  stage TEXT NOT NULL CHECK (stage IN ('research','reconciliation','drafting','assembly')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','needs_research')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
  reason TEXT NOT NULL DEFAULT '',
  output JSONB NOT NULL DEFAULT '{}',
  candidate_ids UUID[] NOT NULL DEFAULT '{}',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id,stage)
);
CREATE TABLE brief_research_attempts (
  run_id UUID NOT NULL,
  stage TEXT NOT NULL,
  attempt INTEGER NOT NULL CHECK (attempt BETWEEN 1 AND 3),
  status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','completed','failed','interrupted','cancelled')),
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ,
  PRIMARY KEY (run_id,stage,attempt),
  FOREIGN KEY (run_id,stage) REFERENCES brief_research_stages(run_id,stage)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  RAISE EXCEPTION 'Retain research checkpoints and review private data before destructive rollback';
END $$;
-- +goose StatementEnd
