# iOS Tours App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the iOS app on the tours API: browse, walk or ride with arrival detection and audio, build and share routes, save and track progress.

**Architecture:** SwiftUI + Observation; one `AppModel` with `APIClient` and `SessionStore`; per-screen observable models; MapKit `Map` with `MapPolyline`; CoreLocation arrival monitor; AVFoundation narration. Existing design-system files reused.

**Tech Stack:** Swift 5.9, iOS 17, SwiftUI, MapKit, CoreLocation, AVFoundation, AuthenticationServices, XCTest. No packages.

**Spec:** `docs/superpowers/specs/2026-11-04-ios-tours-app-design.md`

## Global Constraints

- No Swift toolchain in the authoring session: every file must use only APIs the author is certain exist on iOS 17; prefer explicit types over inference-heavy code; no result builders beyond SwiftUI's own.
- DTO field names mirror `backend/api/openapi.yaml` exactly through `CodingKeys`; snake_case keys decoded with `.convertFromSnakeCase` are not used (explicit keys only), so the OpenAPI document stays the single source of truth.
- Keep `Theme.swift` and `Museum*.swift` unchanged except for removed references.
- Every network call goes through `APIClient`; views never touch `URLSession`.
- Register every new file in `project.pbxproj` (build file + file reference + group child + sources phase) using the existing `A001…`/`A002…` id scheme; remove deleted files from all four places.

## Review Focus

1. Arrival detection when GPS accuracy is poor: the monitor must require `horizontalAccuracy <= 65 m` and distance within radius plus accuracy, so a 200 m fix in a car park does not trigger "arrived". Task 4.
2. Losing signal mid-tour: the active tour continues from the cached route; progress is kept locally and synced when `PUT /me/progress` succeeds. Task 5.
3. A 401 on any call clears the session and shows sign-in without losing the active tour. Task 2.
4. Narration audio with the screen locked: audio session category `.playback` is set before play and the player keeps a strong reference. Task 6.
5. Route builder with fewer than 2 stops or across cities: the Create button stays disabled and the API's 422 field message is shown next to the stops list. Task 7.

---

### Task 1: Prune the old model and views; API layer

**Files:** delete `Models.swift`, `SampleData.swift`, `SampleTours.swift`, `MuseumStore.swift`, `MuseumRepository.swift`, `MuseumLocalPersistence.swift`, `WalkingRouteModels.swift`, `WalkingRouteService.swift`, `ExploreView.swift`, `MapListView.swift`, `ContributeView.swift`, `CollectionView.swift`, `SavedView.swift`, `ProfileView.swift`, `MuseumDetailView.swift`, `ExhibitDetailView.swift`, `ContributionModerationView.swift`, `ContentView.swift`, `APIClient.swift`. Create `API/APIModels.swift`, `API/APIClient.swift`, `API/Endpoints.swift`; tests `InvisibleMuseumTests/APIModelsTests.swift` with fixtures recorded from the API.

- [ ] Record fixtures from the running API: city detail, tour detail, route, moment detail, place detail, session.
- [ ] Write `APIModelsTests` decoding each fixture and asserting key fields (tour has 4 stops; stop 3 featured moment falls back; route geometry count).
- [ ] Implement DTOs (`City`, `Topic`, `PlaceSummary`, `PlaceDetail`, `MomentSummary`, `MomentDetail`, `MediaAsset`, `Source`, `TourSummary`, `TourDetail`, `TourStop`, `RouteSummary`, `TourRoute`, `RouteLeg`, `TourProgress`, `Account`, `Session`, `ListPage<T>`, `APIErrorBody`), `APIClient` (base URL, token provider, `get/post/put/patch/delete`, `APIError` with status/code/message/fields, 401 hook), `Endpoints` (typed functions for every public, auth, and `/me` route).
- [ ] Commit: `feat(ios): API layer for the tours contract; remove the old model`.

### Task 2: App model, session, sign-in, root tabs

**Files:** `App/AppModel.swift`, `App/RootView.swift`, `Session/SessionStore.swift` (Keychain), `Session/SignInView.swift`, modify `InvisibleMuseumApp.swift`, `Info.plist` keys (`NSLocationWhenInUseUsageDescription`, URL scheme `invisiblemuseum`), entitlements for Sign in with Apple.

- [ ] Tests: `SessionStoreTests` (set/clear/restore with an in-memory keychain shim), `AppModelTests` (401 clears session and keeps `activeTour`).
- [ ] Implement; tabs: Home, Tours, Routes, Saved, Profile using `MuseumBottomBar`.
- [ ] Commit: `feat(ios): app model, Keychain session, Sign in with Apple, tabs`.

### Task 3: Home, tour list, tour detail, place and moment views

**Files:** `Features/Home/HomeView.swift`, `Features/Tours/TourListView.swift`, `Features/Tours/TourDetailView.swift`, `Features/Tours/TourDetailModel.swift`, `Features/Places/PlaceDetailView.swift`, `Features/Places/MomentView.swift`, `Features/Places/MediaGalleryView.swift`.

- [ ] Tests: `TourDetailModelTests` with a stubbed client: loads tour, exposes per-mode estimate text ("24 min walk"), save toggles call the right endpoints.
- [ ] Implement with loading/empty/error states.
- [ ] Commit: `feat(ios): browse cities, tours, places, and moments`.

### Task 4: Location and arrival

**Files:** `Location/LocationService.swift` (trim), `Location/ArrivalMonitor.swift`, tests `ArrivalMonitorTests`.

- [ ] Tests: inside radius with 10 m accuracy → arrived; inside radius with 200 m accuracy → not arrived; outside radius → not; arrival fires once per stop until the stop changes (Review Focus 1).
- [ ] Implement `ArrivalMonitor(stop: coordinate, radius: Double)` with `evaluate(location: CLLocation) -> Bool` pure function plus a Combine-free callback wrapper.
- [ ] Commit: `feat(ios): location service and arrival monitor`.

### Task 5: Active tour with map, route, progress, cache

**Files:** `Features/Tours/TourMapView.swift`, `Features/Tours/ActiveTourView.swift`, `Features/Tours/ActiveTourModel.swift`, `Features/Tours/StopMomentSheet.swift`, `Storage/DiskCache.swift`, tests `ActiveTourModelTests`, `DiskCacheTests`.

- [ ] Tests: next stop advances on arrival; visited positions persist to `DiskCache` and are posted to `/me/progress` when signed in (stubbed client records the PUT); route legs split active/future; `openInMaps` builds walking vs cycling launch options (Review Focus 2).
- [ ] Implement; `Map` with `MapPolyline` for active (accent) and future (muted) legs, `Annotation` per stop numbered, `UserAnnotation`.
- [ ] Commit: `feat(ios): walk or ride a tour with route, arrival, and progress`.

### Task 6: Audio guide

**Files:** `Audio/NarrationPlayer.swift`, tests `NarrationPlayerTests` (fallback selection: narration URL present → player; absent → synthesizer; audio session category set before play).

- [ ] Implement with `AVAudioSession.sharedInstance().setCategory(.playback, mode: .spokenAudio, options: [.duckOthers])`, `AVPlayer`, `AVSpeechSynthesizer` with `AVSpeechUtterance(string:)` from the moment body with Markdown stripped (reuse a small `plainText`).
- [ ] Wire the audio toggle in `ActiveTourView`; on arrival auto-play when enabled.
- [ ] Commit: `feat(ios): audio guide with narration and on-device fallback`.

### Task 7: Routes, saved, profile

**Files:** `Features/Routes/MyRoutesView.swift`, `Features/Routes/RouteBuilderView.swift`, `Features/Routes/RouteBuilderModel.swift`, `Features/Saved/SavedView.swift`, `Features/Profile/ProfileView.swift`, tests `RouteBuilderModelTests` (create disabled under 2 stops; 422 fields surface on `stops`; share URL built from slug) (Review Focus 5).

- [ ] Implement; share via `ShareLink`.
- [ ] Commit: `feat(ios): build and share routes, saved tours, profile`.

### Task 8: Project file, docs, handoff

**Files:** `project.pbxproj`, `README.md` (iOS section), `Makefile` (`ios-test`), `docs/superpowers/plans/...` checkboxes.

- [ ] Register all files; remove deleted ones; add a unit test target if absent.
- [ ] README: state that the app targets the new API, how to point at a local API, and that the cycle's first Mac step is `make ios-build && make ios-test`.
- [ ] Commit: `chore(ios): register sources and document the build`.
- [ ] Not run here: `xcodebuild` build and test on a Mac.
