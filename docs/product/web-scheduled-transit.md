# Scheduled connections in web trips

Issue #54 adds web presentation for the existing default-off scheduled CTA
capability. It does not change provider configuration or activate production
transit. iOS and Android support remain separate issues #55 and #56.

## Capability and itinerary

The visitor client advertises `scheduled-transit-v1` on discovery, detail,
revision, route and revision-progress requests. The backend still controls
admission. Disabled/unsupported mixed requests remain authoritative refusals;
they cannot fall back to an older cached unpinned walk. Existing ordinary tours
continue using their walking/biking routes and progress. An ordinary bike
alternative, when present in the frozen revision, stays separate from the
walking-only CTA connection; it does not imply permission to carry a bicycle.

The detail page explains the selected boarding/alighting platforms, walking
estimates and buffer. Its map and the active-trip map draw only frozen ordinary
walking legs. The replaced rail interval never becomes a continuous walking
line. While travelling that interval, the generic walking Directions link to
the next story is replaced by the connection review controls. Story arrivals
and stable stop-ID progress continue across the connection. There is no inferred
train boarding, live vehicle tracking or automatic train-progress claim.

## Request, review and retention

A visitor enters the expected time of leaving the connector's first story stop,
not the time of starting the whole tour. Input uses the device timezone; results
are explicitly shown in America/Chicago. Earlier stories and dwell are outside
the selected departure context. The request sends only version and departure
instant to the selected immutable revision's plan endpoint. It sends no GPS fix
or visitor trace to the API or CTA.

Requesting a plan is separate from accepting it. The response must match the
retained revision, connector version, reviewed platform identity/location,
departure context, scheduled/not-live source, ordered segment kinds, walking
references, durations and times. Unknown fields, including raw-location fields,
are rejected before persistence. Ordinary walks, platform walks, boarding wait,
static rail travel and post-platform walks retain their separate bases. Missing
service, stale source, changed platforms and provider failures remain explicit;
an unusable response cannot replace the selected plan.

Acceptance saves and reads back the plan before changing the displayed choice.
The browser key is scoped to API base, account (or anonymous browsing) and tour;
the payload is validated against its immutable revision. A different revision
cannot reuse it. Records are limited to 256 KiB. Storage failure prevents
adoption, and an unreadable record requires explicit removal before replacement.
The Remove control erases this tour's saved timetable without resetting stories
or stop progress. Plans remain local to this browser; no cloud synchronization
or server trip record is created.

Reopening restores the same selected timetable; it never requests another train.
Capability checks refresh independently, without replanning. A timetable source
older than 24 hours, a source dated more than five minutes in the future or a passed scheduled departure
makes the stored plan unavailable for guidance. Its retained information remains
visible with the reason so a visitor can review what they previously chose.
Byl does not infer that they boarded or that a scheduled train actually ran.
A replacement always requires a fresh explicit request, review and acceptance.
An authoritative platform/selection review response also suspends directions
from the older chosen plan across reload, without replacing its retained train.
Switching tour revisions retains separate stop progress and requires a plan for
the newly selected revision; it does not carry the older timetable forward.

## Directions and service context

CTA times are static scheduled service, not live predictions or disruption data.
The interface links to current CTA Blue Line service information. Before opening
walking directions, a focus-trapped dialog explains the transfer of control.
The external app chooses its own route and may use its own location permission;
Byl's progress stays separate. Only public reference endpoints are placed in the
handoff URL, without account, departure or raw visitor-location data. A walk from
the alighting platform includes that platform as the explicit external origin.
Platform coordinates do not certify street entrances or accessibility.

The active screen keeps the timetable in an expandable connection section so
that its next story and arrival controls remain reachable on a narrow screen.
A returned capability check updates the UI clock before enabling controls;
a few milliseconds of network latency cannot leave a valid handoff waiting for
the next periodic clock tick.

## Verification and limitations

Unit tests cover plan identity/kinds, rail geometry rejection, malformed fields,
explicit durable acceptance, same-plan resume, storage failure, stale/missed
plans, account/end/removal races, revision isolation, progress through mixed
sections, capability refusal and the ordinary bike alternative. Playwright cases
cover the mixed detail/active flow, same-plan reload, narrow layout, explicit
handoff/focus restoration, no service, stale source, a mismatched response,
disabled admission, explicit revision/plan updates and storage failure.

The corpus in `web/src/lib/tour/fixtures/backend-transit-*` was emitted by the
real Go public HTTP handlers against an isolated PostgreSQL database with the
existing synthetic transit provider. It checks optional position-derived leg
IDs, fractional walking seconds projected to integers and actual timestamp
formatting; its provenance and original response hashes are recorded beside it.

Timetable, platform and plan responses in these tests are clearly synthetic.
The ordinary browser API fixture remains real for unrelated paths. These checks
do not establish a live CTA departure, inspected station walk, accessibility,
field timing or production rollout. The provider's recorded-source tests and
separate deployment/custody requirements in [the backend contract](scheduled-transit.md)
still apply. Native clients must qualify independently before public activation.
