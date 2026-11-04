# Tours Content Model and API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the backend with the tours content model, API, accounts, user routes, review queue, and background jobs described in the spec.

**Architecture:** One Go binary serves `/api/v1` with chi and runs an in-process job worker on a Postgres `jobs` table. A concrete `repository.Store` owns all SQL (pgx). External dependencies (routing, narration, blob storage, identity) are interfaces with a real and a local implementation chosen by config. Tests run against a real Postgres schema created per test package through `internal/testdb`.

**Tech Stack:** Go 1.24, chi v5, pgx v5, goose v3 (library, for test migrations), golang-jwt v5, minio-go v7, Postgres 16.

**Spec:** `docs/superpowers/specs/2026-11-04-tours-content-model-and-api-design.md`

## Global Constraints

- Module path stays `invisible-museum/backend`; `go.mod` moves to `go 1.24`.
- API base path stays `/api/v1`. No v2.
- Every error body is `{"error":{"code","message"}}`, validation errors add `fields` and use 422.
- UUID primary keys via `gen_random_uuid()`; slugs accepted anywhere an ID is for cities, places, topics, tours.
- `ADMIN_BEARER_TOKEN` is removed everywhere (code, compose, Makefile, docs).
- Integration tests skip, not fail, when `TEST_DATABASE_URL` is unset.
- No PostGIS.
- Commit after every task with a conventional-commit message.

## Review Focus

1. A user route whose stops span two cities, or reference a draft place: must be 422, never a tour with a broken route. Pinned in Task 10.
2. Publishing a tour while one route is still `pending`: must be 409 naming the mode, never a public tour with no geometry. Pinned in Task 11.
3. A moment published with a media asset that has an empty license: must be 409, never published. Pinned in Task 11.
4. An expired session token: must be 401 and must not update `last_used_at`. Pinned in Task 6.
5. A route job whose provider fails every attempt: the `tour_routes` row ends `failed` with the error text, the job ends `failed`, the tour stays unpublished. Pinned in Task 8.

---

### Task 1: Schema, seed, and test database harness

**Files:**
- Delete: `backend/migrations/00001_init.sql`, `00002_seed_downers_grove.sql`, `00003_tours.sql`
- Create: `backend/migrations/00001_init.sql`, `backend/migrations/00002_seed_downers_grove.sql`
- Create: `backend/internal/testdb/testdb.go`, `backend/internal/testdb/testdb_test.go`

**Interfaces:**
- Produces: `testdb.Open(t *testing.T) *pgxpool.Pool` — skips when `TEST_DATABASE_URL` is empty, creates schema `test_<random>`, sets `search_path`, runs all migrations from `backend/migrations` with goose, registers cleanup that drops the schema.
- Produces: the schema in spec §3 exactly, with enums as Postgres `CREATE TYPE ... AS ENUM`.

- [ ] Write `testdb_test.go`: `TestOpenRunsMigrations` opens, queries `SELECT count(*) FROM tours WHERE slug='downtown-memory-loop'` expecting 1, `SELECT count(*) FROM tour_stops` expecting 4, `SELECT count(*) FROM jobs WHERE kind='compute_route'` expecting 2.
- [ ] Run `go test ./internal/testdb/` → fails (package missing).
- [ ] Write `00001_init.sql` from spec §3: enums, tables, indexes (`places(latitude,longitude)`, `moments(place_id)`, `tours(city_id,status,visibility)`, `candidates(status)`, `jobs(status,run_after)`, partial unique `candidates(source_name,external_id)`), deferred FK for `places.primary_moment_id`, trigger-free `updated_at` (set by repository).
- [ ] Write `00002_seed_downers_grove.sql`: city `downers-grove`, four topics, four places (`main-street-station`, `tivoli-theatre`, `fishel-park`, `downers-grove-museum`) with `kind` station/building/park/site, six moments with sources and topics, `primary_moment_id` set, tour `downtown-memory-loop` public published on `rail-and-the-commuter-town` with four stops and featured moments, two `compute_route` jobs. Use fixed UUIDs so later seeds can reference them.
- [ ] Write `testdb.go` with `Open` as above; add `github.com/pressly/goose/v3` and `github.com/jackc/pgx/v5/stdlib` to go.mod.
- [ ] Start local Postgres, export `TEST_DATABASE_URL`, run `go test ./internal/testdb/` → PASS.
- [ ] Commit: `feat(backend): new schema, seed, and test database harness`.

### Task 2: Models, geo helpers, config

**Files:**
- Replace: `backend/internal/models/models.go` (split into `enums.go`, `content.go`, `tours.go`, `accounts.go`, `review.go`, `requests.go`)
- Create: `backend/internal/geo/geo.go`, `geo_test.go`
- Replace: `backend/internal/config/config.go`, create `config_test.go`

**Interfaces:**
- Produces: enum string types with `Valid() bool` for every enumeration in spec §3.
- Produces: `geo.DistanceMeters(lat1,lng1,lat2,lng2 float64) float64`, `geo.BoundingBox(lat,lng,radiusM float64) (minLat,minLng,maxLat,maxLng float64)`, `geo.Slugify(s string) string`, `geo.RandomSlug(n int) string`.
- Produces: `config.Config` with every key in spec §7 and `config.Load() (Config, error)`; invalid provider names error.
- Produces: request structs with `Validate() map[string]string` (empty map means valid): `CreatePlaceRequest`, `UpdatePlaceRequest`, `CreateMomentRequest`, `UpdateMomentRequest`, `CreateTourRequest`, `UpdateTourRequest`, `StopsRequest` (`[]StopInput{PlaceID, FeaturedMomentID *string, Note}`), `CreateUserTourRequest`, `UpdateUserTourRequest`, `ProgressRequest`, `CreateCandidateRequest`, `MediaMetadataRequest`, `SourcesRequest`, `TopicsRequest`, `RoleRequest`.

- [ ] Write `geo_test.go`: Main Street Station to Tivoli is between 90 and 110 m; `Slugify("Fishel Park!")=="fishel-park"`; `RandomSlug(10)` is 10 chars, two calls differ.
- [ ] Write `config_test.go`: defaults (`:8080`, `straightline`, `none`, `fs`, 2 workers, 2160 h), `DATABASE_URL` required, `ROUTING_PROVIDER=bogus` errors, comma lists parse.
- [ ] Run both → fail.
- [ ] Implement models, geo, config.
- [ ] Write `requests_test.go`: `CreateUserTourRequest` with 1 stop fails on `stops`, with 31 fails, `UpdateProgress` with unknown mode fails on `mode`.
- [ ] Run `go test ./internal/geo ./internal/config ./internal/models` → PASS.
- [ ] Commit: `feat(backend): models, geo helpers, and configuration`.

### Task 3: HTTP skeleton, error envelope, OpenAPI contract test

**Files:**
- Replace: `backend/internal/http/respond.go`, `router.go`; delete `auth.go`, `handlers.go`, `handlers_test.go`
- Create: `backend/internal/http/middleware.go`, `respond_test.go`, `openapi_test.go`
- Create: `backend/api/openapi.yaml` (paths only for now, filled in by later tasks)

**Interfaces:**
- Produces: `writeJSON(w, status, v)`, `writeError(w, status, code, msg)`, `writeValidation(w, fields map[string]string)`, `decodeJSON(r, dst) error` (1 MiB cap, disallow unknown fields → 400).
- Produces: `NewRouter(deps Deps) http.Handler` where `Deps{Store *repository.Store; Sessions *auth.Sessions; Apple, Google auth.Verifier; DevAuth bool; Blob blob.Store; Enqueue func(ctx, kind, payload) error; MediaFSDir string}`. Handlers added by later tasks register inside `NewRouter`.
- Produces: `requireAuth`, `requireRole(models.AccountRole...)` middleware and `accountFrom(ctx) *models.Account`.

- [ ] Write `respond_test.go`: error body shape; `decodeJSON` rejects unknown field with 400 code `bad_request`.
- [ ] Write `openapi_test.go`: parse `../../api/openapi.yaml` (gopkg.in/yaml.v3), walk the router with `chi.Walk`, normalise `{param}` names, assert set equality of `METHOD path` both ways, excluding `/media/files/*`.
- [ ] Run → fail.
- [ ] Implement respond, middleware (auth stub that reads bearer → session lookup, wired fully in Task 6), router with `/healthz` only, and `openapi.yaml` with `/healthz`.
- [ ] Run `go test ./internal/http/` → PASS.
- [ ] Commit: `feat(backend): http skeleton with error envelope and OpenAPI contract test`.

### Task 4: Repository — content (cities, topics, places, moments, media)

**Files:**
- Replace: `backend/internal/repository/repository.go` with `store.go` (`type Store struct{pool *pgxpool.Pool}`, `New(pool)`, `tx` helper, `ErrNotFound`, `ErrConflict`), `cities.go`, `topics.go`, `places.go`, `moments.go`, `media.go`
- Create: `content_test.go`

**Interfaces (all take `ctx` first, `Store` receiver):**
- `ListCities() ([]models.City, error)`, `GetCity(idOrSlug) (*models.CityDetail, error)` (with topics), `CreateCity(CreateCityRequest)`, `UpdateCity(id, UpdateCityRequest)`.
- `ListTopics(cityID)`, `GetTopic(idOrSlug, cityID string)`, `CreateTopic`, `UpdateTopic`.
- `ListPlaces(PlaceFilter{CityID, Status *models.EntityStatus, Near *geo.Point, RadiusM float64, BBox *geo.BBox, TopicID, Limit, Cursor})` → `[]models.PlaceSummary, nextCursor`; `GetPlace(idOrSlug, includeUnpublished bool) (*models.PlaceDetail, error)`; `CreatePlace`, `UpdatePlace`, `SetPlaceStatus(id, status)`, `SetPrimaryMoment(placeID, momentID)`.
- `ListMoments(placeID, includeUnpublished)`, `GetMoment(id, includeUnpublished) (*models.MomentDetail)` (media, sources, topics, narration URL), `CreateMoment`, `UpdateMoment`, `SetMomentStatus`, `ReplaceMomentMedia(id, []string)`, `ReplaceMomentSources(id, []models.Source)`, `ReplaceMomentTopics(id, []string)`, `MomentPublishBlockers(id) ([]string, error)` (media with empty license).
- `CreateMedia(models.MediaAsset) (*models.MediaAsset)`, `GetMedia(id)`, `UpdateMediaMetadata(id, MediaMetadataRequest)`.

- [ ] Write `content_test.go` against `testdb.Open`: seed city resolves by slug and id; `ListPlaces` near Main Street Station radius 300 returns 3 of 4 published places sorted by distance; `GetPlace` hides draft moments unless `includeUnpublished`; `MomentPublishBlockers` reports an unlicensed asset; not-found returns `ErrNotFound`.
- [ ] Run → fail.
- [ ] Implement.
- [ ] Run `go test ./internal/repository/` → PASS.
- [ ] Commit: `feat(backend): content repository`.

### Task 5: Repository — tours, accounts, progress, candidates, jobs

**Files:**
- Create: `backend/internal/repository/tours.go`, `accounts.go`, `candidates.go`, `jobs.go`, `tours_test.go`, `accounts_test.go`, `candidates_test.go`, `jobs_test.go`

**Interfaces:**
- Tours: `ListTours(TourFilter{CityID, TopicID, Mode, OwnerID, PublicOnly bool})`, `GetTour(idOrSlug) (*models.TourDetail)` (stops with place summary and resolved featured moment, route summaries per mode), `GetTourRoute(tourID, mode) (*models.TourRoute)`, `CreateTour(CreateTourInput{...Owner *string}) (*models.TourDetail)` (inserts stops and enqueues `compute_route` per mode in one tx), `UpdateTour`, `ReplaceStops(tourID, []StopInput)` (same tx rule), `DeleteTour`, `SetTourStatus`, `TourPublishBlockers(id) ([]string)`, `UpsertRoute(models.TourRoute)`, `MarkRoutePending(tourID, mode)`.
- Accounts: `UpsertAccount(provider, subject, email, name, bootstrapAdmin bool) (*models.Account)`, `GetAccount(id)`, `ListAccounts()`, `SetRole(id, role)`, `CreateSession(accountID, tokenHash, expires)`, `AccountByToken(tokenHash, now) (*models.Account, error)` (expired → `ErrNotFound`, touches `last_used_at` only on success), `RevokeSession(tokenHash)`.
- Saved/progress: `ListSaved(accountID)`, `Save(accountID, tourID)`, `Unsave`, `ListProgress(accountID)`, `GetProgress(accountID, tourID)`, `PutProgress(models.TourProgress)`.
- Candidates: `ListCandidates(status, kind, limit, cursor)`, `GetCandidate(id)`, `CreateCandidate(models.Candidate)`, `ReviewCandidate(id, reviewerID, status, note, approvedEntityID *string)`.
- Jobs: `Enqueue(ctx, tx pgx.Tx or nil, kind string, payload any) error`, `ClaimJob(ctx, now) (*models.Job, error)` (`FOR UPDATE SKIP LOCKED`, sets running), `CompleteJob(id)`, `FailJob(id, err string, retryAfter *time.Time)`, `ListJobs(status, limit, cursor)`, `RetryJob(id)`.

- [ ] Write tests: `CreateTour` with 3 stops queues 2 jobs; `GetTour` on seed resolves featured moments and falls back to primary moment when null; `ReplaceStops` marks routes pending and queues jobs; `AccountByToken` after expiry → `ErrNotFound` and `last_used_at` unchanged; `ClaimJob` twice concurrently yields one job; `FailJob` past max attempts leaves status `failed`.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): tours, accounts, review queue, and jobs repositories`.

### Task 6: Auth package

**Files:**
- Create: `backend/internal/auth/jwks.go`, `verifier.go`, `apple.go`, `google.go`, `sessions.go`, `jwks_test.go`, `verifier_test.go`, `sessions_test.go`
- Modify: `backend/internal/http/middleware.go` to use `Sessions.Resolve`.

**Interfaces:**
- `type Identity struct{Subject, Email, Name string}`; `type Verifier interface{ Verify(ctx, token string) (Identity, error) }`.
- `NewJWKSVerifier(jwksURL, issuer string, audiences []string, cacheTTL time.Duration) Verifier` with RS256 via golang-jwt v5; `NewApple(audiences)` = JWKS `https://appleid.apple.com/auth/keys`, issuer `https://appleid.apple.com`; `NewGoogle(audiences)` = `https://www.googleapis.com/oauth2/v3/certs`, issuer `https://accounts.google.com` or `accounts.google.com`.
- `type Sessions struct{store *repository.Store; ttl time.Duration}`; `Issue(ctx, accountID) (token string, expires time.Time, error)`; `Resolve(ctx, token) (*models.Account, error)`; `Revoke(ctx, token)`; `HashToken(token) string`.

- [ ] Write `verifier_test.go`: generate RSA key, serve JWKS from `httptest`, sign token with `kid`; valid → identity; wrong audience → error; expired → error; unknown kid → error after one refetch.
- [ ] Write `sessions_test.go` (testdb): issue then resolve; revoke then resolve → error.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): identity verification and sessions`.

### Task 7: Providers — routing, narration, blob

**Files:**
- Create: `backend/internal/routing/{routing.go,straightline.go,valhalla.go,polyline.go}` + tests
- Create: `backend/internal/narration/{narration.go,none.go,openai.go}` + tests
- Create: `backend/internal/blob/{blob.go,fs.go,s3.go}` + tests
- Create: `backend/internal/providers/providers.go` — `Build(cfg) (Providers, error)` selecting implementations.

**Interfaces:**
- `routing.Point{Lat, Lng float64}`; `routing.Leg{DistanceM float64; DurationS int; Geometry [][2]float64}`; `routing.Route{DistanceM, DurationS, Geometry [][2]float64 (lng,lat); Legs []Leg}`; `type Router interface{ Route(ctx, mode models.TravelMode, pts []Point) (Route, error) }`.
- `routing.NewStraightLine()`; `routing.NewValhalla(baseURL string, client *http.Client)`; `routing.DecodePolyline6(s string) [][2]float64`.
- `narration.Audio{Data []byte; MimeType string; DurationS *float64}`; `type Narrator interface{ Synthesize(ctx, text string) (*Audio, error) }` — `None` returns `(nil, nil)`.
- `blob.Store interface{ Put(ctx, key, contentType string, r io.Reader, size int64) error; Delete(ctx, key) error; PublicURL(key) string }`; `blob.NewFS(dir, publicBase)`, `blob.NewS3(cfg)`.

- [ ] Tests: straight line 4-point route has 3 legs and geometry of 4 points, walking duration = distance/1.3; polyline6 decodes Valhalla's documented sample; Valhalla client parses a canned `trip.legs[].shape` from `httptest` and sends `costing: bicycle` for bike; OpenAI narration posts to `/v1/audio/speech` with `model`, `voice`, `input` and returns bytes; FS store writes file and `PublicURL` joins base.
- [ ] Run → fail. Implement (add `github.com/minio/minio-go/v7`). Run → PASS.
- [ ] Commit: `feat(backend): routing, narration, and blob providers`.

### Task 8: Job worker

**Files:**
- Create: `backend/internal/jobs/{worker.go,route_job.go,narration_job.go,worker_test.go}`

**Interfaces:**
- `jobs.New(store, router routing.Router, narrator narration.Narrator, blob blob.Store, workers int, logger *slog.Logger) *Worker`; `Run(ctx)` blocks until ctx done; `RunOnce(ctx) (processed bool, err error)` for tests.
- Backoff: `30s * 2^(attempts-1)`.

- [ ] Tests (testdb + straightline + none + FS): after seed, `RunOnce` twice yields two `ready` routes for `downtown-memory-loop` with 3 legs each; with a router that always errors, after `max_attempts` claims the route row is `failed` with the error and job `failed`; narration job with `None` completes and leaves `narration_media_id` null; with a fake narrator returning bytes, creates an `audio` asset and links it.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): job worker with route and narration jobs`.

### Task 9: Public handlers

**Files:**
- Create: `backend/internal/http/public.go`, `public_test.go`, `testserver_test.go` (helper: builds full router on testdb with straightline/none/fs, returns `*httptest.Server` and a `signIn(email, role)` helper using dev auth + direct role set)
- Modify: `router.go`, `api/openapi.yaml`

- [ ] Tests: `GET /api/v1/cities/downers-grove` has 4 topics; `/cities/downers-grove/places?near=41.7959,-88.0092&radius_m=300` returns 3 with `distance_m` ascending; `/tours/downtown-memory-loop` has 4 stops each with `featured_moment`, and `routes.walk.status=="pending"` before the worker runs; `/tours/{slug}/route?mode=walk` after `RunOnce` returns a LineString; draft place → 404; private tour → 404 for anonymous.
- [ ] Run → fail. Implement. Run → PASS. OpenAPI contract test passes.
- [ ] Commit: `feat(backend): public content and tour endpoints`.

### Task 10: Auth and `/me` handlers

**Files:**
- Create: `backend/internal/http/auth.go`, `me.go`, `auth_test.go`, `me_test.go`
- Modify: `router.go`, `api/openapi.yaml`

- [ ] Tests: `/auth/dev` 404 when disabled, 200 with token when enabled; bootstrap email → `admin`; `/me` without token → 401; `POST /me/tours` with 4 seed places → 201 unlisted with random slug, 2 queued jobs, visible via `/tours/{slug}` anonymously; private → 404 anonymous, 200 owner; stops from a draft place → 422 `stops`; other user's `PATCH` → 404; `PUT /me/progress/{tour}` then `GET` round-trips; `DELETE /auth/session` then `/me` → 401.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): sign-in, sessions, user routes, saved tours, progress`.

### Task 11: Admin handlers

**Files:**
- Create: `backend/internal/http/admin_content.go`, `admin_tours.go`, `admin_media.go`, `admin_review.go`, `admin_system.go`, `admin_test.go`
- Modify: `router.go`, `api/openapi.yaml`

- [ ] Tests: visitor → 403 on `/admin/places`; curator creates place → draft; publish place → published; create moment, attach media with empty license, publish → 409 `conflict` mentioning license; set license, publish → 200 and one `generate_narration` job; create tour, `PUT stops`, publish before worker → 409 naming `walk`; after `RunOnce` x2 publish → 200; candidate approve creates draft moment and stores `approved_entity_id`; reject stores note; `PATCH /admin/accounts/{id}/role` curator → 403, admin → 200; multipart upload to `/admin/media` stores a file under `MEDIA_FS_DIR` and `GET /media/{id}` returns its URL; `/admin/jobs?status=failed` lists and `retry` requeues.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): curator and admin endpoints`.

### Task 12: Wiring, operations, docs

**Files:**
- Replace: `backend/cmd/api/main.go`; modify `backend/Dockerfile` (Go 1.24), `backend/docker-compose.yml`, `backend/docker-compose.nuc.yml`, `backend/.env.example`, `Makefile`, `.gitea/workflows/ci.yml`, `README.md`, `docs/deployment/nuc-almaz.md`, `.gitignore` (`backend/data/`)
- Complete: `backend/api/openapi.yaml` schemas for every response.

- [ ] `main.go`: load config, open pool, build providers, `jobs.New(...).Run` in a goroutine, router, graceful shutdown on SIGINT/SIGTERM with 10 s timeout.
- [ ] Compose: `minio` under profile `media`, `valhalla` under profile `routing` (image `ghcr.io/gis-ops/docker-valhalla/valhalla:latest`, volume `valhalla_tiles`, env `tile_urls=https://download.geofabrik.de/north-america/us/illinois-latest.osm.pbf`), api env for every spec §7 key, `DEV_AUTH_ENABLED=true` in dev compose only.
- [ ] CI: Postgres 16 service, `TEST_DATABASE_URL`, `go vet`, `go test -count=1 ./...`, `go build ./...`.
- [ ] Makefile: replace `api-spaces`/`api-nearby` with `api-cities`, `api-places`, `api-tour`; `nuc-smoke` → `/healthz` and `/api/v1/cities`; add `backend-test` that starts a throwaway Postgres container, runs tests, stops it.
- [ ] Docs: README backend section rewritten from the spec's endpoint tables; deployment doc covers `.env` keys, MinIO bucket creation, Valhalla tile build, and that `DEV_AUTH_ENABLED` must be false in production.
- [ ] Verify: `go vet ./... && go test -count=1 ./... && go build ./...` and `docker compose config` both pass.
- [ ] Commit: `feat(backend): wire providers and worker, update compose, CI, and docs`.
- [ ] Push, open draft PR.
