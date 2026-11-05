-- +goose Up
CREATE TYPE entity_status AS ENUM ('draft', 'in_review', 'published', 'archived');
CREATE TYPE tour_visibility AS ENUM ('public', 'unlisted', 'private');
CREATE TYPE travel_mode AS ENUM ('walk', 'bike');
CREATE TYPE media_kind AS ENUM ('image', 'video', 'audio');
CREATE TYPE date_precision AS ENUM ('day', 'month', 'year', 'circa');
CREATE TYPE account_role AS ENUM ('visitor', 'curator', 'admin');
CREATE TYPE auth_provider AS ENUM ('apple', 'google', 'dev');
CREATE TYPE candidate_kind AS ENUM ('place', 'moment', 'media');
CREATE TYPE candidate_origin AS ENUM ('import', 'draft', 'user');
CREATE TYPE candidate_status AS ENUM ('pending', 'approved', 'rejected');
CREATE TYPE route_status AS ENUM ('pending', 'ready', 'failed');
CREATE TYPE job_status AS ENUM ('queued', 'running', 'done', 'failed');

CREATE TABLE cities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  region TEXT NOT NULL DEFAULT '',
  country_code TEXT NOT NULL DEFAULT 'US',
  center_lat DOUBLE PRECISION NOT NULL,
  center_lng DOUBLE PRECISION NOT NULL,
  bbox_min_lat DOUBLE PRECISION NOT NULL,
  bbox_min_lng DOUBLE PRECISION NOT NULL,
  bbox_max_lat DOUBLE PRECISION NOT NULL,
  bbox_max_lng DOUBLE PRECISION NOT NULL,
  timezone TEXT NOT NULL DEFAULT 'America/Chicago',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE topics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  city_id UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (city_id, slug)
);

CREATE TABLE media_assets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind media_kind NOT NULL,
  storage_key TEXT NOT NULL DEFAULT '',
  public_url TEXT NOT NULL DEFAULT '',
  mime_type TEXT NOT NULL DEFAULT '',
  width INTEGER,
  height INTEGER,
  duration_seconds DOUBLE PRECISION,
  caption TEXT NOT NULL DEFAULT '',
  credit TEXT NOT NULL DEFAULT '',
  license TEXT NOT NULL DEFAULT '',
  license_url TEXT NOT NULL DEFAULT '',
  source_url TEXT NOT NULL DEFAULT '',
  status entity_status NOT NULL DEFAULT 'draft',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE places (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  city_id UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'site',
  latitude DOUBLE PRECISION NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  arrival_radius_m DOUBLE PRECISION NOT NULL DEFAULT 40,
  address TEXT NOT NULL DEFAULT '',
  status entity_status NOT NULL DEFAULT 'draft',
  primary_moment_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (city_id, slug)
);

CREATE TABLE moments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id UUID NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  date_start DATE,
  date_end DATE,
  date_precision date_precision NOT NULL DEFAULT 'year',
  date_display TEXT NOT NULL DEFAULT '',
  hero_media_id UUID REFERENCES media_assets(id) ON DELETE SET NULL,
  narration_media_id UUID REFERENCES media_assets(id) ON DELETE SET NULL,
  status entity_status NOT NULL DEFAULT 'draft',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (place_id, slug)
);

ALTER TABLE places
  ADD CONSTRAINT places_primary_moment_fk
  FOREIGN KEY (primary_moment_id) REFERENCES moments(id)
  ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE moment_topics (
  moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
  topic_id UUID NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  PRIMARY KEY (moment_id, topic_id)
);

CREATE TABLE moment_sources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  title TEXT NOT NULL,
  url TEXT NOT NULL DEFAULT '',
  publisher TEXT NOT NULL DEFAULT '',
  retrieved_at DATE,
  license TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  UNIQUE (moment_id, position)
);

CREATE TABLE moment_media (
  moment_id UUID NOT NULL REFERENCES moments(id) ON DELETE CASCADE,
  media_id UUID NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  PRIMARY KEY (moment_id, media_id)
);

CREATE TABLE accounts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider auth_provider NOT NULL,
  provider_subject TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  role account_role NOT NULL DEFAULT 'visitor',
  last_sign_in_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (provider, provider_subject)
);

CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tours (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  city_id UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  topic_id UUID REFERENCES topics(id) ON DELETE SET NULL,
  modes travel_mode[] NOT NULL DEFAULT ARRAY['walk', 'bike']::travel_mode[],
  hero_media_id UUID REFERENCES media_assets(id) ON DELETE SET NULL,
  status entity_status NOT NULL DEFAULT 'draft',
  visibility tour_visibility NOT NULL DEFAULT 'public',
  owner_account_id UUID REFERENCES accounts(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tour_stops (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tour_id UUID NOT NULL REFERENCES tours(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  place_id UUID NOT NULL REFERENCES places(id) ON DELETE RESTRICT,
  featured_moment_id UUID REFERENCES moments(id) ON DELETE SET NULL,
  note TEXT NOT NULL DEFAULT '',
  UNIQUE (tour_id, position)
);

CREATE TABLE tour_routes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tour_id UUID NOT NULL REFERENCES tours(id) ON DELETE CASCADE,
  mode travel_mode NOT NULL,
  status route_status NOT NULL DEFAULT 'pending',
  provider TEXT NOT NULL DEFAULT '',
  distance_m DOUBLE PRECISION NOT NULL DEFAULT 0,
  duration_s INTEGER NOT NULL DEFAULT 0,
  geometry JSONB,
  legs JSONB,
  error TEXT NOT NULL DEFAULT '',
  computed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tour_id, mode)
);

CREATE TABLE saved_tours (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  tour_id UUID NOT NULL REFERENCES tours(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, tour_id)
);

CREATE TABLE tour_progress (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  tour_id UUID NOT NULL REFERENCES tours(id) ON DELETE CASCADE,
  mode travel_mode NOT NULL DEFAULT 'walk',
  visited_positions INTEGER[] NOT NULL DEFAULT '{}',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, tour_id)
);

CREATE TABLE candidates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind candidate_kind NOT NULL,
  origin candidate_origin NOT NULL,
  source_name TEXT,
  external_id TEXT,
  submitted_by_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
  target_place_id UUID REFERENCES places(id) ON DELETE SET NULL,
  target_moment_id UUID REFERENCES moments(id) ON DELETE SET NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  status candidate_status NOT NULL DEFAULT 'pending',
  reviewer_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
  review_note TEXT NOT NULL DEFAULT '',
  approved_entity_id UUID,
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  status job_status NOT NULL DEFAULT 'queued',
  attempts INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 3,
  run_after TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX places_lat_lng_idx ON places (latitude, longitude);
CREATE INDEX places_city_status_idx ON places (city_id, status);
CREATE INDEX moments_place_idx ON moments (place_id, status);
CREATE INDEX tours_city_status_visibility_idx ON tours (city_id, status, visibility);
CREATE INDEX tours_owner_idx ON tours (owner_account_id);
CREATE INDEX tour_stops_tour_idx ON tour_stops (tour_id, position);
CREATE INDEX sessions_account_idx ON sessions (account_id);
CREATE INDEX candidates_status_idx ON candidates (status, kind);
CREATE UNIQUE INDEX candidates_source_external_idx ON candidates (source_name, external_id)
  WHERE source_name IS NOT NULL AND external_id IS NOT NULL;
CREATE INDEX jobs_status_run_after_idx ON jobs (status, run_after);

-- +goose Down
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS candidates;
DROP TABLE IF EXISTS tour_progress;
DROP TABLE IF EXISTS saved_tours;
DROP TABLE IF EXISTS tour_routes;
DROP TABLE IF EXISTS tour_stops;
DROP TABLE IF EXISTS tours;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS moment_media;
DROP TABLE IF EXISTS moment_sources;
DROP TABLE IF EXISTS moment_topics;
ALTER TABLE places DROP CONSTRAINT IF EXISTS places_primary_moment_fk;
DROP TABLE IF EXISTS moments;
DROP TABLE IF EXISTS places;
DROP TABLE IF EXISTS media_assets;
DROP TABLE IF EXISTS topics;
DROP TABLE IF EXISTS cities;
DROP TYPE IF EXISTS job_status;
DROP TYPE IF EXISTS route_status;
DROP TYPE IF EXISTS candidate_status;
DROP TYPE IF EXISTS candidate_origin;
DROP TYPE IF EXISTS candidate_kind;
DROP TYPE IF EXISTS auth_provider;
DROP TYPE IF EXISTS account_role;
DROP TYPE IF EXISTS date_precision;
DROP TYPE IF EXISTS media_kind;
DROP TYPE IF EXISTS travel_mode;
DROP TYPE IF EXISTS tour_visibility;
DROP TYPE IF EXISTS entity_status;
