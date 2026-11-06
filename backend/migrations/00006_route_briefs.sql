-- +goose Up
CREATE TABLE route_briefs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_account_id UUID NOT NULL REFERENCES accounts(id),
  submitted JSONB NOT NULL CHECK (jsonb_typeof(submitted) = 'object'),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'processing', 'needs_research', 'review', 'accepted', 'declined')),
  reason TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  submission_key TEXT,
  submission_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_account_id, submission_key)
);
CREATE INDEX route_briefs_owner_created_idx ON route_briefs(owner_account_id, created_at DESC);
CREATE INDEX route_briefs_created_idx ON route_briefs(created_at DESC, id);

-- +goose Down
-- Retain private submissions when rolling back the application image.
-- +goose StatementBegin
DO $$ BEGIN
  RAISE EXCEPTION 'Back up and review private route briefs before any destructive schema rollback';
END $$;
-- +goose StatementEnd
