# Invisible Museum

Walking and biking tours of your city as if it were one giant museum. SwiftUI iOS prototype plus a Dockerized Go/PostgreSQL backend.

> **Rebuild in progress.** The app is being refocused from AR exhibits to topic-based walking/biking tours with sourced history and media, plus user-created routes, across native iOS, Android, and web clients. See `docs/superpowers/specs/` for the current design.

## What is here

- `ios-invisible-museum/` — SwiftUI app.
- `backend/` — Go REST API, PostgreSQL, and Goose migrations.
- `docs/superpowers/plans/` — implementation planning notes.
- `docs/deployment/nuc-almaz.md` — NUC/Almaz deployment notes.
- `Makefile` — root commands for backend, migrations, API smoke tests, and Simulator runs.

## Requirements

- Docker Desktop or Docker Engine
- Xcode
- iOS Simulator
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

## iOS app

> The iOS app still targets the previous API and does not run against this backend yet. It is rebuilt in the next cycle.

Current tabs:

- Explore
- Contribute
- Collection
- Profile

The app uses a SwiftUI design system built around semantic tokens and reusable primitives:

- `Theme.swift`
- `MuseumScreen.swift`
- `MuseumCard.swift`
- `MuseumButton.swift`
- `MuseumChip.swift`
- `MuseumField.swift`
- `MuseumSection.swift`
- `MuseumBottomBar.swift`

If the backend is unavailable, the app falls back to bundled sample data and shows the fallback state in Explore/Profile.

### Directions behavior

- A place detail screen offers in-app walking directions (Map tab) and an Apple Maps handoff.
- Arrival at a place is detected within roughly 50m.

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

## Notes

- Use HTTPS for any non-local admin access.
- App icon is currently a generated placeholder.

## License

Licensed under `GPL-3.0-or-later`. See [LICENSE](LICENSE).
