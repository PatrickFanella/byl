# Invisible Museum: iOS App Rebuild

Date: 2026-11-04
Status: approved in conversation (continuing without stops); implementation in progress
Cycle: 3 of 5

## 1. Goal

Replace the AR-era SwiftUI prototype with a tours app on the cycle-1 API:
browse topic tours and places in a city, walk or bike a tour with an in-app
route line and arrival detection, read or listen to the featured moment at
each stop and browse the others, build and share your own routes, save
tours, and keep progress across devices.

## 2. Constraints and honesty

- SwiftUI, iOS 17+, no third-party dependencies. MapKit for maps and the
  Apple Maps handoff, CoreLocation for position and arrival, AVFoundation
  for narration playback, AVSpeechSynthesizer for on-device fallback,
  AuthenticationServices for Sign in with Apple.
- The existing design system files (`Theme.swift`, `Museum*.swift`) are
  kept and reused. Everything that encodes the old content model
  (`Models.swift`, `SampleData.swift`, `SampleTours.swift`, `MuseumStore`,
  `MuseumRepository`, `MuseumLocalPersistence`, `WalkingRoute*`, the Explore,
  Map, Contribute, Collection, Saved, Profile, Detail, and Moderation views)
  is deleted and rewritten.
- **This session has no Swift toolchain.** The code is written to compile
  against iOS 17 SDK APIs that are stable and well known, kept free of
  clever generics, and organised so that compile errors, if any, are local.
  The first `xcodebuild` on a Mac is the gate before this cycle is called
  done; the plan ends with that step marked as not run here.

## 3. Architecture

```
InvisibleMuseum/
  App/            InvisibleMuseumApp.swift, AppModel.swift (session, config), RootView.swift (tabs)
  API/            APIClient.swift (URLSession, bearer, error envelope), APIModels.swift (Codable DTOs
                  matching openapi.yaml), Endpoints.swift (typed calls)
  Session/        SessionStore.swift (Keychain token, account), SignInView.swift (Apple + dev)
  Location/       LocationService.swift (existing, trimmed), ArrivalMonitor.swift (geofence by distance)
  Audio/          NarrationPlayer.swift (AVPlayer for narration_url, AVSpeechSynthesizer fallback)
  Features/
    Home/         HomeView.swift (city picker, featured tours, nearby places)
    Tours/        TourListView.swift, TourDetailView.swift (stops, modes, start), TourMapView.swift (route line,
                  stops, user), ActiveTourView.swift (walking mode: next stop, distance, arrival sheet,
                  audio toggle), StopMomentSheet.swift (featured moment + other moments at the place)
    Places/       PlaceDetailView.swift, MomentView.swift (body, media gallery, sources, narration)
    Routes/       MyRoutesView.swift, RouteBuilderView.swift (pick published places, order, name, mode,
                  share link)
    Saved/        SavedView.swift
    Profile/      ProfileView.swift (account, API URL override, sign out)
  Design/         Theme.swift, MuseumScreen.swift, MuseumCard.swift, MuseumButton.swift, MuseumChip.swift,
                  MuseumField.swift, MuseumSection.swift, MuseumBottomBar.swift (existing)
```

State: one `@Observable` `AppModel` injected through the environment holds
the `APIClient`, `SessionStore`, selected city, and caches for tours and
places. Views load through small `@Observable` view models per screen
(`TourDetailModel`, `ActiveTourModel`, `RouteBuilderModel`) so network and
location logic stays out of view bodies.

## 4. Data flow

- **Startup:** read API base URL (UserDefaults override, Info.plist, default
  `https://museum-api.subcult.tv`), restore the session token from the
  Keychain, `GET /me` to refresh the account, `GET /cities`, pick the last
  used city or the nearest to the user.
- **Home:** `GET /cities/{city}/tours` and `GET /cities/{city}/places?near=`
  when location is available.
- **Tour detail:** `GET /tours/{tour}`; shows stops with featured moments,
  per-mode distance and estimated minutes, Save, Start walking / Start
  riding. Unlisted user routes open from a shared link
  `https://museum-api.subcult.tv/t/{slug}` (universal link placeholder) or
  the `invisiblemuseum://tour/{slug}` scheme.
- **Active tour:** `GET /tours/{tour}/route?mode=` draws the line; the
  active leg is highlighted; `ArrivalMonitor` fires when the user is within
  the place's `arrival_radius_m` of the next stop; arrival presents the
  featured moment sheet, marks the stop visited, and `PUT /me/progress/{tour}`
  when signed in (local progress otherwise). "Open in Maps" hands the leg to
  Apple Maps with walking or cycling directions.
- **Audio mode:** a toggle on the active tour; on arrival the featured
  moment's `narration.public_url` plays through `AVPlayer`; when null, the
  body is read by `AVSpeechSynthesizer`. Audio session is `.playback` with
  `.spokenAudio` mode and `.duckOthers`.
- **Moments:** `GET /moments/{id}` for body, media, sources; a horizontal
  media gallery with captions and credits; sources listed with links.
- **User routes:** signed-in only. Builder lists published places in the
  city (search by name, sort by distance), ordered list with drag to
  reorder, mode toggles, name, visibility unlisted or private;
  `POST /me/tours`; share sheet with the link. `GET /me/tours` lists mine
  with edit (`PATCH`) and delete.
- **Saved:** `GET /me/saved`, `PUT|DELETE /me/saved/{tour}`; progress from
  `GET /me/progress` shown as "3 of 4 stops".
- **Sign-in:** Sign in with Apple (identity token → `POST /auth/apple`), and
  a developer email sign-in shown only when the API returns 200 for it
  (debug builds). Token in Keychain; account cached; 401 clears the session.

## 5. Offline and errors

- Last loaded city, tours, and the active tour's detail and route are cached
  as JSON in Application Support so an active tour survives losing signal.
- Every screen has loading, empty, and error states using the existing
  `MuseumScreen` and card primitives; errors show the API message.

## 6. Testing

- Unit tests (XCTest, runnable on a Mac): DTO decoding against fixtures
  recorded from the running API (`Fixtures/*.json`), `ArrivalMonitor`
  distance logic, route leg selection, `APIClient` error envelope mapping
  with a `URLProtocol` stub.
- UI smoke test: launch, home renders with fixture data via a launch
  argument that points the client at a local stub.
- Not run in this session (no toolchain); listed as the first step on a Mac.

## 7. Out of scope

Push notifications, background location, watchOS, offline map tiles, and
account deletion flows. User-contributed moments (cycle after Android).
