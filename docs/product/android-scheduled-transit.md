# Android scheduled CTA connections

Issue #56 adds Compose presentation and explicit plan retention for one reviewed
CTA Blue Line connection between adjacent story stops. Public use remains under
the backend scheduled-transit gate; this implementation does not activate it.
Live service, street access and field readiness require separate qualification.

The client advertises `scheduled-transit-v1` on itinerary discovery, detail,
route, revision and revision-progress requests. Ordinary walking/biking revisions
retain their existing flow. Mixed starts and explicit revision adoption require
confirmed admission. Unknown or malformed transit data cannot fall through to an
ordinary walking start; an ordinary mode outside the connection stays separate.

## Choosing and retaining a departure

The detail page identifies boarding/alighting platforms and curator walking
assumptions. Opening the mixed trip pins the reviewed revision and stable story
stop identities; it never picks a train. The active trip's **Scheduled CTA
connection** control opens a scrollable plan sheet.

Choose when you expect to leave the connector's first story stop after its
stories. Native date/time pickers use the device timezone; displayed timetable
results explicitly use Chicago time. Earlier walking, narration and dwell are
outside this query. Planning sends only version and the chosen departure instant,
with normal account authorization when applicable. It sends no visitor fix.

Requesting produces a proposal. Explicit acceptance writes and reads back a
private device record before adoption. Records are bounded to 256 KiB on write
and isolated by API origin, account/anonymous scope, tour and revision. The app's
existing backup exclusion applies. Write failure leaves the proposal unadopted.
A corrupt record exposes an explicit removal action even when no plan decodes.
Removal leaves story-stop progress alone. Plans do not sync to an account cloud.

Reopening restores the same choice without another planning POST. Refreshing
availability never changes the selected train. Ending, session loss and revision
changes invalidate pending work. A different revision needs its own explicit
choice; the old revision's retained choice stays separate.

## Presentation and guidance

Walking, walking to the platform, boarding buffer/wait, scheduled rail and walking
from the platform remain ordered, distinct sections. The sheet labels duration
bases, platform identities, requested departure context, Chicago schedule times,
source retrieval time and curator assumptions. Typed validation compares these
with the frozen revision, including ordinary walking legs. Changed revisions,
platform drift, live-labelled results, altered geometry and reordered segments
cannot become an accepted plan.

The map draws ordinary retained walking legs only. It never fabricates a rail
walking line. Generic next-stop directions are suppressed across the connector
interval. Separate boarding and post-alighting walking handoffs require explicit
confirmation. Links use public story/platform references, with no device fix,
account identifier or departure time. The confirmation explains that the maps app
takes over and timetable platform points do not certify public street entrances
or accessibility. Missing maps handlers produce a visible error.

The schedule is not a live arrival prediction. Byl does not detect boarding,
alighting, train movement or whether service ran. A source older than 24 hours,
more than five minutes in the future, or a passed boarding time suspends
directions. No-service, stale, provider-unavailable, unsupported and review states
remain visible without usable guidance. Review/unsupported results persist a
guidance suspension without replacing the chosen plan; clearing it requires a
fresh valid proposal and explicit acceptance.

Admission lasts at most 60 seconds. Checks run while the sheet is visible, on
foreground and before planning. Offline or disabled admission suspends guidance.
An authoritative revision 403/404 revokes the active saved itinerary under the
existing access policy. If a review suspension cannot be saved, the controller
attempts to remove the saved choice and reports the storage failure.

Progress stays keyed by stable story-stop IDs through walking and rail sections.
Timetables never imply a story visit or completed rail trip. Explicit revision
adoption carries only visits to unchanged stops under the existing comparison.

## Verification and limits

Kotlin fixtures cover actual Go-handler wire decoding, ordered segments,
no-service/stale/missed states, explicit acceptance, reopen without replanning,
storage failure/readback, identity isolation, admission expiry/refusal, persistent
review suspension, corrupt records, late replies, directions payloads, mixed
progress, authoritative removal and revision adoption. Existing walking/biking
and privacy tests remain in the full core suite. Debug assembly and Android lint
check app integration.

The `backend-transit-{revision,plan}.json` fixtures are byte-identical to #54's
actual public-handler exports, using a synthetic timetable and source hash.
Transit/walking fields are unchanged; story bodies were removed from the moments
map. Original arrival points are absent. Active-trip tests explicitly inject
synthetic arrival approvals and remove featured summaries. Those variations do
not prove public source quality or field access.

A native UI walkthrough remains unverified in this run. T3 inventory finds an
Android simulator, but opening it returns an active-run ownership error. No
alternate native UI driver was used. Phone layout, appearance/accessibility,
actual CTA service and street entrances still need qualification before public
activation. Web and iOS implementation/limits are documented separately; a source
merge does not establish deployment or field readiness.
