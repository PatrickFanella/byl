# Web tour suggestions

Issue #39 adds `/routes/suggestions` for signed-in visitors to suggest a curated
public tour and follow their own private submission status. The existing personal
route builder at `/routes/new` remains a separate flow for choosing known places.

## Intake gate and submission

`PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED` defaults to false. Set it explicitly on the API
and restart that process to open visitor creation. The web client reads
`GET /api/v1/route-brief-intake`; a missing, failed or disabled capability does not
show a new-submission form. There is no client build flag that can override the
server. The production Compose environment is not changed by this PR.

With the gate open, signed-in visitors can submit only a topic and area. Optional
notes, unresolved place names, event questions and source links are research leads.
No local record IDs, verified citations or selected stops are required. The form
uses the existing brief contract, validation, per-owner retry keys and quotas:
10 new briefs per 24 hours and 20 unresolved briefs per account.

`POST /api/v1/me/route-briefs` captures the authenticated owner and creates a private
draft for curator triage. It does not fetch links, queue research, approve claims,
create a personal route, or publish a public tour. Curators start the existing
bounded research workflow and use the reviewed publication package separately.
Visitor calls to curator state/research/publication endpoints remain forbidden.
Visitor edits to an already submitted brief remain unavailable.

## Drafts and retries

The form retains unfinished input in `sessionStorage` under an account-specific
key. This is a draft in that browser tab, subject to browser storage retention;
it is not a cross-device draft. Other signed-in accounts do not see it through the
UI. When storage is unavailable, input remains on the current page only.

Before a request is sent, the client stores the exact body and a generated retry
key together. An uncertain response locks those fields and offers **Retry same
suggestion**, including after a reload. A successful retry reads the existing
record rather than creating another copy. Validation errors preserve editable
input, identify the fields and move keyboard focus to the first error. A closed
gate or expired sign-in keeps the original retry identity for later recovery.

Users can inspect their submission list before retrying or starting another idea.
**Discard local draft** clears only that local attempt; it cannot delete a
suggestion the server already saved. Successful submission clears the local draft
and opens the owner status view. An account change or component teardown prevents
an old response from navigating the new account's page.

## Private status

`GET /api/v1/me/route-briefs` lists only the authenticated owner's records. The web
list displays 20 at a time with one lookahead record for pagination. Detail reads
use `GET /api/v1/me/route-briefs/{id}`; another owner's ID returns 404. Successful
and handler-error responses use `no-store` and `Vary: Authorization`. Signing out
removes the submission UI, and signing in with another account mounts a separate
list, status loader and local draft.

The current brief state is displayed as Received, Research in progress, More
research needed, Curator review, Accepted or Declined. The detail page shows the
curator's reason and the currently saved research leads; it does not invent a
history of earlier states. Curators may refine the saved brief during research.
Accepted is a brief disposition, not client authority to publish its prose. Only
the separate curator publication workflow can release a reviewed public tour.

Existing submissions remain readable when intake closes. Status refresh and
personal route planning work independently of that gate. No user route is
converted into a curated public tour by submission or status changes.

## Verification and rollout

Backend coverage exercises default-off and explicit-on configuration, real closed
and open API gates, minimal/optional input, validation and unknown-field rejection,
idempotent retries, owner quotas, owner-only status, curator decisions and denial
of visitor curator actions. It also checks that submitting leaves tour, job and
candidate counts unchanged.

Playwright covers signed-out boundaries, minimal creation, optional leads and
focus/error descriptions, lost-response recovery across reload, every lifecycle
state, foreign-account denial, account-specific drafts, unavailable capability,
closed intake, and retry retention after closure or expired sign-in. The default
isolated fixture opens intake to exercise the complete flow. A separate local run
sets `PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED=false` and runs the closed-intake scenario
against the actual rejecting API. The ordinary open-fixture closed-intake browser
scenario substitutes only the capability response; backend disabled-gate tests
and the separate closed run establish server enforcement.

The feature uses schema 16 and adds no migration. Its branch depends on atomic
publication PR #88, which must merge before this PR is retargeted to main. The
gate remains off in production until the owner deliberately enables intake with
working sign-in and curator capacity. This PR does not configure identity
providers, enable production submissions, deploy the app, or qualify real route
directions and field behavior.
