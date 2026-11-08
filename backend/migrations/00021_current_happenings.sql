-- +goose Up
-- Optional current notices have their own history. They never mutate moments,
-- stop progress, historical snapshots or the tour publication pointers.
CREATE TABLE happenings (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 place_id UUID NOT NULL REFERENCES places(id),
 created_by UUID NOT NULL REFERENCES accounts(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX happenings_place_idx ON happenings(place_id,id);
CREATE INDEX source_evidence_latest_source_idx ON source_evidence(source_key,created_at DESC,id DESC);

CREATE TABLE happening_drafts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 happening_id UUID NOT NULL REFERENCES happenings(id),
 version BIGINT NOT NULL CHECK(version>0),
 content JSONB NOT NULL CHECK(jsonb_typeof(content)='object'),
 created_by UUID NOT NULL REFERENCES accounts(id),
 request_key TEXT NOT NULL CHECK(length(request_key) BETWEEN 16 AND 128),
 request_sha256 TEXT NOT NULL CHECK(length(request_sha256)=64),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(happening_id,version),
 UNIQUE(happening_id,id),
 UNIQUE(created_by,request_key)
);
CREATE INDEX happening_drafts_author_idx ON happening_drafts(created_by,created_at);

CREATE TABLE happening_reviews (
 draft_id UUID PRIMARY KEY REFERENCES happening_drafts(id),
 reviewed_by UUID NOT NULL REFERENCES accounts(id),
 action TEXT NOT NULL CHECK(action IN ('approve','reject')),
 package_sha256 TEXT NOT NULL CHECK(length(package_sha256)=64),
 note TEXT NOT NULL CHECK(length(trim(note)) BETWEEN 1 AND 4000),
 reviewed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE happening_revisions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 happening_id UUID NOT NULL REFERENCES happenings(id),
 draft_id UUID NOT NULL UNIQUE REFERENCES happening_reviews(draft_id),
 public_snapshot JSONB NOT NULL CHECK(jsonb_typeof(public_snapshot)='object'),
 private_package JSONB NOT NULL CHECK(jsonb_typeof(private_package)='object'),
 published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(happening_id,id),
 FOREIGN KEY(happening_id,draft_id) REFERENCES happening_drafts(happening_id,id)
);
CREATE TABLE happening_current (
 happening_id UUID PRIMARY KEY REFERENCES happenings(id),
 draft_id UUID NOT NULL,
 published_revision_id UUID,
 FOREIGN KEY(happening_id,draft_id) REFERENCES happening_drafts(happening_id,id),
 FOREIGN KEY(happening_id,published_revision_id) REFERENCES happening_revisions(happening_id,id)
);

-- +goose StatementBegin
CREATE FUNCTION retain_happening_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Happening drafts, reviews and public revisions are immutable; create and review a new draft'; END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_happening_draft BEFORE UPDATE OR DELETE ON happening_drafts FOR EACH ROW EXECUTE FUNCTION retain_happening_history();
CREATE TRIGGER retain_happening_review BEFORE UPDATE OR DELETE ON happening_reviews FOR EACH ROW EXECUTE FUNCTION retain_happening_history();
CREATE TRIGGER retain_happening_revision BEFORE UPDATE OR DELETE ON happening_revisions FOR EACH ROW EXECUTE FUNCTION retain_happening_history();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Retain approved temporary-notice history during image rollback'; END $$;
-- +goose StatementEnd
