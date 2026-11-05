# Android tours app — design

Date: 2026-11-05
Status: approved in conversation (continuing without stops); decisions recorded below
Cycle: 5 of 5

## 1. Goal

A native Android app with the same job as the iOS app (cycle 3) and the web
visitor site (cycle 4), on the same `/api/v1`: browse a city's tours by topic,
walk or ride a tour with a route line, arrival detection and an audio guide,
read the featured moment at each stop and browse the others, build and share
routes, save tours, and keep progress across devices.

## 2. Constraints and decisions

- **Kotlin, Jetpack Compose, Material 3.** Min SDK 26, target SDK 35, Kotlin
  2.1, AGP 8.7, Gradle 8.14 with a version catalog. No Play Services
  dependency for location or maps.
- **Two Gradle modules.** `:core` is a plain Kotlin/JVM library (DTOs with
  kotlinx.serialization, OkHttp API client, session store, arrival monitor,
  active-tour model, route builder, narration planning, directions URIs) with
  JUnit 5 tests against the fixtures recorded for iOS and OkHttp's
  `MockWebServer`. `:app` is the Compose app: ViewModels over `:core`,
  screens, MapLibre, location, audio, sign-in. This session has a JDK and
  Gradle but **no Android SDK**: `:core` is built and its tests run here;
  `:app` is written against stable AndroidX/Compose APIs and compiled on a
  machine with the SDK. The plan ends with that step marked as not run here.
- **Maps:** MapLibre Android SDK with the OpenFreeMap liberty style (no key),
  the same tiles as the web app. Route legs past/active/future, numbered stop
  markers, the user's position.
- **Location:** `android.location.LocationManager` with
  `requestLocationUpdates(GPS_PROVIDER / FUSED_PROVIDER when available)` while
  the walk screen is in the foreground; the same arrival rule as iOS and web
  (accuracy ≤ 65 m, distance ≤ radius + accuracy, once per stop); "I'm here"
  fallback; no background location.
- **Directions handoff:** `google.navigation:q=lat,lng&mode=w|b` when an
  activity resolves it, else `geo:lat,lng?q=lat,lng(name)`.
- **Audio guide:** `MediaPlayer` for `narration.public_url`, `TextToSpeech`
  for the Markdown-stripped body; audio focus requested with
  `AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK` and abandoned when playback ends; a
  foreground service is out of scope (audio stops when the activity stops).
- **Sign-in:** Google through Credential Manager (`androidx.credentials` +
  `googleid`) → `POST /auth/google`; developer email sign-in in debug builds.
  Token and account in `EncryptedSharedPreferences`; a 401 clears the session
  only when the rejected token is still current and shows sign-in after the
  active tour closes.
- **Progress:** local per tour and mode in app storage, merged by union with
  `GET /me/progress`, `PUT /me/progress/{tour}` sent in order and retried when
  stale (on arrival and on resume), mirroring `ActiveTourModel` on iOS.
- **Offline:** the active tour's detail and route, and the cities list, are
  cached as JSON files so a walk resumes after the app is killed or loses
  signal. The active tour id and mode are remembered and restored on launch.
- **Deep links:** `invisiblemuseum://tour/{slug}` and `https://*/t/{slug}`
  intent filters (the App Links association is a deployment step).
- **Design:** dark gallery theme matching iOS and web (near-black surfaces,
  white text, blue accent), Material 3 components, bottom navigation with
  Home · Tours · Routes · Saved · Profile.

## 3. Architecture

```
android/
  settings.gradle.kts, build.gradle.kts, gradle/libs.versions.toml, gradle.properties
  core/                                  Kotlin/JVM library
    src/main/kotlin/tv/subcult/museum/core/
      Models.kt          DTOs (City, Topic, MediaAsset, PlaceSummary, PlaceDetail, Source, MomentSummary,
                         MomentDetail, RouteSummary, RouteLeg, TourRoute, TourSummary, TourStop, TourDetail,
                         TourProgress, Account, Session, ListPage<T>, ApiErrorBody, request bodies)
      ApiClient.kt       OkHttp + kotlinx.serialization; bearer token provider; ApiException(status, code,
                         message, fields); onUnauthorized(tokenUsed)
      Endpoints.kt       typed calls for every public, auth and /me route
      SessionStore.kt    TokenStorage interface, MemoryTokenStorage, SessionStore(restore/set/update/clear)
      KeyValueStore.kt   interface for small JSON persistence; InMemoryKeyValueStore
      Estimates.kt       estimateText, distanceText, modeVerb
      ArrivalMonitor.kt  hasArrived(fix, stop, radiusM), ArrivalMonitor(update once per stop, retarget)
      ActiveTour.kt      ActiveTour(tour, mode, api, store, signedIn): route load with cache fallback,
                         next stop, legs split, local progress, remote merge, serialised sync, maps URI
      RouteBuilder.kt    2–30 stops in one city, modes, visibility, submit → fieldErrors
      Narration.kt       NarrationPlan (Stream/Speech/None), plainText(markdown)
      Directions.kt      navigationUri(lat, lng, mode), geoUri(lat, lng, name)
    src/test/kotlin/...  JUnit 5 + MockWebServer; fixtures copied from ios-invisible-museum/.../Fixtures
  app/                                   Android application (compiled on a machine with the SDK)
    src/main/kotlin/tv/subcult/museum/
      MuseumApp.kt, MainActivity.kt (NavHost, bottom bar, deep links, sign-in sheet)
      AppContainer.kt    wires ApiClient, SessionStore (EncryptedSharedPreferences), file KeyValueStore,
                         LocationSource, NarrationPlayer
      ui/theme/          Color, Type, Theme
      ui/components/     LoadState, TourCard, StopRow, MomentCard, MediaGallery, SignInPrompt
      ui/home/, ui/tours/ (list, detail, walk + map), ui/places/, ui/moments/, ui/routes/ (list, builder),
      ui/saved/, ui/profile/, ui/signin/
      location/LocationSource.kt   LocationManager → Flow<Fix>, permission state
      audio/NarrationPlayer.kt     MediaPlayer + TextToSpeech + audio focus
      session/EncryptedTokenStorage.kt
```

Every screen has a ViewModel that exposes `StateFlow<UiState>` with loading,
empty, error and ready states built from `:core` models; Composables render
state and forward intents. `ActiveTour` and `RouteBuilder` in `:core` carry
the logic and are observed through `StateFlow`s they expose.

## 4. Data flow

Same as iOS §4: startup restores the session and the last city (cached),
refreshes `/me`, loads `/cities`, and restores an interrupted walk; Home shows
tours and nearby places; tour detail shows per-mode estimates, Save, Start;
the walk screen draws the route, watches location, fires arrival, shows the
featured moment and the place's other moments, syncs progress; routes builder
posts or patches `/me/tours` and shares `{apiBase}/t/{slug}` through the
system share sheet; Saved reads `/me/saved` and `/me/progress`.

## 5. Testing

- `:core` JUnit 5 (run here): fixture decoding for every DTO, error envelope
  mapping and the 401 hook with `MockWebServer`, estimates, arrival rule and
  once-per-stop, `ActiveTour` (arrival advances, legs split, cache fallback,
  local progress, PUT body and order, remote merge, resume record),
  `RouteBuilder` (2–30, duplicates, cross-city, 422 fields, PATCH when
  editing, share URL), narration plan and `plainText`, directions URIs.
- `:app`: Compose UI tests are not written this cycle (no emulator); the
  first steps on a machine with the SDK are `./gradlew :core:test :app:assembleDebug`.

## 6. Out of scope

Background location and a foreground audio service, offline map tiles, push
notifications, Wear OS, account deletion, user-contributed moments.
