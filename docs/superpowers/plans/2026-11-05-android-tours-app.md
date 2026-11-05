# Android Tours App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Kotlin/Compose Android app on the cycle-1 API with a JVM-tested `:core` module carrying every piece of logic, and a `:app` module written for compilation on a machine with the Android SDK.

**Architecture:** `android/` Gradle project with `:core` (Kotlin/JVM, OkHttp, kotlinx.serialization, JUnit 5 + MockWebServer, built and tested here) and `:app` (Compose Material 3, Navigation, MapLibre, LocationManager, MediaPlayer/TextToSpeech, Credential Manager; not compiled here). ViewModels observe `StateFlow`s exposed by `:core` classes.

**Tech Stack:** Kotlin 2.1.20, Gradle 8.14, AGP 8.7.3, kotlinx.serialization 1.8.1, kotlinx.coroutines 1.10.2, OkHttp 4.12.0, JUnit 5.11, Compose BOM 2024.12.01, MapLibre Android 11.8.0, androidx.credentials 1.3.0, security-crypto 1.1.0-alpha06.

**Spec:** `docs/superpowers/specs/2026-11-05-android-tours-app-design.md`

## Global Constraints

- Package `tv.subcult.museum.core` for `:core`, `tv.subcult.museum` for `:app`; min SDK 26, target SDK 35.
- `:core` has no Android imports; everything with an Android dependency (storage, location, audio, sign-in, UI) lives in `:app` behind the `:core` interfaces `TokenStorage`, `KeyValueStore`.
- JSON field names follow `backend/api/openapi.yaml` through `@SerialName`; `Json { ignoreUnknownKeys = true; explicitNulls = false }`.
- Arrival rule: accuracy ≤ 65 m and distance ≤ radius + accuracy; fires once per stop.
- Run `:core` with `cd android && gradle :core:test --no-daemon` (the wrapper jar is not committed; `gradle wrapper` is a Mac step). Never run `:app` tasks here.
- Fixtures: copy `ios-invisible-museum/InvisibleMuseumKit/Tests/InvisibleMuseumKitTests/Fixtures/*.json` to `android/core/src/test/resources/fixtures/`.
- Commits use `-c user.name="Claude" -c user.email="noreply@anthropic.com"` and the session trailers.

## Review Focus

1. A 401 that lands after a fresh sign-in must not clear the new session; the hook carries the rejected token and the app compares. Task 2.
2. Resume after the process is killed mid-walk: the active tour record plus cached detail bring the walk back without network. Task 4.
3. Second device or other mode: merging server progress never drops a visited stop; PUTs are serialised. Task 4.
4. Cross-city stops in the route builder keep Create disabled; a 422 field message is mapped next to the stops. Task 5.
5. Poor GPS (200 m fix) must not trigger arrival; "I'm here" must. Task 3.

---

### Task 1: Gradle project, DTOs, fixtures

**Files:**
- Create: `android/settings.gradle.kts`, `android/build.gradle.kts`, `android/gradle.properties`, `android/gradle/libs.versions.toml`, `android/core/build.gradle.kts`, `android/.gitignore`.
- Create: `android/core/src/main/kotlin/tv/subcult/museum/core/Models.kt`, `Json.kt` (`val MuseumJson`).
- Test: `android/core/src/test/kotlin/tv/subcult/museum/core/ModelsTest.kt`, fixtures under `android/core/src/test/resources/fixtures/`.

**Interfaces:**
- Produces: every DTO named in the spec §3 `Models.kt`; `ListPage<T>(items, nextCursor)`; `TourDetail.tour: TourSummary` flattened via a custom serializer? No — Kotlin: `TourDetail` is `TourSummary`'s fields plus `stops` as one flat `@Serializable data class`; same for `PlaceDetail`, `MomentDetail` (flat classes, no inheritance). `TourSummary.isUserRoute`.

- [ ] **Step 1: Failing tests** — `decodes the city with topics` (slug `downers-grove`, 4 topics), `decodes the tour with stops and routes` (4 stops, stop 3 `featuredMomentId == null` and `featuredMoment?.title == "Summer Concert Lawn"`, `routes["walk"]?.estimatedMinutes == 24`), `decodes the route` (3 legs, geometry 4 coordinates), `decodes the place` (primary moment id equals `primaryMomentId`), `decodes the moment` (2 sources, 2 topics, `narration == null`), `decodes the session and the user tour` (slug length 10, `isUserRoute`), `decodes the error envelope` (fields `stops`), `round-trips the tour and route through JSON` (encode → decode equal).
- [ ] **Step 2: Run** `gradle :core:test --no-daemon` — Expected: compilation FAIL (no Models).
- [ ] **Step 3: Implement.** **Step 4: Run** — PASS. **Step 5: Commit** `feat(android): Gradle project and API models`.

### Task 2: API client, endpoints, session store

**Files:**
- Create: `ApiClient.kt`, `Endpoints.kt`, `SessionStore.kt`, `KeyValueStore.kt` in `android/core/src/main/kotlin/tv/subcult/museum/core/`.
- Test: `ApiClientTest.kt`, `SessionStoreTest.kt`.

**Interfaces:**
- Produces: `class ApiException(val status: Int, val code: String, override val message: String, val fields: Map<String, String>) : Exception`; `class ApiClient(baseUrl: String, tokenProvider: () -> String?, client: OkHttpClient = OkHttpClient()) { var onUnauthorized: ((String?) -> Unit)?; suspend inline fun <reified T> get(path, query: Map<String,String> = emptyMap()): T; post/put/patch(path, body: Any?): T; suspend fun delete(path) }`; `class Endpoints(val client: ApiClient)` with the same call list as iOS (`cities, city, tours, places, place, moment, tour, route, signInGoogle, signInDev, me, signOut, myTours, createTour, updateTour, deleteTour, saved, save, unsave, progress, putProgress`); `interface TokenStorage { fun read(key): String?; fun write(key, value); fun remove(key) }`; `class SessionStore(storage: TokenStorage) { val state: StateFlow<Session?>; fun restore(): Boolean; fun set(session); fun update(account); fun clear(); val token: String? }`; `interface KeyValueStore { fun get(key): String?; fun put(key, value); fun remove(key) }` + `InMemoryKeyValueStore`.

- [ ] **Step 1: Failing tests** with `MockWebServer`: bearer header sent; 401 calls `onUnauthorized("old-token")` and throws `ApiException(status 401, message "sign in required")` (Review Focus 1); 422 maps `fields["stops"]`; 204 on DELETE; `SessionStore` set/restore/clear/update as on iOS.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS. **Step 5: Commit** `feat(android): API client, endpoints, session store`.

### Task 3: Estimates, arrival, narration, directions

**Files:**
- Create: `Estimates.kt`, `ArrivalMonitor.kt`, `Narration.kt`, `Directions.kt`.
- Test: `EstimatesTest.kt`, `ArrivalMonitorTest.kt`, `NarrationTest.kt`, `DirectionsTest.kt`.

**Interfaces:**
- Produces: `fun estimateText(route: RouteSummary?): String?` ("24 min walk"/"7 min ride"), `fun distanceText(m: Double): String`, `fun modeVerb(mode)`; `data class Fix(lat, lng, accuracyM)`, `data class Point(lat, lng)`, `fun distanceM(a: Point, b: Point): Double`, `fun hasArrived(fix, stop, radiusM): Boolean`, `class ArrivalMonitor(stop, radiusM) { fun update(fix): Boolean; fun retarget(stop, radiusM); val hasArrived; fun distance(fix) }`; `sealed class NarrationPlan { Stream(url), Speech(text), None }`, `fun planNarration(url: String?, body: String): NarrationPlan`, `fun plainText(markdown): String`; `fun navigationUri(lat, lng, mode): String` (`google.navigation:q=lat,lng&mode=w|b`), `fun geoUri(lat, lng, name): String`.

- [ ] **Step 1: Failing tests** with the same values as iOS/web (20 m/10 m → true; 5 m/200 m → false (Review Focus 5); 120 m → false; 50 m/15 m → true, 60 m → false; -1 → false; 65 ok, 66 not; once per stop until retarget; estimates "24 min walk", "7 min ride", 650 s → "11 min walk", pending → null; plainText fixture sentence; `navigationUri(41.796,-88.0093,"bike") == "google.navigation:q=41.796,-88.0093&mode=b"`).
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS. **Step 5: Commit** `feat(android): arrival, estimates, narration, directions`.

### Task 4: ActiveTour with cache, progress, and resume

**Files:**
- Create: `ActiveTour.kt` (`LocalProgress`, `ActiveTourRecord`, `ActiveTour`).
- Test: `ActiveTourTest.kt` (MockWebServer + `InMemoryKeyValueStore`).

**Interfaces:**
- Produces: `class ActiveTour(api: Endpoints, tour: TourDetail, mode: String, store: KeyValueStore, signedIn: () -> Boolean, scope: CoroutineScope) { val state: StateFlow<ActiveTourState>; suspend fun loadRoute(); fun handleFix(fix); fun markArrived(); suspend fun loadRemoteProgress(); suspend fun syncIfNeeded(); suspend fun flush(); fun end(); fun resetProgress(); fun navigationUri(): String?; companion fun restore(api, store, signedIn, scope): ActiveTour? }`; `data class ActiveTourState(route: TourRoute?, routeStatus: LoadStatus, routeFromCache: Boolean, visited: Set<Int>, nextStop: TourStop?, arrivedStop: TourStop?, distanceToNextM: Double?, syncError: String?, isComplete: Boolean, pastLegs/activeLeg/futureLegs)`; keys `tour.{id}`, `route.{id}.{mode}`, `progress.{id}.{mode}`, `active`.

- [ ] **Step 1: Failing tests** mirroring `ActiveTourModelTests`: next stop advances on arrival; poor fix ignored; progress persisted and PUT body `{mode, visited_positions:[1], completed:false}` when signed in; local only when signed out; completion `completed:true`; manual arrival; legs split; route cache fallback when the server answers 500 (`routeFromCache`); failure without cache reports "down"; server progress merged (remote `[1,2]` → next 3, no PUT); local ahead pushed after merge; stale progress re-syncs; PUTs in order `[[1],[1,2]]`; `restore` after a new instance on the same store returns the tour and visited `[1]`, and `null` after `end()` (Review Focus 2, 3).
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS. **Step 5: Commit** `feat(android): active tour with cache, progress, and resume`.

### Task 5: RouteBuilder

**Files:**
- Create: `RouteBuilder.kt`. Test: `RouteBuilderTest.kt`.

**Interfaces:**
- Produces: `class RouteBuilder(city: City, editing: TourDetail? = null) { val state: StateFlow<RouteBuilderState>; var title/summary/walk/bike/visibility (setters update state); fun add(place); remove(id); move(from,to); contains(place); suspend fun submit(api: Endpoints): TourDetail? }`; `RouteBuilderState(stops, canSubmit, stopsHint, stopsError, error, busy, foreignStops)`; `fun shareUrl(base: String, slug: String)`.

- [ ] **Step 1: Failing tests**: disabled under 2 with hint "Add at least 2 stops (1 so far)"; duplicates; cap 30 "30 stops is the maximum"; cross-city "Every stop must be in Downers Grove" (Review Focus 4); POST body `{city, title, modes:["walk"], visibility:"private", stops:[{place_id}]}`; 422 → `stopsError`; editing prefills and PATCHes; share URL `.../t/abc123XYZ0`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS. **Step 5: Commit** `feat(android): route builder`.

### Task 6: App module — container, theme, navigation, sign-in, browse screens

**Files:**
- Create: `android/app/build.gradle.kts`, `android/app/src/main/AndroidManifest.xml` (permissions `INTERNET`, `ACCESS_FINE_LOCATION`, `ACCESS_COARSE_LOCATION`; intent filters for `invisiblemuseum://tour/*` and `https` `/t/*`), `android/app/src/main/res/values/{strings,themes}.xml`, `android/app/src/main/kotlin/tv/subcult/museum/{MuseumApp.kt,MainActivity.kt,AppContainer.kt,session/EncryptedTokenStorage.kt,storage/FileKeyValueStore.kt,ui/theme/*.kt,ui/components/*.kt,ui/home/HomeScreen.kt,ui/tours/{TourListScreen,TourDetailScreen}.kt,ui/places/PlaceScreen.kt,ui/moments/MomentScreen.kt,ui/signin/SignInScreen.kt,ui/AppViewModel.kt}`.
- [ ] **Step 1: Implement** (no tests run here) with loading/empty/error states; `AppViewModel` restores session + cities + active tour on start; Google sign-in via Credential Manager when `GOOGLE_SERVER_CLIENT_ID` (BuildConfig from `local.properties`) is set, dev email sign-in in debug.
- [ ] **Step 2: Verify** `gradle :core:test --no-daemon` still green; `:app` not compiled here.
- [ ] **Step 3: Commit** `feat(android): app shell, sign-in, browse screens`.

### Task 7: App module — walk screen, audio, routes, saved, profile

**Files:**
- Create: `ui/tours/WalkScreen.kt`, `ui/tours/TourMap.kt` (MapLibre `MapView` in `AndroidView`), `location/LocationSource.kt`, `audio/NarrationPlayer.kt`, `ui/routes/{RoutesScreen,RouteBuilderScreen}.kt`, `ui/saved/SavedScreen.kt`, `ui/profile/ProfileScreen.kt`.
- [ ] **Step 1: Implement** (walk: map with legs and numbered markers, next-stop card, I'm here, Directions intent, audio toggle, arrival sheet with the featured moment and the place's others, End dialog; routes: builder with up/down reorder, search, share via `ACTION_SEND`; saved with progress; profile with account, audio preference, API base URL override).
- [ ] **Step 2: Verify** `gradle :core:test --no-daemon` green. **Step 3: Commit** `feat(android): walk, audio guide, routes, saved, profile`.

### Task 8: Docs and handoff

- [ ] README Android section (what it does, `cd android && gradle :core:test`, first SDK steps `./gradlew :core:test :app:assembleDebug` after `gradle wrapper`), Makefile `android-test`, plan checkboxes.
- [ ] Commit `docs(android): module layout and build`.
- [ ] Not run here: `:app:assembleDebug` and any Compose UI test.
