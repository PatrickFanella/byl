# Rebuild follow-up: fixes and validation

This pass follows the five-cycle rebuild and its [74 rulings and 54 deferred review entries](2026-11-05-rebuild-rulings.md). PR #3 was already merged before this pass. This follow-up implements server progress merging, validates native compilation, and gives every deferred entry a disposition. The original ledger remains a historical record.

## Progress and cache ownership

`PUT /me/progress/{tour}` atomically unions visited positions across devices and retains an existing completion timestamp. Explicit `reset: true`, with empty positions and `completed: false`, clears progress and starts a new walk. PostgreSQL concurrency tests cover disjoint updates; HTTP tests cover merging, completion, invalid reset requests, and reset.

All three clients persist a pending reset until the API accepts it, including when no visited stops remain. They retry reset before merging remote progress. An obsolete remote fetch cannot restore progress after a local reset. Native active tours bind requests to the account token and API host that created them.

Local progress uses versioned account-and-tour keys shared across travel modes, matching server ownership. Native resume records include the account ID. Old unscoped cache files remain on disk but are not assigned to whichever account signs in next; authenticated progress can be restored from the server. Anonymous progress stays separate. Reset and writes from other devices remain ordered by the server: a later write can add its stops again. This API does not introduce distributed reset generations.

## Disposition of all 54 deferred entries

Each numbered row corresponds to one original ledger bullet, including bullets containing several related observations. “Retained” records intentional behavior; other rows describe implemented fixes or a confirmed existing fix. Backend paths below are relative to `backend/`.

### Cycle 1: backend content model and API (12)

| # | Original finding | Disposition and evidence |
| --- | --- | --- |
| 1 | Upload trusts client extension | Fixed. Sniffed MIME determines the extension; active HTML/XML/SVG uploads are rejected and multipart temporary files are removed (`internal/http/admin_media.go`). |
| 2 | Authentication resolves the session twice | Fixed. Required authentication reuses the account placed in context by optional authentication. |
| 3 | Unknown JWKS key IDs cause unlimited refresh | Fixed. A 30-second cooldown permits the first rotation lookup and bounds repeated misses; verifier tests cover repeated misses. |
| 4 | Publish and narration enqueue are separate commits | Fixed. Both share a repository transaction. A failed enqueue test proves publication rolls back. |
| 5 | Concurrent candidate approval; ignored media error | Fixed. Review locks the pending candidate and commits content, media, review, and jobs together. Tests prove one result under concurrent approval and no orphan content after failure. |
| 6 | Malformed body UUIDs return 404 | Fixed. PostgreSQL `22P02` maps to an invalid reference and HTTP 422. Explicitly validated malformed paths retain 404. HTTP tests cover the body case. |
| 7 | Ambiguous place slug selects the oldest match | Fixed. Ambiguity returns conflict; city-qualified lookup resolves the intended place. Repository tests cover both. |
| 8 | Excerpt cap splits UTF-8 | Fixed. Excerpts truncate by rune; Unicode regression coverage checks the boundary. |
| 9 | Zero configured workers still starts one | Fixed. `JOB_WORKERS=0` starts no workers. |
| 10 | Public seed has pending routes; first Valhalla boot can fail | Retained sample/bootstrap behavior. Clients display pending state, keep ready cached geometry, and poll. Operators still need routing tiles and must retry failed jobs after routing becomes available; seed publication does not prove routing readiness. |
| 11 | Runtime image lacks migration executable | Fixed. The image includes `/migrate` and `/migrations`; the built image successfully ran migration status against the validation database. |
| 12 | Missing OpenAPI query and media metadata declarations | Fixed. Admin query parameters, saved-tour pagination, and read-only `storage_key` are declared. Router/OpenAPI conformance tests pass. |

### Cycle 2: sourcing and curator web (14)

| # | Original finding | Disposition and evidence |
| --- | --- | --- |
| 1 | Browser tests absent from CI | Fixed. `scripts/test-web-e2e.sh` creates an isolated PostgreSQL database and dev-auth API, waits for TCP readiness, runs Playwright, and removes its own resources. Web CI runs it. |
| 2 | Downloads over 64 MiB silently truncate | Fixed. Oversized responses fail permanently. A regression test covers the limit. |
| 3 | Schema retry also retries request failures | Fixed. The extra retry applies only to invalid generated output; provider/transport failures propagate. The provider SDK retains its own retry policy. |
| 4 | Disabled dev auth still shows a login form | Fixed. The page probes `GET /auth/dev` before showing the form; POST independently enforces the setting. |
| 5 | `VITE_*` differs from the spec's `PUBLIC_*` | Retained. Browser configuration and Docker build arguments consistently use Vite's supported prefix. A rename would add deployment churn without changing behavior. |
| 6 | URL skip reasons lost; shared last-run state | Fixed. Import jobs consume per-run diagnostics and write actual skip reasons. The legacy `LastSkips` accessor is not used for concurrent job notes. |
| 7 | NC/ND licenses admitted; GFDL version downgraded | Fixed. Automatic open-license mapping rejects NC/ND and preserves GFDL 1.3. Fixtures cover the mapping. |
| 8 | URL and draft caps split UTF-8 | Fixed. Both count runes. |
| 9 | OpenAI errors retain megabytes of response text | Fixed. Error text is capped at 1,024 runes. A 4xx test checks one request and bounded output. |
| 10 | Draft polling hides failed jobs | Fixed. A curator-authorized job endpoint exposes terminal state; the page stops polling and displays failure. |
| 11 | Sign-in redirect accepts `//host` | Fixed. Targets must begin with one slash and contain no backslash. |
| 12 | nginx lacks security and media forwarding headers | Fixed. Responses include nosniff, frame denial, and CSP. Media proxy requests include forwarded address/protocol and original Host. Security headers inherit on assets and the app shell. |
| 13 | Import limit is unenforced | Fixed. The handler validates the documented 0–100 range. |
| 14 | Anthropic schema correction is untested | Fixed. A fixture returns invalid content then valid content and exercises the correction request. |

### Cycle 3: iOS and related backend observations (11)

| # | Original finding | Disposition and evidence |
| --- | --- | --- |
| 1 | GPS remains active after the tour ends | Fixed. Ending clears the location callback and stops updates. |
| 2 | Stale initial location can trigger arrival | Fixed. Monitoring rejects fixes older than 30 seconds and future-dated fixes; package tests cover stale input. |
| 3 | Invisible sync errors and pending routing | Fixed. The walk displays sync errors and pending status. Bounded polling refreshes routes; pending responses keep ready cached geometry. |
| 4 | Narration strips underscores within words | Fixed on all clients; narration tests distinguish word-internal underscores from emphasis. |
| 5 | Headings show raw Markdown | Fixed. SwiftUI uses full Markdown parsing. |
| 6 | Recreated AVPlayer; missing scroll target layout | Fixed. Tiles retain the player in view state, pause on disappearance, and declare the scroll target layout. |
| 7 | Edit hides errors; delete failure replaces the route list | Fixed. Action errors appear alongside the existing list. |
| 8 | Local progress ownership differs by mode | Fixed. Account-and-tour keys share progress across modes; package tests cover sharing and account separation. |
| 9 | Session token reads race mutation | Fixed. A recursive lock protects reads, restoration, mutation, and conditional clearing. |
| 10 | SSRF filter accepts reserved/transition IP ranges | Fixed. Reported special-use IPv4, NAT64, 6to4, and reserved ranges are rejected. IPv6 is restricted to ordinary global-unicast space; address fixtures cover the reported ranges. |
| 11 | One Archive metadata error aborts the import | Fixed. Item errors are logged and skipped; cancellation still propagates. |

### Cycle 4: visitor web (9)

| # | Original finding | Disposition and evidence |
| --- | --- | --- |
| 1 | Dialogs lack modal keyboard behavior | Fixed. Shared behavior supplies `aria-modal`, initial focus, Tab containment, Escape, and focus restoration. Browser coverage exercises End dialog keyboard behavior. |
| 2 | Walk session subscription should be reactive | Confirmed the existing `$current` subscription handles authentication and account ownership; no snapshot read was introduced. |
| 3 | Plain paragraphs retain raw link syntax | Fixed. Rendering reuses narration's plain-text conversion per paragraph. |
| 4 | Missing permission state and topic grouping | Fixed. Profile tracks supported browser permission changes; Home groups by topic. City selection fetches full details so topic filters are populated. |
| 5 | Browser progress crosses accounts and separates modes | Fixed. Account-and-tour keys match server ownership. Tests cover separation, mode sharing, reset retries, stale reads, and storage failure. |
| 6 | Place picker stops at 200 | Fixed. Web and both native pickers follow pagination. |
| 7 | Mode-filter test is vacuous | Fixed. A controlled walk-only published tour is excluded for biking, included for Any, and excluded by a different topic; the test removes its fixture. |
| 8 | Missing error page, dark basemap, encoded paths | Fixed. Visitor error handling is present, dark mode selects the dark OpenFreeMap style, and dynamic API path segments are encoded. The public dark style returned valid style JSON during validation. |
| 9 | Redundant admin layout redirect | Fixed. The duplicate effect was removed; route loading still enforces authentication. |

### Cycle 5: Android (8)

| # | Original finding | Disposition and evidence |
| --- | --- | --- |
| 1 | API switch can send sign-out to the new host | Fixed. Override is debug-only; release ignores it. Sign-out captures the original host/token before clearing local state. |
| 2 | Attach sends duplicate progress writes | Fixed. Serialized synchronization coalesces changes and skips accepted state; core tests check ordered, monotonic payloads. |
| 3 | Failed empty reset is never retried | Fixed on all clients. Persisted reset markers retry before ordinary merges; native and browser tests cover failure then retry. |
| 4 | Separate active instances overwrite stored stops | Fixed. Android serializes the store read/union/write and includes stored positions. |
| 5 | Stale/replayed fixes trigger arrival | Fixed. Fix age uses elapsed realtime; stale/future input is rejected and retained location is dropped on stop. Core tests cover stale fixes. |
| 6 | Coarse-only permission selects GPS | Fixed. Coarse permission selects the network provider. |
| 7 | Duplicate route error; pending route never refreshes and removes cache | Fixed. One status message is shown, bounded polling refreshes pending routes, and ready cached geometry survives pending responses. Core tests cover the cache case. |
| 8 | Directions fallback, sign-in cancel, locale, icon, delete error | Fixed. Missing fallback handlers are caught, credential cancellation closes quietly, TTS uses the device locale, an adaptive icon is supplied, and delete errors clear for each dialog. |

## Build and lifecycle corrections

Swift compilation exposed actor-isolation errors in narration planning and speech delegation. Pure planning/text functions are nonisolated, speech operations use the main actor, and delegate callbacks return explicitly to it. `make ios-build` uses a generic Simulator destination and an absolute product directory so the local package and app find the same outputs.

The Android Gradle 8.14.3 wrapper is checked in with the official distribution SHA-256. `make android-build` runs core tests, debug assembly, and lint. Native clients reject narration arriving after a walk ends or its arrival changes. Android cancels pending narration on leaving the foreground; browser narration uses an invalidation counter to prevent late responses from restarting stopped audio.

## Validation and limits

Validation used isolated PostgreSQL/API resources on Kvant and an isolated source copy on Soyuz. No production deployment, production migration, signing, or store release was performed.

| Area | Result |
| --- | --- |
| Backend | Full Go suite against PostgreSQL, vet, and build passed, including OpenAPI conformance, progress concurrency/reset, transaction rollback, and source/provider regressions. |
| Web | Type checks, lint, 64 unit tests, and all 14 browser integration tests passed. Browser tests used a real isolated API/database. Production Docker image built successfully. |
| Android | 68 core tests, debug assembly, and lint passed with Java 21, Gradle 8.14.3, and SDK 35. Core bytecode targets Java 17. An API 35 emulator loaded the seed tour, showed its first moment after manual arrival, and advanced to stop 2. |
| iOS | All 79 Swift package tests passed on Soyuz with Xcode 27.2. Unsigned iPhone and generic Simulator builds passed. Supported device tooling exposed no iOS host; iOS UI, physical GPS, and background audio remain untested. |
| CI configuration | Backend quality; web quality including browser integration; SDK-independent Android core tests; iOS build/package tests. Android app assembly/lint were validated locally. Hosted results belong to the follow-up PR's checks. |

These builds replace the earlier “written blind” status with compilation evidence. Physical-device and production runtime qualification remain separate. Routing still requires a ready service and tiles, and external media/model-provider availability remains an operational dependency.
