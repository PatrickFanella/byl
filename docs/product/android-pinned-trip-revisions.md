# Android trips and reviewed revisions

Issue #44 pins new Android trips to an immutable reviewed revision. Starting a
trip fetches that exact revision, checks the tour/stop identities, selected mode,
usable path projection, geometry, retained stories, and reviewed arrival access,
then writes its snapshot and active record before opening the trip. A failed
snapshot write prevents the start.

Older responses still decode. A new start requires revision/readiness data and
the selected mode's approved arrivals. An older saved active trip can resume its
cached stops and geometry with a compatibility notice; it keeps progress local
and does not join current geometry or sync position-based visits into a newer
itinerary. End that trip to choose a reviewed revision.

## Retained content and progress

Snapshots retain ordered stops, complete selected stories, media/narration
references, and per-mode geometry and journey information. Active map pins,
arrival detection, and direction handoff use the reviewed entrance or viewpoint
for that mode. A pinned trip never substitutes a site's centroid for an unknown
arrival. The trip story route and narration plan use retained story content;
the arrival sheet does not join newer catalogue stories at the place.

Snapshot and progress keys include an API-origin hash, account, and revision.
Files use the app's private storage; Android backup remains disabled. The active
record names the revision and server. Restore requires that exact snapshot for
the same account and origin. A missing pinned snapshot cannot fall back to a
newer tour cache. These records contain neither session tokens nor GPS traces.

Visits are stored by stable stop ID within the revision; positions shown by the
UI come from its retained order. Remote progress must match account, tour, and
revision, and unknown IDs are ignored. Requests use the revision progress API.
Walking and biking share visits within one revision, matching the server.
Offline resets are saved and sent before later visits. An older in-flight write
cannot mark a newer reset generation as synchronized. A new trip waits for the
previous trip's queued writes to finish.

Trip APIs retain the opening credential and origin. Account/token/origin changes
cannot populate the old flow with new responses or record visits through it.
The walk screen shows a session boundary instead of continuing those controls.
An old origin's 401 cannot clear the current origin's session.

The exact revision is rechecked on opening/foreground. Network failure keeps the
saved itinerary. A known 403/404 clears its pinned cache/progress, disables
arrivals, and shows an unavailable state. Narration and location updates stop
when that state reaches the app. Offline clients cannot discover revocation
until a request reaches the server.

## Deliberate updates and trip information

The Trip panel identifies the started revision and offers an update check. A
new release is held separately for review, including its title, stop order,
readiness, access guidance, and estimates. Receiving it never replaces the
started itinerary. A failed selected mode disables adoption.

Switching requires confirmation and another server visibility/readiness check.
The model also checks that the account/session and selected candidate remain
current. Visits carry only when stop ID, place, approved arrival, and featured
story identity are unchanged. Reordered stops retain visits by ID; changed
entrances and new stops remain unvisited. Existing progress for the target
revision stays scoped there, and the old revision's progress is kept separately.

Tour detail and Trip show computation/quality/readiness, eligible modes,
connector and other segment kinds, surface/access status, public reference
metadata, and retained corridor/arrival/stop notices. Corridor access notes are
filtered to the selected mode and segments. Estimates separate travel, dwell,
story allowance, itinerary total, and a planned return; an unplanned return has
no invented round-trip total. The API's estimate assumptions are displayed.
An independently refreshed urgent-notice feed is outside this change.

The UI uses the existing Byl palette and type styles, scrolling trip review,
wrapping text, a confirmation dialog, and explicit failure messages. A failed
local trip-end removal keeps both the active state and walk screen open so the
error can be read and retried.

## Evidence and limits

All 108 Kotlin core tests pass, including 19 new revision tests. The Android debug
build and lint pass with 0 errors and 39 existing warnings, with no lint finding
pointing to the new revision model or trip-summary file. Tests use the actual
file store and cover shared fixture compatibility/reorder, legacy responses and
cache-only resume, retained geometry/stories, offline disk restore despite a
newer tour cache, missing-cache refusal, failed durable start, account/origin
partitioning, exact progress path/body, owner/revision validation, explicit
adoption, reordered visits, changed entrances, unavailable modes, known
revocation, reviewed navigation coordinates, offline reset recovery, selected
notices, late account responses, and a reset during an in-flight PUT.

The three resources copied from `backend/api/fixtures/tour-revisions-v1` are
checked byte-for-byte. The captures contain synthetic straight-line routes, not
qualified public paths. Tests inject explicitly synthetic review and arrival
assertions for client behavior; those assertions are not source, public-access,
provider, or field evidence.

No emulator/device walkthrough is claimed. T3 device opening returned an
active-run ownership error without a bound device input session. Model recreation
over disk proves cache restore behavior, not Android process-death/navigation,
map rendering, update confirmation, large text, accessibility, or provider
sign-in. Complete that walkthrough when the device boundary is usable.

Story text and saved geometry can resume offline, but media/audio files and map
tiles are not downloaded by this change. This adds no database migration,
transit connector, telemetry, runtime deployment, or field-route qualification.
