# Byl

Local history, on foot or by bike. Byl turns a town's history into walking and cycling tours: each stop is a real place, with dated stories, photographs, sources and an audio guide you can listen to on the way. It has an iOS app, an Android app and a web app, all served by one Go API.

Byl is in development. It is being rebuilt from an earlier augmented-reality prototype into topic-based tours with user-built routes, and the seeded content covers Downers Grove, Illinois. The repository, Xcode project and Android identifiers still use the earlier name, Invisible Museum.

Public site: https://byl.subcult.tv

![Byl web app, profile screen](docs/design/byl/screenshots/web-profile-light-1440.png)

## Features

For visitors (web, iOS and Android):

- Browse a city's tours by topic, with walking and cycling estimates and a map.
- Walk or ride a tour with GPS arrival detection, an "I'm here" fallback, and hand-off to Apple Maps or Google Maps for directions.
- Hear each stop's story as recorded narration, or as on-device or browser speech when no recording exists.
- Read place and moment pages with sources, credits and media.
- Build your own route from existing places and share it as a `/t/{slug}` link (the iOS app also opens `invisiblemuseum://tour/{slug}`).
- Save tours and keep progress. Progress is stored locally and merged with your account when you sign in with Apple or Google.
- Keep an active tour going from the cached route if the network drops.
- Suggest a new tour, or report a route that has changed, for curator review.

For curators (web admin at `/admin`):

- One review queue for imports, drafts and visitor proposals.
- Import candidates from Wikipedia, Wikimedia Commons, Internet Archive, the Library of Congress, or a list of local archive pages.
- Edit places, moments, media and tours, with a map and a stop editor.
- Optional LLM drafting of a moment from a candidate's source text, and ranking of a stop's moments against the tour topic.
- Route-report investigations and corrections, and a job monitor.

Off by default, enabled per deployment: scheduled transit connections between stops, current-event notices at places, and voluntary route contributions from the mobile apps.

## How the data fits together

- A **city** holds **topics**, **places**, **moments** and **tours**.
- A **place** is a point on the map with an arrival radius.
- A **moment** is a dated story at a place, with media, sources and topics. A place has many moments and one primary moment.
- A **tour** is an ordered walk or ride through places. A background job computes a route per tour and mode (walk, bike) and serves it as GeoJSON.
- Every unreviewed item (imports, drafts, user proposals) is a **candidate** in one review queue.

The API contract is [`backend/api/openapi.yaml`](backend/api/openapi.yaml). A test fails if the router and the document drift apart. Errors use `{"error": {"code", "message", "fields"?}}`; lists use `{"items": [...], "next_cursor": ""}`.

## Quick start

Requirements:

- Docker with Compose
- Go, if you want to run backend tests outside Docker
- Node.js with npm 11 for the web app
- Xcode on macOS for the iOS app
- JDK 21 and Android SDK platform 35 for the Android app (the checked-in Gradle wrapper supplies Gradle)

Start PostgreSQL, run migrations and start the API on port 8080:

```bash
make backend-up
```

In another shell, check it:

```bash
make api-health
make api-cities
make api-tour
```

Run the web app against that API:

```bash
make web-install
make web-dev        # http://localhost:5173
```

With `DEV_AUTH_ENABLED=true` on the API, you can sign in to the web app with any email address.

Run `make help` for every target. The main ones:

| Command | What it does |
| --- | --- |
| `make backend-up` / `backend-up-detached` / `backend-down` | Start or stop the API and PostgreSQL |
| `make migrate` / `migrate-status` / `migrate-down` | Run Goose migrations |
| `make db-shell` / `db-reset` | Open `psql`, or delete and recreate the database volume |
| `make backend-test` | Run the Go suite against a throwaway PostgreSQL container |
| `make web-check` | Type-check, lint and unit-test the web app |
| `make web-build` | Build the static web app |
| `make web-e2e` | Run Playwright against a temporary API and database |
| `make ios-test` | Run the `InvisibleMuseumKit` Swift package tests |
| `make ios-build` | Build the iOS app for a generic Simulator, unsigned |
| `make ios-run` | Build, install and launch on the simulator in `SIMULATOR_ID` |
| `make android-test` | Run the Android `:core` JUnit tests |
| `make android-build` | Test core, then build and lint the debug app (needs an Android SDK) |

Without `TEST_DATABASE_URL`, `go test ./...` in `backend/` runs only the unit tests. `go run ./cmd/migrate up` applies migrations without Docker.

## Configuration

The API reads environment variables. [`backend/.env.example`](backend/.env.example) lists all of them with development defaults.

| Area | Variables |
| --- | --- |
| Server and database | `API_ADDR`, `DATABASE_URL`, `PUBLIC_BASE_URL`, `JOB_WORKERS` |
| Sign-in | `DEV_AUTH_ENABLED`, `APPLE_CLIENT_IDS`, `GOOGLE_CLIENT_IDS`, `SESSION_TTL_HOURS`, `ADMIN_BOOTSTRAP_EMAILS` |
| Routing | `ROUTING_PROVIDER` (`straightline` or `valhalla`), `VALHALLA_URL` |
| Narration | `NARRATION_PROVIDER` (`none` or `openai` for any OpenAI-compatible speech endpoint), `NARRATION_BASE_URL`, `NARRATION_API_KEY`, `NARRATION_MODEL`, `NARRATION_VOICE` |
| Media | `MEDIA_STORE` (`fs` or `s3`), `MEDIA_FS_DIR`, `MEDIA_PUBLIC_BASE_URL`, `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_SSL` |
| LLM drafting | `LLM_PROVIDER` (`anthropic` or `openai`), `LLM_API_KEY`, `LLM_BASE_URL`, `LLM_MODEL` |
| Sourcing | `LOCAL_ARCHIVE_URLS`, `SOURCES_USER_AGENT`, `SOURCES_ALLOW_PRIVATE_HOSTS`, `BRIEF_DISCOVERY_PROVIDERS`, `BRIEF_DISCOVERY_SEED_URLS`, `PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED` |
| Optional features | `SCHEDULED_TRANSIT_ENABLED`, `TRANSIT_PROVIDER`, `HAPPENINGS_IMPORT_ENABLED`, `ROUTE_CONTRIBUTIONS_ENABLED` |

Accounts whose email is in `ADMIN_BOOTSTRAP_EMAILS` become admins on first sign-in. The media fetch job refuses private, loopback and link-local addresses unless `SOURCES_ALLOW_PRIVATE_HOSTS=true`.

Client settings:

- Web: set `VITE_GOOGLE_CLIENT_ID` at build time to show Google sign-in. In development, Vite proxies `/api` and `/media` to the API.
- iOS: the API base URL defaults to `https://byl.subcult.tv`. Override it from the Profile tab, or with `InvisibleMuseum.apiBaseURL` in `UserDefaults`, the process environment or `Info.plist`.
- Android: set `museum.apiBaseUrl` and `museum.googleServerClientId` in `android/local.properties`. Debug builds allow HTTP to `localhost` and the emulator host; release builds do not.

## Deploy

The backend builds into one container image (`backend/Dockerfile`) that holds the API, the migration tool and the contribution-retention job. The web app builds into an nginx image (`web/Dockerfile`) that serves the static app and proxies `/api` and `/media` to the API.

[`backend/docker-compose.yml`](backend/docker-compose.yml) runs PostgreSQL, migrations, the API and the web app. It also has optional profiles for MinIO media storage (`--profile media`) and a self-hosted Valhalla routing server with an Illinois extract (`--profile routing`). For a production deployment:

1. Build both images, or run `scripts/release.sh` to build and smoke-test them.
2. Provide PostgreSQL and set `DATABASE_URL`, `PUBLIC_BASE_URL` and your sign-in client IDs.
   Set `DEV_AUTH_ENABLED=false` and your own `ADMIN_BOOTSTRAP_EMAILS`. The development Compose file turns developer sign-in on and uses an example admin address by default, and developer sign-in accepts any email, so leaving it on lets anyone sign in as an admin.
3. Run migrations with the `migrate` service before starting a new API version.
4. Serve the web image over HTTPS. Use HTTPS for any non-local admin access.

## Project layout

```text
backend/               Go API, jobs, migrations, OpenAPI contract, Compose files
web/                   SvelteKit visitor site and curator admin (Vitest, Playwright)
ios-invisible-museum/  SwiftUI app (iOS 17+) and the InvisibleMuseumKit Swift package
android/               Kotlin/Compose app; :core is plain Kotlin/JVM with JUnit tests
scripts/               Release, end-to-end and content build scripts
docs/                  Product notes, content intake guide, design system
```

Content contributors can start with the [content intake guide](docs/content/INTAKE.md). The design tokens and app screenshots are in [`docs/design/byl/`](docs/design/byl/README.md).

## Contributing

Report bugs and request features in [GitHub issues](https://github.com/subculture-collective/byl/issues).

Send code as a pull request on GitHub against `main`. CI runs the backend tests against PostgreSQL, Go formatting and vet, web type checks, lint, unit tests and Playwright flows, Android core tests, Swift package tests, an unsigned iOS build, and a release image smoke test. All of them must pass before a pull request merges.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).
