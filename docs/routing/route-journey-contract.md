# Route readiness and trip estimates

Tour detail responses add `journeys`, keyed by every requested mode. Full route
responses add `journey`. Both use the version-1 `RouteJourney` contract in
`backend/api/openapi.yaml`. These are read projections, including for existing
immutable revisions; stored snapshots, geometry and routing fingerprints do not
change. Retained revisions use their own frozen stops, stories, geometry and
approved corridor, without joining newer working content.

## Readiness

`computation` retains pending/ready/failed, or missing when no mode result exists.
`state` distinguishes missing, pending, failed, unreviewed, needs_research,
blocked, stale, unavailable and usable. `usable` requires selected geometry and a
current passing quality checkpoint. A quality pass means eligible for curator
review, not publication approval, a current access guarantee or field timing.
A ready legacy route with no checkpoint is unreviewed. A partial mode response
can contain a usable walk and a failed bike result; clients must not substitute
one mode's path or estimates for the other.

Existing `routes` summaries keep their status, distance, duration and
`estimated_minutes` fields. Ready still means computed in that older contract;
estimated minutes remain travel-only, rounded up. An absent legacy mode stays
absent, and the existing full-route endpoint still returns 404 for a missing
result. The additive tour `journeys` map represents that mode explicitly as
missing. Older APIs omit the new fields; clients must allow their absence.

## Time and return accounting

`travel_distance_m` and `travel_duration_s` come from the selected provider result
and are available only with ready selected geometry. A blocked or unreviewed
computed result can retain those metrics for inspection while `usable` is false.
Pending/failed/missing results have null travel and itinerary totals, even if an
older numeric field still contains a prior value. Reference lines never supply
substitute travel estimates.

The provisional stop assumptions are explicit: 180 seconds of dwell per story
stop, plus 120 seconds of narration for each featured story. These are planning
assumptions, not measured audio durations or a brief's promised time budget.
`itinerary_duration_s` adds those assumptions to computed travel. It includes an
explicit closing return leg once and adds no extra story stop for return travel.
`return_plan` is included only for a final marked leg from the last story position
to the first; its metrics are a subset of total travel, not another addition.
`round_trip_duration_s` then equals the itinerary total.

Without that explicit computed return, `return_plan.status` is unplanned and
return metrics and round-trip total are null. The itinerary total excludes an
unplanned return. Visitors still need an onward/return arrangement before treating
an open itinerary as a complete round trip. No straight-line, reversed one-way
path or transit timetable is invented as a return plan.

The synthetic shared fixture has 1,704 seconds of computed travel, four stops,
720 seconds of dwell and 480 seconds of narration. Its total is 2,904 seconds
(49 minutes rounded up), with 300 seconds of return already inside travel. Its
legacy travel estimate remains 29 minutes. These are contract fixture values,
not measurements from a real trail.

## Segment provenance and privacy

Segments retain ordered approved reference traversals, including repeated and
reverse occurrences. Each states `basis: approved_reference`, trail versus road
connector, permitted modes, per-mode access assertion, optional surface and
public reference URL/title/publisher/version/retrieval/license metadata. A road
connector uses its own source context. An unknown surface is null. The optional
surface input is bounded to 80 characters and follows the existing explicit
corridor proposal/review path; it creates no new approval authority.

Reference segments are not an inferred provider-edge mapping. This contract
supplies no invented per-segment computed distance/time or current field condition.
The full route retains its actual computed story legs separately. Private
inspection IDs, quoted passages, provider edge attribution, review authors/notes
and candidate payloads are excluded from the journey projection. Issue codes
carry bounded outcomes without copying private finding details.

## Verification and clients

Canonical synthetic responses are under `backend/api/fixtures/route-journeys-v1`:
computed return, unreviewed, failed, pending and a tour with partial mode results.
The native fixture copies must match those bytes. Backend projection tests,
PostgreSQL HTTP tests and web/Swift/Kotlin decoder tests cover new responses and
legacy field absence. Native caches round-trip the new optional metadata.
Web keeps `estimateText` travel-only and adds `tripEstimateText` for the stated
itinerary total, with an explicit unplanned-return label. Client interface
redesign, active-trip revision adoption and transit presentation are separate
issues.

No migration, production fixture, provider activation or publication-gate change
is required. Recovery retains schema 16 and immutable history; use a forward fix
or read-only prior-image recovery with older workers stopped. Runtime delivery,
authenticated production curator use, device and field qualification are separate
from local and hosted checks.
