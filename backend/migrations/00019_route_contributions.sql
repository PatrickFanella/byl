-- +goose Up
-- These private scopes cannot be joined across routes/modes by account ID.
-- There are no raw traces, exact trip/receipt times, device IDs or account FKs.
-- Operator admission is excluded from backups as well: a filtered restore must
-- not inherit collection authority from the server's environment settings.
CREATE TABLE route_contribution_custody (
 singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),
 policy_version INTEGER NOT NULL CHECK(policy_version=1),
 key_fingerprint TEXT NOT NULL CHECK(length(key_fingerprint)=64),
 admitted_on DATE NOT NULL
);

CREATE TABLE route_contribution_consents (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_scope TEXT NOT NULL CHECK(length(owner_scope)=64),
 tour_id UUID NOT NULL REFERENCES tours(id),
 revision_id UUID NOT NULL REFERENCES tour_revisions(id),
 mode TEXT NOT NULL CHECK(mode IN ('walk','bike')),
 grant_key_hash TEXT NOT NULL CHECK(length(grant_key_hash)=64),
 request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
 deletion_hash TEXT NOT NULL CHECK(length(deletion_hash)=64),
 requested_on DATE NOT NULL,
 created_on DATE NOT NULL,
 expires_on DATE NOT NULL,
 revoked_on DATE,
 UNIQUE(owner_scope,grant_key_hash),
 UNIQUE(id,owner_scope),
 CHECK(expires_on=created_on+30),
 CHECK(requested_on BETWEEN created_on-1 AND created_on)
);
CREATE INDEX route_contribution_consent_expiry_idx ON route_contribution_consents(expires_on);

CREATE TABLE route_contributions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 consent_id UUID NOT NULL,
 owner_scope TEXT NOT NULL CHECK(length(owner_scope)=64),
 segment_id TEXT NOT NULL CHECK(length(segment_id) BETWEEN 1 AND 120),
 direction TEXT NOT NULL CHECK(direction IN ('forward','reverse')),
 observation_period DATE NOT NULL,
 schema_version INTEGER NOT NULL CHECK(schema_version=1),
 detector_version TEXT NOT NULL CHECK(detector_version='uncalibrated-v1'),
 calibrated BOOLEAN NOT NULL CHECK(NOT calibrated),
 qualified_passages INTEGER NOT NULL CHECK(qualified_passages=1),
 sustained_departure_rejoins INTEGER NOT NULL CHECK(sustained_departure_rejoins BETWEEN 0 AND 1),
 submission_key_hash TEXT NOT NULL CHECK(length(submission_key_hash)=64),
 request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
 expires_on DATE NOT NULL,
 FOREIGN KEY(consent_id,owner_scope) REFERENCES route_contribution_consents(id,owner_scope) ON DELETE CASCADE,
 UNIQUE(owner_scope,submission_key_hash),
 UNIQUE(owner_scope,segment_id,direction,observation_period),
 CHECK(expires_on<=observation_period+30)
);
CREATE INDEX route_contribution_expiry_idx ON route_contributions(expires_on);

-- Unlinked, day-scoped abuse counters expire after two UTC days. No route or
-- account identifier is stored alongside them.
CREATE TABLE route_contribution_limits (
 scope TEXT PRIMARY KEY CHECK(length(scope)=64),
 count INTEGER NOT NULL CHECK(count>0),
 expires_on DATE NOT NULL
);

-- A live internal projection repairs counts immediately on withdrawal/deletion.
-- Expired/revoked rows cannot contribute while physical cleanup is pending.
CREATE VIEW route_contribution_counts AS
 SELECT c.revision_id,c.mode,r.segment_id,r.direction,r.observation_period,
        count(*) AS qualified_passages,
        sum(r.sustained_departure_rejoins) AS sustained_departure_rejoins
 FROM route_contributions r JOIN route_contribution_consents c ON c.id=r.consent_id
 WHERE c.revoked_on IS NULL AND c.expires_on>(now() AT TIME ZONE 'UTC')::date AND r.expires_on>(now() AT TIME ZONE 'UTC')::date
 GROUP BY c.revision_id,c.mode,r.segment_id,r.direction,r.observation_period;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Withdraw/purge private contributions and review backup custody before removing contribution storage';
END $$;
-- +goose StatementEnd
