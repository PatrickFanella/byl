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
make api-nearby
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
make api-spaces
make api-nearby
make ios-build
make ios-run
make ios-simulators
```

## Backend

The backend exposes:

- `GET /healthz`
- `GET /api/v1/spaces`
- `GET /api/v1/spaces/nearby?lat=41.794&lng=-88.007&radius_m=5000`
- `GET /api/v1/spaces/{space_id}`
- `GET /api/v1/exhibits/{exhibit_id}`
- `POST /api/v1/contributions`
- `GET /api/v1/admin/contributions?status=pending|approved|rejected|all`
- `PATCH /api/v1/admin/contributions/{id}/status`

Admin moderation routes require `Authorization: Bearer <ADMIN_BEARER_TOKEN>`.
If `ADMIN_BEARER_TOKEN` is unset on the server, admin routes return 503 and stay closed.

Migrations seed Downers Grove test data.

## iOS app

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

The local iOS admin token is stored in `UserDefaults["InvisibleMuseum.adminBearerToken"]`.

For NUC/Almaz deployment, the intended shape is:

```txt
iOS app
  -> https://museum-api.subcult.tv
  -> Cloudflare / Caddy on Almaz
  -> Go API + PostgreSQL on NUC
```

## Notes

- Contribution moderation is available in the Profile screen; set an admin token first.
- Use HTTPS for any non-local admin access.
- Rotate the admin token by updating the env var and restarting the backend.
- App icon is currently a generated placeholder.

## License

Licensed under `GPL-3.0-or-later`. See [LICENSE](LICENSE).
