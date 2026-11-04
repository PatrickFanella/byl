# Invisible Museum: Tours Content Model and API

Date: 2026-11-04
Status: approved in conversation, implementation in progress
Cycle: 1 of 5 (prune + content model + API)

## 1. Product direction

Invisible Museum is being rebuilt around one idea: your city is a giant museum,
and a **tour** is a walking or biking route through it organised around a
**topic**. Every stop on a tour is a **place**. A place can carry many
**moments** (events, people, photographs, film, oral history). A tour features
one moment per stop, chosen because it fits the topic, and the rest stay
browsable from that stop.

Two kinds of tours exist: **curated** topic tours, built by curators from
sourced and reviewed history, and **user routes**, which any signed-in person
can assemble from published places and share by link.

AR is gone. Navigation is an in-app route line with arrival detection, a
hand-off to the platform maps app for turn-by-turn, and an audio-guide mode
where the featured moment is narrated on arrival.

Clients are native per platform: SwiftUI (iOS), Jetpack Compose (Android), and
a web app. All three talk to one Go backend over one HTTP API.

### Cycles

1. **This cycle.** Remove AR. Wipe and rebuild the backend content model and API.
2. Sourcing pipeline (open-archive adapters, LLM drafting through the Claude
   API and any OpenAI-compatible endpoint) and the admin/review web UI.
3. iOS rebuild on the new API.
4. Web visitor app: browse tours, build and share routes.
5. Android.

Decisions already taken for later cycles that shape this one: content is
sourced from open archives plus local Chicago-area archives through a common
adapter interface, drafts are reviewed by a human before publishing, and user
routes order existing published places only.

## 2. Scope of this cycle

In:

- Delete AR code and the React mockup (done, committed separately).
- Replace the backend schema, models, repository, handlers, and tests.
- Keep the `/api/v1` path. Nothing depends on the old contract.
- Seed Downers Grove in the new model.
- Background jobs for route computation and narration.
- Accounts (Sign in with Apple, Google, and a dev-only email login).
- User routes, saved tours, and tour progress.
- The unified review queue table and its admin endpoints.
- An OpenAPI 3.1 document checked against the router in tests.
- Updated Makefile, compose files, deployment notes, README, CI.

Out (later cycles):

- Source adapters and LLM drafting (the `candidates` table and admin review
  endpoints land now; nothing fills the queue automatically yet).
- Any client work. The iOS app will not run against the new API until cycle 3.
- Public gallery of user routes, likes, comments.
- Media transcoding and image resizing.

## 3. Domain model

All tables use UUID primary keys (`gen_random_uuid()`), `created_at` and
`updated_at` timestamps. Human-facing entities also have a unique `slug`.
Coordinates are plain `latitude`/`longitude` doubles with a btree index;
nearby queries use a bounding-box prefilter plus haversine in SQL. PostGIS is
deliberately not used.

### Enumerations

- `entity_status`: `draft`, `in_review`, `published`, `archived`
- `tour_visibility`: `public`, `unlisted`, `private`
- `travel_mode`: `walk`, `bike`
- `media_kind`: `image`, `video`, `audio`
- `date_precision`: `day`, `month`, `year`, `circa`
- `account_role`: `visitor`, `curator`, `admin`
- `auth_provider`: `apple`, `google`, `dev`
- `candidate_kind`: `place`, `moment`, `media`
- `candidate_origin`: `import`, `draft`, `user`
- `candidate_status`: `pending`, `approved`, `rejected`
- `route_status`: `pending`, `ready`, `failed`
- `job_status`: `queued`, `running`, `done`, `failed`

### Tables

**cities** — `slug`, `name`, `region` (e.g. "Illinois"), `country_code`,
`center_lat`, `center_lng`, `bbox_min_lat`, `bbox_min_lng`, `bbox_max_lat`,
`bbox_max_lng`, `timezone`.

**topics** — `city_id`, `slug`, `name`, `description`. Unique on
`(city_id, slug)`. A topic is a museum wing.

**places** — `city_id`, `slug`, `name`, `summary`, `kind` (free text,
seeded values: `building`, `park`, `street`, `station`, `monument`, `site`),
`latitude`, `longitude`, `arrival_radius_m` (default 40), `address`,
`status`, `primary_moment_id` (nullable, deferred FK to moments).
Unique on `(city_id, slug)`.

**moments** — `place_id`, `slug`, `title`, `body` (Markdown), `date_start`
(date, nullable), `date_end` (date, nullable), `date_precision`,
`date_display` (text shown to users, e.g. "Summer 1923"), `hero_media_id`
(nullable), `narration_media_id` (nullable), `status`. Unique on
`(place_id, slug)`.

**moment_topics** — `moment_id`, `topic_id`. Primary key on both.

**moment_sources** — `moment_id`, `position`, `title`, `url`, `publisher`,
`retrieved_at` (date, nullable), `license`, `note`.

**media_assets** — `kind`, `storage_key`, `public_url`, `mime_type`,
`width`, `height` (nullable ints), `duration_seconds` (nullable), `caption`,
`credit`, `license` (text identifier such as `CC-BY-4.0`, `CC0-1.0`,
`PD-US`, `all-rights-reserved`), `license_url`, `source_url`,
`status` (same `entity_status`). A media asset cannot be set to `published`
with an empty `license`.

**moment_media** — `moment_id`, `media_id`, `position`. Primary key on
`(moment_id, media_id)`.

**tours** — `city_id`, `slug`, `title`, `summary`, `topic_id` (nullable for
user routes), `modes` (`travel_mode[]`, default `{walk,bike}`),
`hero_media_id` (nullable), `status`, `visibility`, `owner_account_id`
(nullable). Curated tours: owner null, visibility `public`. User routes:
owner set, visibility `unlisted` or `private`, status `published` on creation.
Unique on `slug` globally; user route slugs are random 10-character strings.

**tour_stops** — `tour_id`, `position`, `place_id`, `featured_moment_id`
(nullable), `note`. Unique on `(tour_id, position)`. When
`featured_moment_id` is null, the API returns the place's primary moment as the
featured moment.

**tour_routes** — `tour_id`, `mode`, `status`, `provider`, `distance_m`,
`duration_s`, `geometry` (jsonb GeoJSON LineString), `legs` (jsonb array of
`{from_position, to_position, distance_m, duration_s, geometry}`),
`error`, `computed_at`. Unique on `(tour_id, mode)`.

**accounts** — `provider`, `provider_subject`, `email`, `display_name`,
`role`, `last_sign_in_at`. Unique on `(provider, provider_subject)`.

**sessions** — `token_hash` (SHA-256 of the bearer token, unique),
`account_id`, `expires_at`, `last_used_at`. Tokens are 32 random bytes,
base64url encoded, valid 90 days.

**saved_tours** — `account_id`, `tour_id`. Primary key on both.

**tour_progress** — `account_id`, `tour_id`, `mode`, `visited_positions`
(`int[]`), `started_at`, `completed_at` (nullable). Primary key on
`(account_id, tour_id)`.

**candidates** — the unified review queue. `kind`, `origin`, `source_name`
(adapter name, nullable), `external_id` (nullable), `submitted_by_account_id`
(nullable), `target_place_id` (nullable), `target_moment_id` (nullable, for
media candidates), `payload` (jsonb, shaped like the create request for the
target kind), `status`, `reviewer_account_id`, `review_note`,
`approved_entity_id` (nullable), `reviewed_at`. Unique on
`(source_name, external_id)` where both are non-null.

**jobs** — `kind`, `payload` (jsonb), `status`, `attempts`, `max_attempts`
(default 3), `run_after`, `last_error`, `started_at`, `finished_at`. Index on
`(status, run_after)`.

### Derived values

- A place's `moment_count` and a tour's `stop_count`, `distance_m`, and
  `duration_s` per mode are computed in queries, never stored on the parent.
- `estimated_minutes` for a tour and mode is `duration_s / 60` from the ready
  route, rounded up. Absent when the route is not ready.

### What was dropped and why

Accent hue, downloaded flag, likes, confidence, AR offsets, contributor count,
and the exhibit kinds `artifact`, `timeline`, `thread`. Media kind lives on
the asset. Confidence is replaced by source citations, which can be inspected.

## 4. API

Base path `/api/v1`. JSON everywhere. Timestamps are RFC 3339. IDs are UUID
strings. Slugs are accepted wherever an ID is, for cities, places, topics, and
tours. Pagination uses `?limit=` (default 50, max 200) and `?cursor=` on list
endpoints that can grow unbounded (places, moments, candidates, jobs).

### Public

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/healthz` | `{ "status": "ok" }`; also checks the database |
| GET | `/cities` | list |
| GET | `/cities/{city}` | city plus its topics |
| GET | `/cities/{city}/places` | published places; `?near=lat,lng&radius_m=` or `?bbox=minLng,minLat,maxLng,maxLat`; `?topic=` |
| GET | `/cities/{city}/tours` | public published tours; `?topic=`, `?mode=` |
| GET | `/places/{place}` | published place with moment summaries and primary moment |
| GET | `/places/{place}/moments` | published moments in date order |
| GET | `/moments/{id}` | full moment: body, media, sources, topics, narration |
| GET | `/topics/{id}` | topic with its moment count |
| GET | `/tours/{tour}` | published tour with stops, featured moments, route summaries per mode; unlisted tours resolve by slug, private ones only for their owner |
| GET | `/tours/{tour}/route?mode=` | full route geometry and legs |
| GET | `/media/{id}` | asset metadata |

### Auth

| Method | Path | Notes |
| --- | --- | --- |
| POST | `/auth/apple` | `{ "identity_token", "display_name"? }` → session |
| POST | `/auth/google` | `{ "id_token" }` → session |
| POST | `/auth/dev` | `{ "email", "display_name"? }`; only when `DEV_AUTH_ENABLED=true` |
| GET | `/me` | current account |
| DELETE | `/auth/session` | revoke current token |

Session response: `{ "token", "expires_at", "account" }`. Clients send
`Authorization: Bearer <token>`.

Apple identity tokens are verified against Apple's JWKS with audience in
`APPLE_CLIENT_IDS`. Google ID tokens against Google's JWKS with audience in
`GOOGLE_CLIENT_IDS`. JWKS are cached for one hour. Accounts whose email is in
`ADMIN_BOOTSTRAP_EMAILS` get the `admin` role on first sign-in.

### Signed-in visitor

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/me/tours` | own user routes |
| POST | `/me/tours` | `{ city, title, summary?, modes?, visibility?, stops: [{ place_id }] }`; enqueues route jobs |
| GET | `/me/tours/{id}` | own route, any visibility |
| PATCH | `/me/tours/{id}` | title, summary, modes, visibility, stops; stop changes re-enqueue routes |
| DELETE | `/me/tours/{id}` | |
| GET | `/me/saved` | saved tours |
| PUT | `/me/saved/{tour}` | |
| DELETE | `/me/saved/{tour}` | |
| GET | `/me/progress` | all progress rows |
| GET | `/me/progress/{tour}` | |
| PUT | `/me/progress/{tour}` | `{ mode, visited_positions, completed }` |

Stops in user routes must reference published places in the same city, with
2 to 30 stops.

### Curator and admin (`/admin`, role `curator` or `admin`)

CRUD for cities, topics, places, moments, media, and tours, with these shapes:

- `POST/PATCH /admin/cities`, `/admin/topics`, `/admin/places`,
  `/admin/moments`, `/admin/tours`; `GET` lists include every status;
  `DELETE` archives rather than deletes, except for tours which delete.
- `PUT /admin/tours/{id}/stops` replaces the stop list in one call and
  enqueues route jobs for every mode.
- `POST /admin/places/{id}/publish`, `/admin/moments/{id}/publish`,
  `/admin/tours/{id}/publish`, and matching `/unpublish`. Publishing a moment
  enqueues narration. Publishing a tour requires every stop's place to be
  published and every route to be `ready`; otherwise 409 with the reason.
- `POST /admin/tours/{id}/routes/recompute`.
- `POST /admin/media` multipart upload (`file`, plus metadata fields) → asset
  stored through the blob store. `PATCH /admin/media/{id}` metadata.
- `PUT /admin/moments/{id}/media` ordered list of asset IDs.
  `PUT /admin/moments/{id}/sources` ordered sources.
  `PUT /admin/moments/{id}/topics` topic IDs.
- Review queue: `GET /admin/candidates?status=&kind=`, `GET /admin/candidates/{id}`,
  `POST /admin/candidates` (manual entry), `POST /admin/candidates/{id}/approve`
  with an optional payload override, which creates the target entity in
  `draft` status and records `approved_entity_id`; `POST /admin/candidates/{id}/reject`
  with a note.
- Jobs: `GET /admin/jobs?status=`, `POST /admin/jobs/{id}/retry`.
- Accounts (admin only): `GET /admin/accounts`, `PATCH /admin/accounts/{id}/role`.

### Errors

Every error is `{ "error": { "code": "<snake_case>", "message": "<human>" } }`.
Codes in use: `bad_request`, `validation_failed` (with a `fields` map),
`unauthorized`, `forbidden`, `not_found`, `conflict`, `internal`. Validation
problems are 422, malformed JSON is 400.

### OpenAPI

`backend/api/openapi.yaml` is the contract for all clients. A test walks the
chi router and fails if any route is missing from the document, or the
document lists a route the router does not serve.

## 5. Background jobs

A `jobs` table and an in-process worker. The worker runs in the API binary,
polls every two seconds with `SELECT ... FOR UPDATE SKIP LOCKED`, runs one
job at a time per worker goroutine (two goroutines by default), and retries
with exponential backoff up to `max_attempts`. Failures keep `last_error`.

Job kinds in this cycle:

- `compute_route` `{ tour_id, mode }`: loads stops, calls the routing
  provider, writes `tour_routes`. Replaces any existing row for that tour and
  mode. Marks the route `failed` with the error after the final attempt.
- `generate_narration` `{ moment_id }`: renders the moment body to plain
  text, calls the narration provider, stores the audio through the blob store,
  creates a `media_assets` row (kind `audio`, license `all-rights-reserved`,
  credit "Invisible Museum narration"), and sets `narration_media_id`. If the
  provider is `none`, the job completes without creating anything and clients
  use on-device speech.

Jobs are enqueued inside the same transaction as the change that needs them,
so a tour never has stops without a route job.

## 6. Providers

Each external dependency is an interface in its own package with one real
implementation and one local implementation, selected by configuration.

**Routing** (`internal/routing`): `Route(ctx, mode, waypoints) (Route, error)`.

- `valhalla`: HTTP client for a self-hosted Valhalla (`VALHALLA_URL`),
  costing `pedestrian` or `bicycle`, decodes the polyline6 shape to GeoJSON.
- `straightline`: haversine distance between consecutive waypoints, walking
  at 1.3 m/s and cycling at 4.5 m/s, straight segments. Default for dev and
  tests.

Compose includes a `valhalla` service under the `routing` profile, built from
the Illinois extract from Geofabrik, with a documented one-time tile build.

**Narration** (`internal/narration`): `Synthesize(ctx, text) (Audio, error)`.

- `openai`: OpenAI-compatible `POST {base}/v1/audio/speech`
  (`NARRATION_BASE_URL`, `NARRATION_API_KEY`, `NARRATION_MODEL`,
  `NARRATION_VOICE`). Returns MP3.
- `none`: default. Narration jobs succeed without output.

**Blob store** (`internal/blob`): `Put(ctx, key, contentType, reader)`,
`Delete`, `PublicURL(key)`.

- `s3`: MinIO/R2/S3 through the MinIO Go client (`S3_ENDPOINT`, `S3_BUCKET`,
  `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_SSL`, `MEDIA_PUBLIC_BASE_URL`).
- `fs`: local directory (`MEDIA_FS_DIR`), served by the API at
  `/media/files/{key}`. Default for dev and tests.

**LLM drafting** (`internal/llm`, cycle 2): `Draft(ctx, request) (Draft, error)`
with `anthropic` and `openai` implementations. The configuration keys are
reserved now (`LLM_PROVIDER`, `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`) but
nothing uses them in this cycle.

**Identity** (`internal/auth`): JWKS fetch with caching, RS256 verification,
issuer and audience checks, per provider.

## 7. Configuration

Environment variables, all read in `internal/config`:

```
API_ADDR=:8080
DATABASE_URL=postgres://...
PUBLIC_BASE_URL=https://museum-api.subcult.tv

DEV_AUTH_ENABLED=false
APPLE_CLIENT_IDS=com.rork.InvisibleMuseum,tv.subcult.museum.web
GOOGLE_CLIENT_IDS=...apps.googleusercontent.com
ADMIN_BOOTSTRAP_EMAILS=you@example.com
SESSION_TTL_HOURS=2160

ROUTING_PROVIDER=straightline|valhalla
VALHALLA_URL=http://valhalla:8002

NARRATION_PROVIDER=none|openai
NARRATION_BASE_URL=https://api.openai.com
NARRATION_API_KEY=
NARRATION_MODEL=gpt-4o-mini-tts
NARRATION_VOICE=alloy

MEDIA_STORE=fs|s3
MEDIA_FS_DIR=./data/media
MEDIA_PUBLIC_BASE_URL=
S3_ENDPOINT= S3_BUCKET= S3_ACCESS_KEY= S3_SECRET_KEY= S3_USE_SSL=

LLM_PROVIDER=anthropic|openai   (reserved for cycle 2)
LLM_BASE_URL= LLM_API_KEY= LLM_MODEL=

JOB_WORKERS=2
```

`ADMIN_BEARER_TOKEN` is removed.

## 8. Seed data

Migration `00002_seed_downers_grove.sql` creates the Downers Grove city, four
topics ("Rail and the Commuter Town", "Downtown After Dark", "Parks and
Public Life", "Houses and Builders"), the four existing places re-expressed as
places, one or two moments each with dates, sources, and topics, and one
public tour "Downtown Memory Loop" on the rail topic with four stops and a
featured moment per stop. Route rows are left for the job worker to compute
on first boot: the migration inserts `compute_route` jobs.

Seed moment bodies are rewritten as real short local-history text with
citations to Wikipedia and the Downers Grove Historical Society where a URL
is known, and flagged `in_review` where it is not.

## 9. Backend layout

```
backend/
  api/openapi.yaml
  cmd/api/main.go                 wires config, db, providers, worker, router
  internal/
    config/                       env parsing
    db/                           pool + tx helper
    models/                       API and storage structs, enums, validation
    repository/                   one file per aggregate: cities.go, places.go,
                                  moments.go, media.go, tours.go, accounts.go,
                                  candidates.go, jobs.go
    http/                         router.go, middleware.go, respond.go, and one
                                  handler file per area: public.go, auth.go,
                                  me.go, admin_*.go
    auth/                         jwks.go, apple.go, google.go, sessions.go
    jobs/                         worker.go, route_job.go, narration_job.go
    routing/                      routing.go, valhalla.go, straightline.go
    narration/                    narration.go, openai.go, none.go
    blob/                         blob.go, s3.go, fs.go
    geo/                          haversine, bbox, slugify
  migrations/
    00001_init.sql
    00002_seed_downers_grove.sql
```

## 10. Testing

- **Unit:** geo math, slug generation, config parsing, provider clients
  against `httptest` servers, JWT verification against a locally generated
  RSA key and fake JWKS.
- **Handler:** every endpoint through the chi router against the real repository
  on the integration database (the fake-repository pattern is retired because the
  surface is now too wide to fake honestly). Covers auth gating, role gating, validation
  errors, ownership checks on `/me/tours`.
- **Repository integration:** real Postgres when `TEST_DATABASE_URL` is set,
  skipped otherwise. Runs the migrations with goose (as a library) into a
  fresh schema, exercises every repository method, and verifies the seed
  produces a tour with four stops and queued route jobs.
- **Worker:** `compute_route` and `generate_narration` against the local
  providers and the integration database.
- **Contract:** router versus OpenAPI document.
- **CI:** the Gitea workflow adds a Postgres service and sets
  `TEST_DATABASE_URL`.

## 11. Operations

- Compose: add `minio` (optional, `media` profile) and `valhalla` (optional,
  `routing` profile). Defaults run only Postgres and the API with `fs` media
  and `straightline` routing.
- NUC compose: same, with `.env` holding the real values. Deployment doc
  updated with the Valhalla tile build and the MinIO bucket setup.
- Makefile: `api-tours`, `api-places`, `api-tour` replace `api-spaces` and
  `api-nearby`; `nuc-smoke` checks `/healthz` and `/api/v1/cities`.
- Dockerfile moves to Go 1.24.

## 12. Non-goals and risks

- No rate limiting or abuse protection on `/me/tours` yet beyond the stop cap.
- Apple and Google verification cannot be exercised end to end without real
  client IDs; the dev login exists so every other flow can be tested.
- Valhalla's Illinois tile build takes a while and several gigabytes; it is
  optional and documented, not required for development.
- The iOS app is knowingly broken against this API until cycle 3.
