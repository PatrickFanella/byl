# Invisible Museum User Stories + Gap Audit

Updated: 2026-06-15

> **Historical note (2026-11-04):** this audit describes the pre-rebuild AR prototype. AR has since been removed and the product is being rebuilt around walking/biking tours. Stories 3 and 5 no longer apply. See `docs/superpowers/specs/` for the current direction.

## Purpose

This document captures the product behavior the app should support, checks what is currently fulfilled, and identifies what needs work. It is intentionally user-facing: each story describes what someone expects to happen, not only what the code currently does.

## Status legend

- **Fulfilled** — works well enough for prototype testing.
- **Partial** — exists, but the flow is confusing, incomplete, or not fully wired to real behavior.
- **Missing** — not implemented yet.

---

## Core visitor stories

### 1. Discover nearby places

**Story:** As a visitor, I want to open the app and immediately see nearby invisible museum spaces so I can decide where to go.

**Status:** Partial

**Current behavior:**
- Explore loads spaces from the backend or sample fallback.
- Explore shows nearby cards and a styled abstract map preview.
- The backend currently seeds Downers Grove spaces.

**Gaps:**
- Explore does not clearly sort/re-rank by the user's live device location.
- Distance values mostly come from seeded/backend values rather than always being recomputed on-device.
- The abstract map preview is decorative; the real map is now in the Map tab.

**Acceptance criteria:**
- App requests/uses location in a controlled way.
- Nearby spaces are sorted by live distance when location is available.
- Each card shows live distance and whether AR is available now or requires walking directions.

### 2. Explore places on a map

**Story:** As a visitor, I want a map view with all nearby locations so I can understand where things are relative to me.

**Status:** Partial

**Current behavior:**
- A primary **Map** tab exists.
- Map tab uses MapKit with markers, user annotation, filters, selected-space card, and list navigation.
- Map tab now draws a walking route line and shows distance/ETA for the selected space when location is available.
- Map tab now has a local walking mode with a pinned route card, Start Walking / End actions, and a basic Downtown Memory Loop tour flow with ordered stops and visited state.

**Gaps:**
- Marker selection can be more obvious visually.
- The map could better explain marker colors/types.
- Tour flow is still prototype-simple: no offline media, no chained inter-stop route logic beyond the active stop, and no saved tour history beyond local progress.

**Acceptance criteria:**
- Map tab is visible from bottom nav.
- User can tap a marker and see a compact site card.
- User can open site details from the marker card.
- Optional next step: show walking route/time to selected marker.

### 3. Decide whether to walk or use AR

**Story:** As a visitor, when I tap **Open AR / Directions**, I want the app to decide what to show based on my distance from the place.

**Status:** Partial

**Current behavior:**
- The detail screen checks location state before opening AR.
- Unknown, denied, nearby, and too-far states show an in-app prompt first.
- Only `inside` opens `ARMuseumView` directly.
- `ARMuseumView` still handles the fallback guidance experience once opened.
- Backend `radiusMeters` is a discovery/site radius; AR activation is capped on-device at ~50m.

**Main UX gap:**
- Even when the user is too far, they still enter the AR full-screen flow first. This makes it feel like the app “stays on the AR screen.”

**Preferred behavior:**
- The detail screen should make a routing decision before opening AR:
  - **Too far:** show an in-app directions prompt or map guidance sheet.
  - **Nearby:** show the directions prompt until the user is inside the AR activation radius.
  - **Unknown location:** ask for location permission first.
  - **Preference override:** let user choose “Always show prompt,” “Auto-open directions when far,” or “Always ask.”
- AR should only unlock within roughly 50m of the site, even if the backend discovery radius is larger.

**Acceptance criteria:**
- If distance is beyond the activation radius, tapping **Open AR / Directions** does not immediately show camera AR.
- The user sees a clear prompt: “You’re too far away. Open walking directions?”
- The prompt includes a compact site card with name, distance, and CTA buttons.
- CTA options: **Open Walking Directions**, **View on Map**, **Try AR Anyway**.

### 4. Get walking directions

**Story:** As a visitor who is too far away, I want one clear action to get walking directions to the selected place.

**Status:** Partial

**Current behavior:**
- The AR guidance card has a **Walking directions** button.
- It opens Apple Maps walking directions via `MKMapItem.openInMaps`.

**Gaps:**
- The app does not dismiss or clearly transition after opening Apple Maps.
- There is no in-app route preview.
- There is no user preference for automatic directions vs prompt.

**Acceptance criteria:**
- From a place detail page, far-away users get a directions prompt before camera AR.
- Opening Apple Maps is explicit and expected.
- Returning to the app lands somewhere sensible: detail page or map view, not an ambiguous AR screen.

### 5. Use nearby AR

**Story:** As a visitor near a selected place, I want AR to open and place exhibit markers in the world.

**Status:** Partial

**Current behavior:**
- The app uses ARKit/RealityKit wrappers.
- It attempts `ARGeoTrackingConfiguration` when supported/available.
- It falls back to `ARWorldTrackingConfiguration` when geo tracking is unavailable.
- Exhibit marker entities are placed using existing `Exhibit.offset` data.

**Gaps:**
- Real AR needs physical-device testing.
- Marker interaction is minimal.
- There is no robust per-exhibit geo coordinate model.
- Geo tracking availability depends on device/location.

**Acceptance criteria:**
- On an ARKit-capable iPhone, nearby spaces open camera AR.
- HUD clearly states whether mode is Geo AR or approximate AR.
- Markers are visible and stable enough for prototype testing.
- If AR is unsupported, the app falls back to map/directions.

### 6. Save exhibits

**Story:** As a visitor, I want to save exhibits so I can revisit them later.

**Status:** Fulfilled for local prototype

**Current behavior:**
- Saved exhibit IDs persist locally.
- Collection tab lists saved exhibits.
- Exhibit detail supports save/unsave.

**Gaps:**
- Saved state is local only, not account-backed.
- No cross-device sync.

### 7. Download/offline a place

**Story:** As a visitor, I want to download a place for offline viewing before walking around.

**Status:** Partial

**Current behavior:**
- Download/offline state persists locally.
- UI has offline toggles/status.

**Gaps:**
- No real media package download/cache.
- Offline behavior is not verified with network disabled.

### 8. Contribute a story

**Story:** As a local contributor, I want to submit a story or memory for a place.

**Status:** Partial

**Current behavior:**
- Contribution form validates title/body/space.
- Contribution posts to backend.
- Backend stores pending contributions.

**Gaps:**
- No image/audio upload.
- No contributor account or submission history.
- No draft persistence.

### 9. Moderate contributions

**Story:** As an admin, I want to review pending contributions and approve/reject them.

**Status:** Partial

**Current behavior:**
- Admin token gates moderation calls.
- Profile lets admin store/clear token.
- Moderation view lists pending items and supports approve/reject.

**Gaps:**
- Static admin token only; no real user auth/roles.
- No audit trail or moderation notes.

---

## Recommended next implementation batch

### Batch A: Open AR / Directions decision flow (partially implemented)

This should be next because it directly matches the current confusion.

**Build:**
- Add a reusable `PlaceActionDecision` model.
- Add a location/proximity check on `MuseumDetailView` before presenting AR.
- Add a `DirectionsPromptView` sheet with:
  - site card,
  - distance,
  - **Open Walking Directions**,
  - **View on Map**,
  - **Try AR Anyway**.
- Add a simple preference:
  - `automatic` — if too far, show directions prompt;
  - `alwaysAsk` — always ask AR vs directions;
  - `preferAR` — open AR unless impossible.

**Implemented so far:**
- `MuseumDetailView` now branches on location state before presenting AR.
- Unknown / denied / nearby / too-far states show a prompt before AR.
- Only `inside` opens `ARMuseumView` directly.
- Walking directions open Apple Maps.
- Map view can be opened from the prompt.
- `radiusMeters` is now treated as discovery radius; AR activation is capped at ~50m.

**Definition of done:**
- Tapping **Open AR / Directions** while too far shows a prompt first, not camera AR.
- Tapping **Open Walking Directions** opens Apple Maps.
- Tapping **View on Map** switches to the Map tab or opens map view for that space.
- Tapping **Try AR Anyway** still allows the current AR guidance flow.

### Batch B: Improve Map tab polish

**Build:**
- Show walking time/distance on selected marker card.
- Add “Start walking” / “End” active mode on the map card.
- Add a local Downtown Memory Loop tour flow with step numbers and visited state.
- Add marker legend.

### Batch C: Nearby AR quality pass

**Build:**
- Make HUD state clearer.
- Add an AR readiness checklist.
- Add fallback copy when geo tracking is unavailable.
- Test on physical device near at least one seeded Downers Grove POI.

---

## Current conclusion

The app has enough building blocks now, but the main product flow needs clearer decision routing. The next code change should not be “more AR”; it should be the **Open AR / Directions decision layer** that decides between directions, map, permission prompt, and AR before presenting the AR screen.
