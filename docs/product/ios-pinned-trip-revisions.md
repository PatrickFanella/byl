# iOS trips and reviewed revisions

Issue #43 pins a new iOS trip to the immutable reviewed revision selected at
start. The app fetches that exact revision, checks its tour and stop identities,
selected mode, usable path projection, geometry, retained stories, and reviewed
arrival access, then saves the snapshot and active record before opening the trip.
A storage failure prevents a new trip from being recorded as started.

Older tour responses still decode. They do not offer a new reviewed start when
revision or readiness information is absent. An older saved active trip can
continue from its cached stops and route with a compatibility notice. It does not
fetch current geometry or sync position-based progress into a newer itinerary.
End that trip to choose a reviewed revision.

## Snapshot, cache, and progress

Revision snapshots contain the selected tour, ordered stops, complete stories,
media/narration references, and per-mode geometry and journey information.
Active map geometry, arrival detection, navigation handoff, story reading, and
audio planning use that snapshot. Featured stories do not fetch the current
catalogue, and the arrival sheet does not join newer stories at the same place.
Navigation uses the selected mode's approved entrance or viewpoint, with no site
centroid fallback for a pinned trip.

Snapshots and progress are partitioned by API-origin hash, account, and revision.
They exclude backups and use iOS file protection until first authentication.
The active record identifies the revision and server. Restore requires that exact
snapshot for the same account and origin; a missing or unreadable snapshot cannot
fall back to a newer tour cache. No session token or recorded GPS trace is stored
in these records.

Progress stores stable stop IDs within one revision. The displayed positions are
derived from that revision's stop order. Remote reads must match account, tour,
and revision, and unknown stop IDs are ignored. Writes target the revision-scoped
progress endpoint. Walking and biking share visits within the same revision,
matching the server contract. An offline reset is retained and sent before later
visits. Queued responses cannot mark another reset generation as synchronized.

Loading a pinned route rechecks the exact revision's visibility while retaining
its started content. Network failures keep the saved revision. A known 403/404
removes its cached snapshot and progress, disables arrivals, and shows an
unavailable state. The app stops narration when that state reaches the view or
foreground check. Offline caches cannot discover a server revocation until a
request reaches the server.

## Reviewing an update

The Trip panel identifies the started revision and offers an explicit update
check. A new revision is held separately, with its stop order, path readiness,
access notices, and estimates available for review. The existing trip is not
replaced when this metadata arrives. An unavailable selected mode disables the
transition.

Switching requires confirmation and another server visibility/readiness check.
The transition also verifies that the active trip, account, credential, origin,
and selected candidate still match. Visits carry forward only when the stable
stop ID, place, approved arrival, and featured story identity are unchanged.
Reordering preserves those visits by ID; changed entrances and new stops remain
unvisited. Any legitimate progress already saved for the target revision remains
scoped there. The previous revision's progress is retained separately.

## Public trip information

Tour detail and the active Trip panel display the selected mode's computation,
quality/readiness, eligible travel modes, connector and other segment kinds,
surface, access status, and public reference metadata. The summary separates
travel, stop dwell, story allowance, itinerary total, and planned return. A
missing return has no invented round-trip total. Estimate assumptions are shown.

Approved corridor reference notes and access notes are retained and filtered to
the selected mode and segments. Stop notes and approved arrival access/surface
notes are displayed with the trip. These are the started revision's guidance;
an independently refreshed urgent-notice feed is outside this change.

## Verification and limits

On Soyuz, all 112 Swift package tests pass, including 17 new revision tests.
They cover shared fixture decoding and reordered stop IDs, older response
compatibility, cache-only legacy resume, retained geometry/stories, offline
restore despite a newer tour cache, missing-cache refusal, durable-start failure,
account/origin isolation, exact progress endpoint/body, owner/revision validation,
offline reset recovery, explicit adoption and reorder reconciliation, changed entrances, unavailable
modes, known revocation, reviewed arrival coordinates, and mode-specific notices.
An unsigned iOS app build passes on the same source.

The shared captures under `backend/api/fixtures/tour-revisions-v1` are copied
byte-for-byte into package resources. Their original straight-line routes are
not qualified public paths. Tests explicitly inject synthetic review and arrival
assertions to exercise client eligibility and transitions; these assertions are
not source, access, provider, or field evidence.

No native UI walkthrough is claimed. T3 device opening failed with an active-run
ownership error and returned no bound device input session. Simulator/device
process termination, update confirmation, keyboard, large-text, map rendering,
and provider sign-in remain separate checks. Model recreation tests establish
disk restore behavior only.

The snapshot retains media and narration references but does not download those
files or map tiles for offline use. Story text and cached geometry can resume
without those assets. This change adds no database migration, transit connector,
telemetry collection, runtime deployment, or field-route qualification.
