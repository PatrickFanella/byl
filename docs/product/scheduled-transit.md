# Scheduled CTA connectors

Issue #35 adds one curator-selected Blue Line ride between adjacent story stops.
The backend represents ordinary walking, walking to a platform, boarding wait,
scheduled rail travel and walking from a platform as separate sections. Public
use defaults off. Web/iOS/Android presentation and retained trip plans remain
issues #54–#56; this change does not activate the capability in production.

## Provider and scope

The source is CTA's [official static GTFS feed](https://www.transitchicago.com/developers/gtfs/),
downloaded only from its [official archive endpoint](https://www.transitchicago.com/downloads/sch_data/google_transit.zip).
CTA documents this as scheduled service data and notes that short/unexpected
reroutes may be absent. A timetable result therefore includes source identity,
retrieval time and a scheduled label, with a separate instruction to check
[current Blue Line service information](https://www.transitchicago.com/blueline/).
It is not a live departure prediction or disruption check.

The first implementation supports one direct Blue Line ride with explicitly
selected platform IDs. It does not infer a station from proximity. Both platforms
must actually occur in the retained Blue Line timetable, in the correct order on
the same trip. Frequency-only service cannot become an exact departure. Pickup
and drop-off restrictions are respected. Transfers return unsupported. The
initial connection mode is walking; bicycles are unsupported rather than
implicitly allowed. CTA's [bike rules](https://www.transitchicago.com/bikeandride/)
include time, bike-type and operational restrictions, so a generic rail-mode flag
would not establish eligibility.

Use of the data is governed by CTA's
[Developer License Agreement and Terms of Use](https://www.transitchicago.com/developers/terms/).
The archive's SHA-256 identifies its exact bytes. It is not a visitor identifier.
The provider uses `America/Chicago`, including the
[GTFS service-day/time conventions](https://gtfs.org/documentation/schedule/reference/).
Calendar exceptions override weekday service. Times beyond 24:00 use the previous
service date where appropriate; clock seconds start at local noon minus twelve
elapsed hours. The API returns RFC3339 instants and the agency timezone.

The client fetches and parses only when configured and needed. It retains one
in-process timetable for at most 24 hours, coalesces concurrent downloads and
backs off failed refreshes for a minute. A failed refresh never reuses stale
departures as current service. The fixed-host HTTPS/redirect restriction also
applies to injected HTTP clients. Requests download the same public feed; visitor
coordinates, account identifiers and requested departure times are not sent to
CTA. Planning creates no database trip or visitor tracking record.

Feed limits are 80 MiB compressed, 500 MiB declared expansion, 400 MiB per parsed
table, six million rows per table and 4 KiB per field. Retained data is bounded to
40,000 stops, 10,000 services, 10,000 Blue Line trips and 100 stops per selected
trip. Other routes' stop times are streamed past, not retained. Oversized,
malformed or unsupported feeds fail closed. The release binary embeds timezone
data, so the Alpine image does not depend on a host timezone installation.

## Curator selection and publication

Configure `TRANSIT_PROVIDER=cta` for private verification and previews. Leave
`SCHEDULED_TRANSIT_ENABLED` false while clients are unsupported. Neither ordinary
browsing nor API startup downloads the archive.

A curator uses `PUT /api/v1/admin/tours/{id}/transit` with the expected
`draft_version`, one version-1 intent and a private review note. The intent names
adjacent story stops, the boarding/alighting platform IDs, explicit walking-time
assumptions and a boarding buffer. The backend verifies the platforms against a
fresh source and stores the public platform/source metadata. A source can prove
platform membership without a usable departure at that particular time; missing
service remains visible in a preview. Platform coordinates are timetable points,
not reviewed street entrances or an accessibility certificate.

Each approved choice is append-only. A new choice or explicit removal increments
the tour draft; a stale expected version conflicts. Removal works while public
transit is disabled and with the provider unconfigured. A selection does not
publish a tour. Ordinary routing workers cannot automatically release a transit
selection or its pending removal through legacy save/share behavior. Once an
explicit release restores an ordinary walking revision, its existing publication
behavior resumes.

Existing walking, public-arrival and path-quality publication checks still apply.
This bounded connection does not relax the underlying walking qualification.
Publishing freezes the selected platforms, assumptions, source proof and choice
version in the immutable tour revision. The snapshot retains ordinary qualified
walking legs separately and omits the transit-replaced leg. It removes the
continuous `routes.walk` entry, so a legacy reader cannot mistake rail travel for
a walking line. Other modes and older revision rows are unchanged.

## Departure context and client contract

`GET /api/v1/itinerary-capabilities` reports public admission. Public mixed-tour
requests additionally require `X-Byl-Capabilities: scheduled-transit-v1`.
Discovery filters mixed tours for unsupported/disabled clients; direct mixed
detail, revision, route and progress requests return HTTP 403
`unsupported_itinerary`. This authoritative refusal stops existing clients from
falling back to an older unpinned cached walking tour.
An older active trip pinned to an ordinary revision can still read that revision
and its walking geometry. A capable client must request a transit plan instead
of a continuous walking route. Responses are `no-store` and vary on Authorization
and the capability header.

Account saved/progress lists and revision metadata remain readable without the
capability. They contain no walking geometry or walk summary for a mixed
revision. Legacy reset checks the actual new target and fences the selected
revision inside the progress transaction; it cannot move an unsupported client
from an old ordinary pin into a mixed release. Explicit reset back to a newly
released ordinary revision remains possible after a client downgrade.

`POST /api/v1/tours/{id}/revisions/{revision}/transit-plan` accepts version 1 and a
future `departure_at` within seven days. This means **leaving the selected
connector's first story stop**, not starting the whole tour. Earlier stories,
dwell and narration are outside this timetable query. The planner adds the
curator's walking estimate and boarding buffer before selecting the first
supported departure within six hours. A ride is bounded to three hours. The
provider's boarding horizon is seven days; an edge-of-window walking allowance
can therefore make a requested plan unavailable.

The response orders ordinary retained walking sections around four explicit
connector sections: `walk_to_stop`, `boarding_wait`, `transit`, and
`walk_from_stop`. Each duration states its basis: qualified walking geometry,
curator estimate, boarding/schedule wait or static rail timetable. Station
connectors have no fabricated walking geometry and require an explicit external
directions handoff. Clients must explain that handoff before transferring control.
Missing ordinary legs, changed platform identity/name/location, stale source,
missing or missed service and provider failure cannot produce a usable mixed
itinerary. Unavailable/review responses have no itinerary segments.

A working preview uses `POST /api/v1/admin/tours/{id}/transit-plan` with the current
draft version. It identifies that draft and returns `revision_id: null`. A public
plan identifies its immutable revision and connector version. Later working
choices do not alter a retained plan's selection. Timetables may change between
explicit planning requests: the result includes the exact feed hash and planned
times. Clients must privately retain the chosen plan with their active revision
and explicitly request/review a replacement. Reopening an active trip must not
silently select a new train, revise progress or adopt a new tour revision.

## Recorded Chicago–Forest Park candidate

On November 8, 2026 at 12:31:57 UTC, the official archive had SHA-256
`a9372be5b70fcee4ed98c9476433f86065637a9a500b8af6afbb84d000ef2746`.
Its calendar spans September 11–November 30. The full archive parser retained
896 Blue Line trips, 11,138 source stops and 127 service calendars. Its Sunday
November 8 trip `92281538010`, service `109209`, lists Clark/Lake (Subway)
platform `30374` departing at **10:09:30 Chicago time** and Forest Park platform
`30077` arriving at **10:53:00** (`16:09:30Z`–`16:53:00Z`).

This is a checked **dated scheduled candidate** from the retained archive, not a
claim that the train ran, a current live recommendation, a checked station walk,
or historical narrative evidence. Tiny recorded table subsets and their source
provenance are in `backend/internal/transit/testdata/cta-blue`. Added/invalid
calendar, pickup and clock cases are explicit test mutations. The full archive
is not committed. Its candidate was checked against the full archive separately
from the small deterministic provider fixtures.

## Verification and rollout

Provider race tests cover recorded service, no service, missed departure, wrong
direction/line, source staleness, calendar exceptions, restricted pickup,
frequency-only service, times beyond midnight, malformed feeds, unavailable
provider, redirect restrictions and concurrent fetches. PostgreSQL tests cover
version fences, immutable choice history/removal, snapshot projection, old
revision preservation, suppression of automatic release and 19→20 migration
fingerprints. API tests cover curator authorization, strict planning input,
private draft previews, default-off discovery/access, explicit mixed segments,
old active pins/progress, later unpublished choices, unavailable service and
changed platforms. Synthetic route/HTTP proof does not establish station access
or field timing.

Before rollout, rehearse exact schema-20 images and the retained-data checks on a
private restored baseline, then admit the matching operator hook/manifest. The
installed production hook remains at schema 16 and rejects unadmitted changes.
Keep public transit off until #54–#56 demonstrate the client contract, persistence,
progress, explicit updates, unavailable states and handoff in browser/native UI.
Qualify the intended station walks and actual source freshness/alert workflow
before relying on the candidate. Existing source/backup custody and deployment
constraints still apply; this PR does not change host settings.

Rollback must retain immutable choices and revisions; migration down is refused.
A revision-aware older API may lack mixed UI, but the stored mixed snapshot still
has no continuous walking route. Do not roll back to a pre-revision API that reads
live walking rows for published trips. Verify the actual rollback image's error,
discovery and old-pin behavior against a clone before admission. Never restore a
walking line over the transit section to make an unsupported client proceed.
