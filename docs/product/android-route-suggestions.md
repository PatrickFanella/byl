# Android tour suggestions

Issue #41 adds **Tour suggestions** under Routes. It supports topic/area input,
optional place/event/source leads and owner-only submission status. The personal
route builder remains a separate screen for choosing existing places.

## API and ownership

This branch depends on the public-intake API from PR #93 and atomic publication
from PR #88. The app reads `/api/v1/route-brief-intake` and fails closed when it is
missing, unavailable or disabled. The server's default-off
`PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED` gate still controls every POST. There is no
Android switch that can open server intake.

Signed-out visitors see a sign-in prompt. Each signed-in screen receives an API
client bound to its opening credential, plus a current-account/credential/origin
check. Changing account, session or API origin creates a separate flow. Late
responses cannot populate another account's screen. Unauthorized responses from
an old API origin cannot invalidate the current origin's session.

The DTOs use the existing private brief contract. Only topic and area are required;
unresolved place names and source URLs remain leads. Requests target the owner's
`/me/route-briefs` endpoints and carry no curator state or publication action.
Creating a suggestion does not create a personal route or convert one into a
curated public tour. Server quotas and curator-only research/publication remain
the authority.

## Local drafts and submission recovery

Unfinished form fields and a pending attempt are stored in private app files,
partitioned by account ID and a SHA-256 namespace of the API origin. Tokens are
not included in those files. Android backup remains disabled in the manifest.
The file format supports restoring these fields after navigation, rotation or
process recreation; server status is fetched again rather than stored as an
offline private cache.

Before sending, the app persists the exact request body and a UUID retry key. If
that write fails, it sends no request. A lost or uncertain response keeps the
body/key pair and locks edits until retry or explicit local discard. Recreating
the flow reuses that pair. Two simultaneous submit taps cannot send two distinct
attempts. Validation failures keep the input editable and show field messages;
closed intake and expired sign-in retain the pending identity for later recovery.

The app's file store throws when a replacement rename or removal fails and cleans
up its own temporary write. Draft edits report failed autosave; failed local
discard retains the current form and attempt. The pure JVM store lives in `core`
so hosted tests exercise the same file implementation used by the Android app.
The existing active-trip caller also keeps a trip open with an error if its
resume record cannot be removed, rather than reporting a completed end.

**Discard local draft** removes only that local input and attempt. It does not
delete a suggestion already saved by the server. Successful submission clears
the local draft and opens the returned owner status. The screen scrolls to the
new detail view so its confirmation and status remain visible.

## Status and failure states

The private list uses a 21-record lookahead to display 20 submissions per page.
Selection reads a fresh owner detail. Both list and detail reject mismatched
owners, and older requests cannot replace newer selections. Closing a screen
clears its in-memory private state while retaining its unfinished local draft.

Received, Research in progress, More research needed, Curator review, Accepted
and Declined map to the shared brief lifecycle. Unknown future states display
Status unavailable. Detail shows the curator's reason, current submitted research
leads and source links. Accepted is not authority for the client to publish prose.
Status and personal planning remain available when new intake closes.

Network, gate, authorization, missing-record, quota and validation states have
separate messages. Field errors use the Material text field's error state and
supporting text. Optional fields expand when their errors or a recovered attempt
need attention. The screen uses the existing Byl palette/type system, vertical
scrolling, keyboard insets and wrapping action labels.

## Evidence and limits

The shared synthetic fixture is
`backend/api/fixtures/route-briefs-v1/brief-response.json`. Go validates its model
and input, while Android checks a byte-identical resource copy. Kotlin tests cover
minimal DTO defaults, every status and an unknown future state, closed/unavailable
gates, field errors, quotas, disconnected-response recovery, exact request/key
replay, account/origin partitioning, late account responses, list paging, stale
selection responses, screen closure, simultaneous taps and failed persistence.

Core tests recreate the flow over serialized storage after a disconnected
response. Actual Android rotation and process-death recovery need the separate
device walkthrough below.

Local verification runs `:core:test`, `:app:assembleDebug` and `:app:lintDebug` with
the installed Android SDK. These establish core behavior, app compilation and
lint results, not device rendering or provider sign-in.

All 89 core tests pass, including five tests using actual temporary files. The
regression first reproduced three failures: ignored rename failure, ignored
removal failure, and a suggestion POST after failed persistence. The repaired
store and callers pass the same checks, including refusal to send the POST and
keeping a trip open when its local end cannot be recorded.

The local Android 15 emulator `T3_Pixel_API_35` is available and disk space was
checked. T3's `device_open` returned **The calling provider no longer owns an active
thread run.** No device session or bound input command was returned, so no app
launch, emulator walkthrough, TalkBack, large-text or keyboard rendering result
is claimed. Complete that walkthrough when the device-session boundary is usable.
An iOS simulator is unavailable on this Linux host.

The feature adds no database migration, configures no identity provider and does
not enable production intake or change the existing API address/application ID.
Hosted CI and clean merge are separate from local tests; runtime deployment and
field route qualification remain separate checks.
