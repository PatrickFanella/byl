-- +goose Up
CREATE TABLE place_arrival_proposals (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 place_id UUID NOT NULL REFERENCES places(id) ON DELETE RESTRICT,
 base_version INTEGER NOT NULL CHECK (base_version >= 0),
 point JSONB NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
 created_by UUID NOT NULL REFERENCES accounts(id),
 reviewed_by UUID REFERENCES accounts(id),
 review_note TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 reviewed_at TIMESTAMPTZ,
 CHECK ((status='pending' AND reviewed_by IS NULL AND reviewed_at IS NULL) OR
        (status<>'pending' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL AND length(trim(review_note))>0)),
 UNIQUE (place_id,id)
);
CREATE INDEX place_arrival_proposals_place_idx ON place_arrival_proposals(place_id,created_at,id);
-- +goose StatementBegin
CREATE FUNCTION retain_arrival_proposal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.status <> 'pending' OR NEW.id IS DISTINCT FROM OLD.id OR
    NEW.place_id IS DISTINCT FROM OLD.place_id OR NEW.base_version IS DISTINCT FROM OLD.base_version OR
    NEW.point IS DISTINCT FROM OLD.point OR NEW.created_by IS DISTINCT FROM OLD.created_by OR
    NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.status NOT IN ('approved','rejected') THEN
  RAISE EXCEPTION 'Arrival proposal input and completed reviews are immutable';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_arrival_proposal BEFORE UPDATE ON place_arrival_proposals
 FOR EACH ROW EXECUTE FUNCTION retain_arrival_proposal();
CREATE TABLE place_arrival_current (
 place_id UUID PRIMARY KEY REFERENCES places(id) ON DELETE RESTRICT,
 proposal_id UUID NOT NULL UNIQUE,
 version INTEGER NOT NULL CHECK (version > 0),
 FOREIGN KEY (place_id,proposal_id) REFERENCES place_arrival_proposals(place_id,id) ON DELETE RESTRICT
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain reviewed arrival points and source context during image rollback';
END $$;
-- +goose StatementEnd
