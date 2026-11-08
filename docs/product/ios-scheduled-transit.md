# iOS scheduled CTA connections

Issue #55 adds presentation and explicit plan retention for one reviewed CTA
Blue Line connection between adjacent story stops. Public use remains controlled
by the backend scheduled-transit gate. This source change does not enable it in
production or establish live service or field readiness.

The iOS client advertises `scheduled-transit-v1` on itinerary discovery, detail,
route, revision and revision-progress requests. Ordinary walking and biking
revisions retain their existing navigation. Mixed starts and explicit revision
adoption require confirmed admission. Unknown or malformed transit metadata
cannot fall through to an ordinary walking start.

## Choosing and reopening a plan

The detail page identifies the boarding and alighting platforms and the curator's
walking assumptions. Opening the mixed trip pins its reviewed revision and stop
identities; it does not choose a train. The active trip's **Scheduled CTA
connection** control opens a scrollable plan sheet.

Choose the time you expect to leave the connector's first story stop after its
stories. The picker uses the device timezone; displayed results explicitly use
Chicago time. Earlier walking, narration and story dwell are outside this query.
Requests contain only version and the chosen departure instant, with the normal
account authorization when applicable. No visitor coordinates go into planning.

Requesting produces a proposal. **Accept and retain this plan** first writes and
reads back a private device record, excluded from backups. A write failure leaves
the proposal unadopted. Records are isolated by API origin, account or anonymous
scope, tour and revision, and bounded to 256 KiB on write. A corrupt record requires
explicit removal before another choice. Retention is local; it is not an account
plan-sync feature.

Reopening restores the same plan without posting another planning request.
Opening or refreshing availability never picks another departure. Removing a
choice leaves stable stop progress alone. Ending the trip, losing its session or
adopting another revision invalidates pending proposal work. The old revision's
plan remains separate; a different revision needs its own explicit choice.

## Schedule and directions limits

The sheet distinguishes ordinary reviewed walking, walking to the platform,
boarding buffer/wait, scheduled rail and walking after alighting. It preserves
segment order, duration basis, platform identity, schedule source retrieval time,
requested departure context and curator assumptions. The validator checks these
against the frozen revision, rejects rail geometry, and matches ordinary walking
sections against retained legs. Changed revision/platforms, live-labelled results,
reordered sections or altered walking geometry cannot become an accepted plan.

CTA results are static timetable estimates. Byl does not detect boarding,
alighting, train movement or whether scheduled service ran. A source older than
24 hours, a source more than five minutes in the future, or a passed boarding time
suspends directions. No-service, stale, provider-unavailable, unsupported and
review-required results remain readable without usable guidance.

A current admission check is valid for at most 60 seconds. The plan sheet checks
while visible, on foreground and before planning. Offline or disabled admission
suspends directions. An authoritative 403/404 on revision loading revokes the
active cached itinerary under the existing account-access policy. A review or
unsupported result persists a guidance suspension without replacing the retained
choice. A fresh, valid proposal requires explicit acceptance to clear it. If that
suspension cannot be saved, the client attempts to remove the saved choice and
reports the storage problem.

The map draws only ordinary retained walking legs. No continuous walking route is
synthesized across rail. Generic next-stop Maps handoff is suppressed across the
connector interval. The transit sheet offers separate boarding and post-alighting
walking directions only for an admitted, usable retained plan. Confirmation
explains that Apple Maps takes over and timetable platform points do not certify
public street entrances or accessibility. Links use public story/platform
references, without device location, account identifiers or departure times.

Stop progress continues by stable story-stop ID across all sections. It never
infers a visit or rail journey from the timetable. Explicit revision adoption
carries visits only for unchanged stops under the existing comparison rules.

## Verification and remaining qualification

Swift tests cover actual Go-handler wire decoding, segment validation, explicit
acceptance, reopening without replanning, storage failure, private identity
isolation, gate expiry/offline refusal, persistent review suspension, corrupt
records, late responses, directions URLs, mixed progress, revision adoption and
authoritative removal. The existing walking/biking tests remain part of the full
package suite. The unsigned iPhoneOS build checks SwiftUI integration.

`Fixtures/backend-transit-{revision,plan}.json` are copies of #54's actual Go
public-handler export, using a synthetic timetable provider and source hash.
The export retained all transit and walking contract fields; story bodies were
removed from its moments map. Its original arrival points are absent. Active-trip
tests explicitly add synthetic arrival approvals and remove featured-story
summaries; those variations are not source or field proof. The raw corpus stays
unchanged for the wire-contract check.

Native UI interaction remains unverified in this run. T3 device inventory found
a local Android simulator, but opening it failed with an active-run ownership
error. Soyuz supplied package tests and an unsigned build, but is not an exposed
T3 iOS device host. No alternate native UI driver was used. A phone walkthrough,
accessibility and appearance checks, real CTA timetable/service checks and street
entrance verification remain qualification work before enabling public transit.
Android transit presentation remains issue #56; deployment and production
activation are separate from this implementation.
