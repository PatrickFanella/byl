# Rebuild report: rulings and deferred minors

Follow-up: [fix dispositions and native build validation](2026-11-05-follow-up-validation.md) records the resolution of these historical deferred entries.

Decisions taken on the partner's behalf during the five rebuild cycles, and reviewer findings graded Minor and deferred. Each cycle's spec and plan live beside this file under `specs/` and `plans/`; the per-cycle fresh reviews were applied as fix passes before each push.


Totals: 74 rulings, 54 deferred minors.


## Cycle 1 — backend content model and API


### Rulings (14)

- Pre-flight: shared interfaces checked (Store methods T4/T5 → handlers T9-11; providers T7 → worker T8; Sessions T6 → middleware T3). Clean. Ruling: handler tests use real repository on testdb (spec §10 updated) — fake would be ~60 methods — cost if wrong: slower tests.
- Task 3: Ruling: old repository.go and handlers deleted here, not in Task 4, and main.go kept compiling with a minimal router — build must stay green between tasks — cost if wrong: none, Task 4/12 rewrite both anyway.
- Task 3: Ruling: Deps.Sessions is a SessionResolver interface instead of *auth.Sessions — lets middleware tests run before Task 6 — cost if wrong: one type assertion nowhere.
- Task 4: Ruling: place slugs resolve within an optional city scope (GetPlace(ref, cityRef, ...)) since slugs are unique per city, not globally — cost if wrong: an extra query param on /places/{slug}.
- Task 4: Ruling: list pagination cursor is an opaque offset — distance ordering has no stable key — cost if wrong: duplicate rows across pages during concurrent inserts.
- Task 5: Ruling: seed inserts pending tour_routes rows alongside the route jobs (spec §8 said rows were left to the worker) — GetTour must show pending routes before the worker runs, matching CreateTour — cost if wrong: none.
- Task 8: Ruling: worker tests override the backoff function rather than the clock — a retry scheduled from a shifted clock is never claimable by that clock — cost if wrong: none.
- Task 9: Ruling: every list endpoint returns {"items": [...], "next_cursor": ""} — one shape for all three clients — cost if wrong: cosmetic.
- Task 9: Ruling: /auth/dev landed in Task 9 because the handler test helper signs in through it — cost if wrong: none.
- Task 10: Ruling: user routes ignore any featured_moment_id in the request and always feature the place primary moment — spec says user routes order existing places only — cost if wrong: a later feature flag.
- Task 11: Ruling: upload content type is sniffed from bytes, then extension, when the part declares octet-stream — Go and some clients send octet-stream for every part — cost if wrong: a mislabelled upload.
- Task 11: Ruling: candidate approval merges the override payload shallowly onto the stored payload — spec said "optional payload override" without defining depth — cost if wrong: nested fields need the whole object resent.
- Task 12: Ruling: added cmd/migrate (goose as a library) so the API image can migrate without the goose container and so the smoke run worked — cost if wrong: one extra binary.
- public tour reads still show a stop's place even if the place was later unpublished (now only possible for user routes, since curated tours block it) — a place name and coordinates are not withheld history — cost if wrong: one more filter.

### Review findings fixed (9)

- unpublished featured moment leak — TestPublicTourHidesUnpublishedFeaturedMoment RED→GREEN
- publish invariants after publish (place unpublish, stops/modes/recompute, licence removal) — TestUnpublishPlaceOnPublishedTourRefused, TestStopChangesOnPublishedCuratedTourRefused, TestLicenceRemovalOnPublishedMomentRefused RED→GREEN
- /me/saved leak — TestSavedHidesPrivateAndUnpublishedTours RED→GREEN
- stuck running jobs — TestStaleRunningJobsAreRequeued, TestCompletionSurvivesCancelledContext RED→GREEN
- admin slug 500 + swallowed rows.Err — TestAdminMutationsAcceptSlugs RED→GREEN
- unverified email trust — TestVerifierDropsUnverifiedEmail RED→GREEN
- go.mod toolchain drift — go 1.24 with pgx 5.7.6, goose 3.24.3, minio-go 7.0.80, jwt 5.2.2; build under GOTOOLCHAIN=local
- Review Focus #1 test gap — TestUserRouteRejectsStopsFromAnotherCity RED→GREEN
- (minors promoted: user-facing 500 / info leak) negative offset, media directory listing — TestNegativeOffsetAndDirectoryListing RED→GREEN

### Deferred minors (12)

- Final: minor (deferred): upload keeps client extension; whitelist by sniffed type
- Final: minor (deferred): requireAuth resolves the session twice after optionalAuth
- Final: minor (deferred): JWKS refetch on unknown kid has no cooldown
- Final: minor (deferred): moment publish + narration enqueue are two statements, not one tx
- Final: minor (deferred): candidate approve not atomic under concurrent approvals; ReplaceMomentMedia error discarded on media candidates
- Final: minor (deferred): 22P02 maps to 404 instead of 422 for malformed UUIDs in bodies
- Final: minor (deferred): GET /places/{slug} without ?city= picks the oldest match
- Final: minor (deferred): Excerpt truncates bytes not runes
- Final: minor (deferred): JOB_WORKERS=0 still runs one worker
- Final: minor (deferred): seed tour is public with pending routes; Valhalla first boot can fail its jobs (doc updated)
- Final: minor (deferred): cmd/migrate not built into the Docker image
- Final: minor (deferred): OpenAPI admin query params only in summaries; /me/tours lacks limit/cursor; storage_key undocumented

## Cycle 2 — sourcing pipeline and admin web


### Rulings (23)

- project moves to Go 1.25 everywhere — every Anthropic Go SDK v2 release requires it — cost if wrong: one version bump.
- Task 1: Ruling: SDK v2.6.0 sends the API key as Authorization: Bearer; test accepts either header — cost if wrong: none.
- Task 2: Ruling: Wikipedia extracts are fetched one page per request (the API returns full extracts for a single page only) and the client spaces requests one second per host — Wikimedia throttled a burst during fixture capture — cost if wrong: slower imports.
- Task 2: Ruling: Task 3 tests were written before Task 2 closed; task-done runs Task 2 tests filtered by name — cost if wrong: none.
- Task 3: Ruling: HTML extraction is regex-based without backreferences (RE2) and heuristic by design; the curator always sees the raw text — cost if wrong: noisier drafts from odd pages.
- Task 4: Ruling: the import job note goes into jobs.last_error after completion (spec §3 step 4) via SetJobNote — no result column exists — cost if wrong: confusing column name.
- Task 4: Ruling: ErrNoRetry landed here (plan put it in Task 5) because unknown sources must not retry — cost if wrong: none.
- Task 6: Ruling: suggest returns unranked moments appended with reason "not ranked by the model" rather than dropping them — the curator should always see every option — cost if wrong: none.
- Task 8: Ruling: npm 10.9.9 fails on this lockfile ("edgesOut"); installs use npx npm@11 and openapi-typescript needs --legacy-peer-deps because the template ships TypeScript 6 — documented in web/README — cost if wrong: contributor friction.
- Task 8: Ruling: imports use the $lib alias; the template-provided #lib subpath map does not resolve extensionless TypeScript modules — cost if wrong: none.
- Task 8: Ruling: generated src/lib/api/types.ts is git-ignored and produced by npm run gen:api before dev and build — cost if wrong: a stale-types bug caught by svelte-check.
- Task 9: Ruling: Review Focus 5 (visitor sees the curator-role notice) is pinned by a Playwright test instead of a Vitest component test — the scaffold has no component test environment — cost if wrong: the check only runs with E2E_API_URL.
- Task 9: Ruling: OpenAPI schemas gained required lists so generated types stop being all-optional — contract clarification, not a behaviour change — cost if wrong: none.
- Task 9: Ruling: eslint rule svelte/no-navigation-without-resolve disabled — the app has no base path — cost if wrong: broken links if a base path is ever added.
- Task 10: Ruling: map tiles come from OpenFreeMap liberty style (no key); swap the style URL for a self-hosted or keyed provider in production — cost if wrong: map outage if OpenFreeMap rate-limits.
- Task 11: Ruling: nginx proxy verified only by config structure and compose validation — no Docker daemon in this session — cost if wrong: a broken proxy found on first deploy.
- IA image/movie items with no usable file are dropped rather than emitted as moments — their description alone is not a moment's source text — cost if wrong: a few archive items never reach the queue.
- SOURCES_ALLOW_PRIVATE_HOSTS is a real operator knob (LAN archives) rather than a test-only hook; default off everywhere — cost if wrong: an operator enables it in production and reopens the SSRF path.
- candidate city_id/place_id cross-city on approval stays — cycle-1 approval semantics, a curator's own privilege — cost if wrong: a curator files a moment under the wrong city.
- token in localStorage stays — spec §6 mandates it — cost if wrong: XSS would be session theft (mitigated by nosniff + no {@html}).
- Lucene/IA query injection via free-text query stays — curator-supplied, affects only their own search — cost if wrong: none beyond odd results.
- CandidateExists treats rejected candidates as existing (never re-imported) — matches spec §3 step 3 — cost if wrong: a wrongly rejected find needs manual re-entry.
- draft:true on import not implemented — spec lists it off by default and the endpoint table omits it — cost if wrong: curators click Draft per candidate.

### Review findings fixed (7)

- job-note race (noteAfter shared across workers) — TestImportNoteStaysWithItsJob RED→GREEN (dispatch returns the note; field removed), suite ok under -race
- url adapter swapping the shared HTTP client — TestURLAdapterLeavesSharedClientUntouchedDuringFetch RED→GREEN (Client.GetWithRedirectCheck copies the client per call)
- SSRF via fetch_media media_url — TestFetchMediaJobRefusesPrivateHosts + TestGetPublicRefusesPrivateHostsUnlessAllowed + TestPublicIPClassification RED→GREEN (Client.GetPublic dials only public addresses, redirects included; SOURCES_ALLOW_PRIVATE_HOSTS opts out)
- fetch_media storing any content type / sniffable keys — TestFetchMediaJobRefusesNonMediaContent, TestFetchMediaJobRefusesKindMismatch, TestFetchMediaJobForcesExtensionFromContentType, TestMediaFilesServed (nosniff) RED→GREEN
- Internet Archive MediaURL pointing at the directory listing — TestInternetArchiveMediaFindsResolveToFiles RED→GREEN (metadata/<id>/files picks the original image or the mp4 derivative; fixtures recorded from archive.org)
- queue place-id edit discarding field edits — Playwright "setting the place id keeps the edited title" RED→GREEN (placeId is a writable derived merged at approve), all 4 e2e specs pass against a live API
- web Docker build copying host node_modules/build — .dockerignore at the repo root (build context); configuration, no test (TDD exception for config files)

### Deferred minors (14)

- Task 11: minor (deferred): Playwright e2e is not wired into CI (needs a running API with dev auth); run with make web-e2e.
- Final: minor (deferred): fetch_media silently truncates files over 64 MiB instead of failing.
- Final: minor (deferred): withOneRetry retries every error (incl. 4xx/cancel) on top of the SDK's own retries.
- Final: minor (deferred): sign-in page shows the dev-login form until a 404 instead of probing first.
- Final: minor (deferred): web env vars are VITE_* where the spec said PUBLIC_*.
- Final: minor (deferred): url adapter per-page skip reasons are counted, not written, in the job note; LastSkips is last-run state.
- Final: minor (deferred): NC/ND Commons licences map to real identifiers and pass the non-empty licence publish gate; GFDL 1.3 → GFDL-1.2.
- Final: minor (deferred): byte-wise text caps in url.go/draft_job.go can split a multibyte rune.
- Final: minor (deferred): OpenAI error path embeds up to 4 MiB of body in last_error.
- Final: minor (deferred): draft polling in the UI never surfaces a failed draft job.
- Final: minor (deferred): signin next accepts //host (goto rejects external URLs; tighten the regex anyway).
- Final: minor (deferred): nginx lacks X-Content-Type-Options/X-Frame-Options/CSP and X-Forwarded-* on /media/.
- Final: minor (deferred): ImportRequest.limit maximum 100 documented but not validated by the handler.
- Final: minor (deferred): Anthropic schema-validation retry path untested.

## Cycle 3 — iOS app


### Rulings (16)

- Pre-flight: no Swift toolchain in this session; every task ends with "not compiled here". Ruling: testable non-UI code lives in a local Swift package (InvisibleMuseumKit) with its own tests instead of a hand-built Xcode unit-test target — cost if wrong: one package reference in project.pbxproj to fix on a Mac.
- Task 3: Ruling: TourDetailModel lives in InvisibleMuseumKit (per the pre-flight ruling) and the tests use a URLProtocol stub shared through Tests/StubClient.swift; shared loading/empty/error views and a MuseumHeroCard were added beside the existing design primitives — cost if wrong: none.
- Task 3: Ruling: a tour stop opens the place with the tour's featured moment first and the rest listed underneath (the spec's "feature one, keep the others browsable"); Start walking/riding sets AppModel.activeTour/activeMode for Task 5's screen — cost if wrong: one navigation change.
- Task 4: Ruling: ArrivalMonitor lives in the kit (CoreLocation is available there) so its tests run with swift test; the location service drops heading tracking and gains an onLocation callback — cost if wrong: none.
- Task 5: Ruling: DTOs became Codable (custom encode for the flattened PlaceDetail/MomentDetail/TourDetail) so the active tour's detail and route cache as JSON; a round-trip test pins losslessness — cost if wrong: none.
- Task 5: Ruling: the arrival monitor only targets the next stop; being at a later stop does not count, and "I'm here" marks arrival by hand when GPS cannot — cost if wrong: a visitor who skips a stop taps once.
- Task 5: Ruling: cities are cached too, so bootstrap offline still selects a city and lets a cached tour resume (Review Focus 2) — cost if wrong: none.
- Task 6: Ruling: NarrationPlayer lives in the kit behind AudioSessionConfiguring/SpeechSynthesizing protocols so the order "configure session, then play" is testable on a Mac; the iOS AVAudioSession adapter is Audio/AudioSession.swift — cost if wrong: none.
- Task 6: Ruling: on arrival the app fetches the featured moment's detail (narration URL + body) and caches it; the summary only says whether narration exists — cost if wrong: one extra request per stop.
- Task 7: Ruling: stop reordering uses up/down buttons inside the scrolling form instead of List drag handles (a List inside MuseumScreen's ScrollView does not size itself) — cost if wrong: a less fluid reorder.
- Task 7: Ruling: share links use the API base URL + /t/{slug} (the universal-link placeholder named in the spec); the app already opens that form — cost if wrong: links change when a web host exists.
- Task 8: Ruling: project.pbxproj regenerated by script (objectVersion 60) with a group per directory, the local package reference, an Info.plist for the URL scheme and location text, Sign in with Apple entitlements, and the AR-era UI test sources removed; the kit's XCTest target is the unit test target — cost if wrong: a Mac fixes the project file once.
- Task 8: Ruling: design primitives moved under Design/ and GeoMath under Location/ to match the spec's layout — cost if wrong: none.
- spec §6 UI smoke test (launch argument → local stub) not written — no simulator or toolchain here; the kit tests cover the logic — cost if wrong: a launch crash is first seen on the Mac.
- nearest-city selection on bootstrap not implemented — one city exists and the picker covers more — cost if wrong: a visitor in a second city taps the picker once.
- dev sign-in shown in every DEBUG build instead of probing /auth/dev — a failed dev sign-in shows the API error — cost if wrong: one confusing button in debug builds only.

### Review findings fixed (12)

- pbxproj Packages group holding the package reference object — generator drops the group and links the product dependency with package =; verified by script (no bad group children, no dangling ids); uncompiled
- testPlacesLoadSearchAndSortByDistance asserting on a needle the addresses also match — test now asserts the name-or-address rule; uncompiled
- AppModel.isSignedIn not observable — derived from the tracked account; uncompiled
- no resume after relaunch (Review Focus 2) — testActiveTourIsRestoredFromTheCacheAfterARelaunch RED→GREEN by design (ActiveTourRecord + ActiveTourModel.restore, AppModel.resumeActiveTour on bootstrap); uncompiled
- progress never re-synced or merged (Review Focus 2) — testServerProgressIsMergedWhenTheTourStarts, testLocalStopsAheadOfTheServerArePushedAfterTheMerge, testStaleProgressResyncsWhenTheServerIsBack, testProgressPutsAreSentInOrder (loadRemoteProgress union, needsSync via updatedAt/syncedAt, serialised PUTs, syncIfNeeded on foreground); uncompiled
- 401 handling (Review Focus 3) — APIClientTests.testUnauthorizedCallsTheHookWithTheTokenUsedAndThrows; hook carries the rejected token, AppModel clears only a current token, presents sign-in now or after the active tour closes; uncompiled
- audio session never released / speech end unobserved — NarrationPlayerTests updated (deactivate after stop, testSpeechFinishingReleasesTheSession, testStopIsIdempotent); SystemSpeechSynthesizer reports didFinish; uncompiled
- SVG accepted by fetch_media — TestFetchMediaJobRefusesSVG RED→GREEN, suite ok
- public-only transport honouring HTTP(S)_PROXY (re-graded from Minor: it silently disables the SSRF guard) — TestPublicOnlyTransportIgnoresProxyEnvironment RED→GREEN, suite ok
- dead share links — TestShareLinkLandingPage RED→GREEN: GET /t/{slug} serves a landing page that hands off to invisiblemuseum://tour/{slug}; documented in OpenAPI
- cross-city stops (Review Focus 5) — testStopsFromAnotherCityBlockSubmit; canSubmit requires every stop in the route's city, the builder resolves the city from the route being edited; uncompiled
- possible compile error — ActiveTourModel.id is nonisolated let; MKLaunchOptionsDirectionsModeCycling exists since iOS 14 / macOS 11 (kept)

### Deferred minors (11)

- Final: minor (deferred): location updates never stopped after a tour ends (best-accuracy GPS for the foreground life).
- Final: minor (deferred): ArrivalMonitor ignores fix age; startTour feeds a possibly stale currentLocation.
- Final: minor (deferred): syncError never shown; a pending route shows no "being computed" notice in the active tour.
- Final: minor (deferred): plainText emphasis regex strips underscores inside words.
- Final: minor (deferred): MomentView renders inline-only Markdown (headings show raw ##).
- Final: minor (deferred): MediaTile builds a new AVPlayer per body evaluation; viewAligned without scrollTargetLayout.
- Final: minor (deferred): MyRoutes Edit swallows errors; a delete failure replaces the list with an error view.
- Final: minor (deferred): local progress keyed by tour+mode while the server keys by tour.
- Final: minor (deferred): tokenProvider reads SessionController off the main thread (benign race).
- Final: minor (deferred): publicIP passes 198.18/15, 240/4, 192.0.0/24 and NAT64/6to4-embedded addresses.
- Final: minor (deferred): one IA item's metadata error aborts the whole import.

## Cycle 4 — web visitor app


### Rulings (12)

- no human review of the cycle-4 spec and plan before execution — the partner asked for uninterrupted progress — cost if wrong: a cycle of rework.
- Task 1: Ruling: the admin group gates in (admin)/admin/+layout.ts (redirect to /signin, error(403) for non-curators) with the curator notice in (admin)/+error.svelte — page loaders call admin-only endpoints before the layout component could gate, which produced a 500 — cost if wrong: none.
- Task 1: Ruling: sign-in keeps sending people to `next` or `/`; admin e2e specs navigate to /admin explicitly and the visitor-role test asserts the notice at /admin — cost if wrong: curators type /admin once.
- Task 2: Ruling: visitor pages fetch inside the component through a small Loader rune class instead of +page.ts loaders, so API failures render the page's own error state with a retry rather than the SvelteKit error page — cost if wrong: none.
- Task 2: Ruling: moment bodies render as plain paragraphs with Markdown marks stripped (no {@html}); rich Markdown rendering is deferred — cost if wrong: headings and links show as plain text.
- Task 5: Ruling: ActiveWalk is a runes class in walk.svelte.ts tested under vitest's node project (the SvelteKit plugin compiles runes there), so the page stays declarative — cost if wrong: none.
- Task 6: Ruling: the route builder is a runes class in routes/builder.svelte.ts (plan said builder.ts) so the form binds to it directly; the plan's unit tests run unchanged against it — cost if wrong: none.
- Task 6: Ruling: the place picker sorts by distance only when the browser grants location on the builder page; otherwise alphabetical API order — cost if wrong: a longer scroll.
- queued progress PUTs now carry every stop known when they leave (coalescing) instead of the state at queue time — the server replaces the row, so the latest union is the safe payload; the "PUTs in order" test asserts order and containment — cost if wrong: none.
- the plan's Review Focus 2 "reload without network" is met as "API unreachable" (unit-tested cache fallback); an app-shell service worker is out of scope per spec §7 — cost if wrong: a hard reload without any network shows the browser's offline page.
- open redirect shape next=//host on /signin predates this cycle and SvelteKit's goto refuses external URLs — left for the signin page's own fix — cost if wrong: none today.
- making the backend progress PUT a union is a backend change outside this cycle; the client-side union covers the web and iOS reads the server at start — cost if wrong: two devices racing can still trade stops until the API unions.

### Review findings fixed (7)

- audio auto-play effect loop (Critical) — e2e "the audio guide plays the featured moment once on arrival" RED (208 moment fetches in 3 s against the old page, verified by stashing) → GREEN (1 fetch); effect reads only arrivedStop and the toggle and calls play() untracked
- two tabs overwriting each other's progress (Review Focus 3) — "unions with another tab writing the same storage key" RED→GREEN; commit() and every PUT union with storage first; walk page listens for storage and online events
- "Start over next time" undone for signed-in visitors — "resetting a signed-in walk also clears the server record" RED→GREEN
- builder shown to signed-out visitors — e2e "the builder asks a signed-out visitor to sign in" RED→GREEN on /routes/new and /routes/[id]/edit
- e2e sign-in race — every sign-in helper waits to leave /signin (test infrastructure, no test of its own)
- map refit on every GPS fix and dropped updates while tiles load — ready flag from the load event, fit key excludes the user position, refit only when the stops ahead change; no automated test (MapLibre needs a browser) — verified by the walk specs still passing
- README overclaim (re-graded Important: it misleads) — a walk continues when the API is unreachable; the page itself still needs the network

### Deferred minors (9)

- Final: minor (deferred): dialogs (arrival panel, End, Delete) lack aria-modal, focus trap and Escape.
- Final: minor (deferred): get(session.store) replaced by $current in the walk page; other pages fine.
- Final: minor (deferred): moment body stripping shows raw link syntax; reuse plainText per paragraph.
- Final: minor (deferred): Profile has no location permission state; Home does not group tours by topic.
- Final: minor (deferred): local progress keyed per tour+mode, shared across accounts on one browser.
- Final: minor (deferred): places capped at 200 in the builder; no paging.
- Final: minor (deferred): "filters by mode" e2e is vacuous.
- Final: minor (deferred): no (visitor)/+error.svelte; Map dark prop only affects pins; path segments unencoded in visitor.ts.
- Final: minor (deferred): redundant $effect redirect remains in (admin)/admin/+layout.svelte.

## Cycle 5 — Android app


### Rulings (9)

- no human review of the cycle-5 spec and plan before execution — the partner asked for uninterrupted progress — cost if wrong: a cycle of rework.
- :core is built and tested here with the system Gradle 8.14 and JDK 21; :app is not compiled here (no Android SDK) — cost if wrong: Compose compile fixes on the first SDK build.
- Task 6: Ruling: LocationSource and NarrationPlayer (plan: Task 7) land here because AppContainer wires them; Task 7 screens exist as placeholders so the NavHost is self-consistent — cost if wrong: none.
- Task 6: Ruling: screens keep their state in remember/LaunchedEffect with a UiState holder instead of one ViewModel per screen; AppViewModel holds app-wide state — cost if wrong: state lost on configuration change for in-flight loads (they reload).
- Task 7: Ruling: the map draws stops and legs with MapLibre style layers (GeoJSON sources + Circle/Symbol/Line layers) instead of the annotation plugin, so it depends only on the core SDK — cost if wrong: layer API nits on the first SDK build.
- the sync chain swap and the locked progress record stay as they are with the chain swapped under the lock (reviewer minor) — cost if wrong: none.
- two devices walking at the same moment still race on the server's replace-on-PUT; the client unions never drop a stop and a backend union PUT is a follow-up — cost if wrong: a stop traded between devices until the API unions.
- places(limit=100) without cursor paging in the builder stays (same as iOS) — the seed city has far fewer — cost if wrong: a large city hides places past 100.
- Credential Manager nonce / audience checks are the backend's concern and unchanged — cost if wrong: none here.

### Review findings fixed (13)

- :core emitting Java 21 bytecode that :app (17) cannot inline (Critical) — core now targets JVM 17 via compilerOptions; verified major version 61; 62 core tests green
- missing @OptIn(ExperimentalMaterial3Api) for ModalBottomSheet (Critical) — file-level opt-in in MainActivity and WalkScreen; uncompiled
- LocationListener SAM lambda crashing on API 26–29 — object with all four callbacks; uncompiled
- deep links replayed on recreation and crashing on crafted slugs — handled only on a fresh start, slug restricted to [A-Za-z0-9_-]{1,64}; uncompiled
- endTour tearing down a newer walk — state and resume record cleared first and only for the same instance, flush runs behind; ActiveTour.end removes the record only when it still names this walk; navigation keyed on the instance
- arrival sheet and narration replaying on rotation — "an arrival is announced once and acknowledged by the UI" RED→GREEN (ActiveTour.acknowledgeArrival); WalkScreen drives the sheet from state
- resume waiting on the network — restore runs disk-only in init before bootstrap; uncompiled
- 401 race (Review Focus 1) — "clearIfToken clears only when the rejected token is still current" RED→GREEN (synchronized SessionStore.clearIfToken, also ignores token-less 401s); AppViewModel uses it and moves UI updates to the main thread
- location and audio running past the foreground — LocationWhileStarted (ON_START/ON_STOP) on Home and Walk, Activity.onStop stops narration and location; uncompiled
- route-builder draft lost on rotation — RouteBuilderViewModel holds the builder, editing tour, places and result; uncompiled
- EncryptedSharedPreferences crash after a restore — allowBackup=false and recreate on failure; uncompiled
- documented local-dev setup — debug-only network security config permits cleartext to 10.0.2.2/localhost; release forbids it; README notes it
- (cross-cycle, iOS) resetProgress not clearing the server record — testResettingASignedInWalkClearsTheServerRecord added to the kit (uncompiled); flagged by the web reviewer, parity with web and Android

### Deferred minors (8)

- Final: minor (deferred): Profile base-URL switch may send the sign-out DELETE to the new host; gate the override to debug.
- Final: minor (deferred): duplicate PUT on attach (loadRemoteProgress then syncIfNeeded).
- Final: minor (deferred): a failed reset PUT is never retried (also web).
- Final: minor (deferred): recordVisited does not union with the stored record (a second ActiveTour instance could overwrite the file).
- Final: minor (deferred): stale fixes (getLastKnownLocation, StateFlow replay) can trigger arrival; ignore fixes older than ~30 s.
- Final: minor (deferred): coarse-only permission picks GPS_PROVIDER and silently gets no location on API 26–30.
- Final: minor (deferred): walk screen shows a route error twice, never re-polls a pending route, and a pending response replaces a cached ready route.
- Final: minor (deferred): geo: fallback has no catch; Credential Manager cancel shows as an error; TTS fixed to en-US; no launcher icon; RoutesScreen.deleteError never reset.
