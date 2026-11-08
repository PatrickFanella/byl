-- +goose Up
CREATE TABLE route_corrections (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 report_id UUID NOT NULL UNIQUE REFERENCES route_reports(id),
 investigation_id UUID NOT NULL REFERENCES route_investigations(id),
 tour_id UUID NOT NULL REFERENCES tours(id),
 previous_revision_id UUID NOT NULL REFERENCES tour_revisions(id),
 revision_id UUID NOT NULL UNIQUE REFERENCES tour_revisions(id),
 comparison_version TEXT NOT NULL CHECK(length(comparison_version)=64),
 corridor_id UUID NOT NULL REFERENCES tour_corridor_proposals(id),
 reviewed_by UUID NOT NULL REFERENCES accounts(id),
 note TEXT NOT NULL,
 temporary BOOLEAN NOT NULL,
 review_on DATE,
 potential_tours JSONB NOT NULL CHECK(jsonb_typeof(potential_tours)='array'),
 request_sha256 TEXT NOT NULL CHECK(length(request_sha256)=64),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(temporary=(review_on IS NOT NULL)),
 CHECK(previous_revision_id<>revision_id)
);
CREATE INDEX route_corrections_review_idx ON route_corrections(review_on,id) WHERE temporary;
-- +goose StatementBegin
CREATE FUNCTION retain_route_correction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'Route correction releases and reassessment dates are retained';
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_route_correction BEFORE UPDATE OR DELETE ON route_corrections FOR EACH ROW EXECUTE FUNCTION retain_route_correction();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain correction releases and prior revisions during application rollback';
END $$;
-- +goose StatementEnd
