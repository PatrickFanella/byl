# Byl

Local history, on foot or by bike. Byl has SwiftUI, Compose and SvelteKit clients backed by Go and PostgreSQL. The repository and platform identifiers retain the Invisible Museum name.

The apps use Studio's Classic solid / P3 Oxblood design system. See the [BYL integration record](docs/design/byl/README.md) for source ownership, asset hashes, regeneration and verification limits.

> **Rebuild in progress.** The app is being refocused from AR exhibits to topic-based walking/biking tours with sourced history and media, plus user-created routes, across native iOS, Android, and web clients. See `docs/superpowers/specs/` for the current design.

## What is here

- `ios-invisible-museum/` — SwiftUI app.
- `backend/` — Go REST API, PostgreSQL, and Goose migrations.
- `web/` — SvelteKit app: the visitor site at `/` and the curator admin at `/admin`.
- `android/` — Kotlin/Compose visitor app with a JVM-tested `:core` module.
- `docs/superpowers/plans/` — implementation planning notes.
- `docs/deployment/nuc-almaz.md` — NUC/Almaz deployment notes.
- `docs/deployment/byl.md` — public Byl hosting, automatic releases and recovery.
- `Makefile` — root commands for backend, migrations, API smoke tests, and Simulator runs.

## Requirements

- Docker Desktop or Docker Engine
- Xcode on macOS for iOS builds and package tests
- JDK 21 and Android SDK platform 35 for the Android app; the checked-in Gradle wrapper supplies Gradle
- Optional local Go toolchain if you want to run backend tests outside Docker

## Quick start

```bash
make backend-up
```

In another shell:

```bash
make api-health
make api-tour
make ios-run
```

For deployment-oriented checks:

```bash
make nuc-up
make nuc-smoke
```

The iOS app defaults to:

```txt
https://museum-api.subcult.tv
```

That works from the simulator or a physical device anywhere with internet access. Use the API URL override if you want to point at a local backend.

## Useful commands

```bash
make help
make backend-up
make backend-up-detached
make backend-down
make backend-logs
make migrate
make migrate-status
make migrate-down
make db-shell
make db-reset
make api-health
make api-cities
make api-places
make api-tour
make backend-test
make ios-build
make ios-run
make ios-simulators
```

## Backend

One Go API serves every client. The contract is `backend/api/openapi.yaml`; a
test fails if the router and the document drift.

Core ideas:

- A **city** is a museum. A **topic** is a wing.
- A **place** is a wall: a point on the map with an arrival radius.
- A **moment** is an exhibit: a dated story at a place, with media, sources, and topics. A place has many moments and one primary moment.
- A **tour** is an ordered walk or ride through places. Curated tours feature one moment per stop chosen for the topic. User routes feature each place's primary moment.
- A **route** per tour and mode (walk, bike) is computed by a background job and served as GeoJSON.
- Every unreviewed thing (imports, drafts, user proposals) is a **candidate** in one review queue.

Public endpoints:

- `GET /healthz`
- `GET /api/v1/cities`, `GET /api/v1/cities/{city}`
- `GET /api/v1/cities/{city}/places?near=lat,lng&radius_m=&bbox=&topic=`
- `GET /api/v1/cities/{city}/tours?topic=&mode=`
- `GET /api/v1/places/{place}`, `GET /api/v1/places/{place}/moments`
- `GET /api/v1/moments/{id}`, `GET /api/v1/topics/{id}`, `GET /api/v1/media/{id}`
- `GET /api/v1/tours/{tour}`, `GET /api/v1/tours/{tour}/route?mode=walk|bike`

Sign-in and personal data (`Authorization: Bearer <token>`):

- `POST /api/v1/auth/apple`, `POST /api/v1/auth/google`, `POST /api/v1/auth/dev` (only with `DEV_AUTH_ENABLED=true`)
- `GET /api/v1/me`, `DELETE /api/v1/auth/session`
- `GET|POST /api/v1/me/tours`, `GET|PATCH|DELETE /api/v1/me/tours/{id}`
- `GET /api/v1/me/saved`, `PUT|DELETE /api/v1/me/saved/{tour}`
- `GET /api/v1/me/progress`, `GET|PUT /api/v1/me/progress/{tour}`

Curator and admin endpoints live under `/api/v1/admin` and require the
`curator` or `admin` role. Accounts whose email is listed in
`ADMIN_BOOTSTRAP_EMAILS` become admins on first sign-in.

Every error is `{"error": {"code", "message", "fields"?}}`. Lists are
`{"items": [...], "next_cursor": ""}`.

### Providers

| Concern | Dev default | Production option |
| --- | --- | --- |
| Routing | `straightline` | `valhalla` (self-hosted, `--profile routing`) |
| Narration | `none` (on-device speech) | `openai` (any OpenAI-compatible speech endpoint) |
| Media | `fs` (served at `/media/files/`) | `s3` (MinIO, R2, S3) |
| LLM drafting | unset (draft and suggest return 503) | `anthropic` (Claude API) or `openai` (any OpenAI-compatible endpoint) |

### Sourcing

Curators fill the review queue from open archives through `POST /api/v1/admin/imports`
with a `source` from `GET /api/v1/admin/sources`:

| Source | Finds | Centre |
| --- | --- | --- |
| `wikipedia` | a place and a moment per article near a point | city, place, or `near` |
| `wikimedia_commons` | images near a point with licence metadata | city, place, or `near` |
| `internet_archive` | texts as moments, images and films as media | `query` |
| `loc` | Library of Congress items for a query | `query` |
| `url` | pages listed in `LOCAL_ARCHIVE_URLS` as moments | optional `query` filter |

Approved media candidates are downloaded by the `fetch_media` job, which only accepts
image, video or audio responses matching the asset kind and refuses private, loopback and
link-local addresses (`SOURCES_ALLOW_PRIVATE_HOSTS=true` lifts that for development).

`POST /api/v1/admin/candidates/{id}/draft` asks the configured LLM to write a moment
from a candidate's source text; `POST /api/v1/admin/tours/{id}/stops/{position}/suggest`
ranks a stop's moments against the tour topic. Both need `LLM_PROVIDER`.

See `backend/.env.example` for every variable. `go run ./cmd/migrate up` applies migrations without Docker.

### Tests

```bash
make backend-test
```

That starts a throwaway Postgres in Docker and runs the full suite against it.
Without `TEST_DATABASE_URL`, `go test ./...` runs only the pure unit tests.

## Web app

`web/` is one SvelteKit single-page app with two faces on the same origin:

- **Visitor site at `/`** (cycle 4): browse a city's tours by topic, open a tour with
  per-mode estimates and a map, walk or ride it in the browser (`/tours/{slug}/walk`)
  with GPS arrival detection (65 m accuracy floor, "I'm here" fallback), Directions
  handoff to Apple or Google Maps, an audio guide (recorded narration or browser
  speech), local progress merged with the account when signed in, place and moment
  pages with sources and credits, saved tours, and a route builder that shares
  `/t/{slug}` links (which also open in the iOS app). A walk keeps going when the
  API becomes unreachable, from the cached tour and route (the page itself still
  needs the network to load: there is no service worker yet).
- **Curator admin at `/admin`**: dashboard, review queue with AI drafting, imports,
  places, moments, media, tours with a map and stops editor, and jobs. Non-curators
  see a role notice; signed-out visitors are sent to `/signin`.

nginx proxies `/api` and `/media` in the compose stack (`web` service on port 3000),
and `vite dev` proxies them in development. Logic that matters (arrival, progress
merge, estimates, narration, route-builder validation) lives in `web/src/lib` with
Vitest; Playwright covers the visitor flows and the admin smoke against a live API.

```bash
make web-install
make web-dev        # http://localhost:5173, sign in with any email while DEV_AUTH_ENABLED=true
make web-check
make web-e2e        # starts its own temporary PostgreSQL and API; requires Docker, Go, Node, and Playwright Chromium
```

Set `VITE_GOOGLE_CLIENT_ID` at build time to show Google sign-in.

## iOS app

`ios-invisible-museum/` is the SwiftUI visitor app (iOS 17+), rebuilt in cycle 3 against this
backend's `/api/v1`. It is an Xcode project plus a local Swift package:

- `InvisibleMuseumKit/` — API client, DTOs matching `backend/api/openapi.yaml`, session
  controller, and the view models (`TourDetailModel`, `ActiveTourModel`, `RouteBuilderModel`,
  `ArrivalMonitor`, `NarrationPlayer`, `DiskCache`) with XCTest coverage against recorded
  fixtures and a `URLProtocol` stub. Runs on a Mac with `swift test`.
- `InvisibleMuseum/` — the app: `App/` (model, root tabs), `Features/` (Home, Tours incl. the
  active walking/riding mode with map, Places and Moments, Routes builder, Saved, Profile),
  `Location/`, `Audio/`, `Session/`, `Design/`.

What it does: browse a city's tours by topic, open a tour and its stops, walk or ride it with a
route line, arrival detection (65 m accuracy floor, "I'm here" fallback) and an Apple Maps
handoff, hear each stop's story (recorded narration or on-device speech), build and share your
own routes (`/t/{slug}` links and the `invisiblemuseum://tour/{slug}` scheme), save tours and
keep progress when signed in with Apple. An active tour keeps working offline from the cached
route and progress.

Validated on Soyuz with Xcode 27.2: the unsigned iOS app builds and all 79
`InvisibleMuseumKit` tests pass. `make ios-build` uses a generic Simulator destination,
so it does not depend on a machine-specific simulator UUID. For an unsigned iPhone build,
run `make ios-build IOS_DESTINATION='generic/platform=iOS'`. These are compile and package-test
checks; physical-device GPS, sign-in, background behavior, and audio still need device qualification.

Point the app at a local API from the Profile tab (API base URL), or launch with the
environment variable `InvisibleMuseum.apiBaseURL=http://localhost:8080`. Debug builds show the
developer email sign-in when the API runs with `DEV_AUTH_ENABLED=true`.

## Android app

`android/` is a two-module Gradle project (cycle 5):

- `:core` — plain Kotlin/JVM: DTOs matching `backend/api/openapi.yaml`, an OkHttp API
  client with the error envelope and 401 hook, session store, estimates, the arrival
  rule (65 m accuracy floor, radius plus accuracy, once per stop), `ActiveTour` (route
  with cached fallback, local progress merged with the server by union, serialised
  PUTs, resume after the process is killed), `RouteBuilder`, narration planning and
  navigation URIs. 68 JUnit 5 tests against the recorded fixtures and MockWebServer;
  they run anywhere with a JDK: `make android-test`.
- `:app` — Jetpack Compose (Material 3, Navigation, MapLibre with OpenFreeMap tiles,
  LocationManager, MediaPlayer + TextToSpeech, Credential Manager for Google sign-in,
  EncryptedSharedPreferences). Included only when an Android SDK is present
  (`ANDROID_HOME` or `local.properties`).

Validated on Kvant with JDK 21, Gradle 8.14.3, and Android SDK 35: all 68 core
tests, `:app:assembleDebug`, and `:app:lintDebug` pass. The Gradle wrapper is checked in
with the distribution checksum. Run `make android-test` for the core, or
`ANDROID_HOME="$HOME/Android/Sdk" make android-build` for the app. A local Android 15 emulator
also loaded the seeded tour and advanced through manual arrival.

Set `museum.googleServerClientId` in `android/local.properties` to the web OAuth client ID
when configuring Google sign-in. For local development, set `museum.apiBaseUrl` to your API;
debug builds allow HTTP to localhost and the emulator host at `10.0.2.2`. Release builds
forbid cleartext and hide the server-override form.

## API URL configuration

The default API base URL is `https://museum-api.subcult.tv`.

Override order in the app:

1. `UserDefaults["InvisibleMuseum.apiBaseURL"]`
2. Process environment `InvisibleMuseum.apiBaseURL`
3. Info.plist key `InvisibleMuseum.apiBaseURL`
4. Default `https://museum-api.subcult.tv`

For NUC/Almaz deployment, the intended shape is:

```txt
iOS app
  -> https://museum-api.subcult.tv
  -> Cloudflare / Caddy on Almaz
  -> Go API + PostgreSQL on NUC
```

## Validation and follow-up

The follow-up to the five rebuild cycles is recorded in
[Native builds and review follow-up](docs/superpowers/2026-11-05-follow-up-validation.md),
including the disposition of all 54 deferred report entries. CI runs the backend suite,
web checks and browser flows, Kotlin core tests, Swift package tests, and an unsigned iOS build.

## Notes

- Use HTTPS for any non-local admin access.
- App icon is currently a generated placeholder.

## License

Licensed under `GPL-3.0-or-later`. See [LICENSE](LICENSE).
