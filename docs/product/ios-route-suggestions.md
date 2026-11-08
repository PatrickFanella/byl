# iOS route suggestions

Issue #40 adds **Suggest a route for curator review** under Your routes. Only a
topic and area are required. Optional notes and place, event, and source leads
accept unresolved names, addresses, dates, and URLs. They remain unverified input.
The personal route builder continues to choose existing places on its own screen.

The flow uses the private brief API and public-intake gate introduced by PR #93,
with atomic curator publication from PR #88. The server defaults
`PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED` to false. A missing, malformed, or unavailable
gate response disables submission. Owners can still read prior submissions when
intake closes. Creating a suggestion does not publish content, start research,
or create or change a personal route.

## Account and draft ownership

Signed-out visitors see a sign-in prompt. Each signed-in screen receives an API
client fixed to the opening credential and API origin. The screen identity also
includes the account, credential, and origin. Every asynchronous result checks
that this identity is still current. List and detail responses must match the
owner; a detail must also match the requested record. An unauthorized response
from an old origin cannot clear the current origin's session.

Unfinished fields and a pending submission are stored in Application Support,
partitioned by account ID and a SHA-256 namespace of the API origin. These files
exclude backups and use iOS file protection until first authentication. Tokens
and downloaded submission status are not stored in the draft. Leaving the screen
clears its private in-memory content while retaining unfinished input. Signing
back into the same account and server restores that input on the device.

The app atomically saves the request body and UUID retry key before sending a
POST. A failed write prevents submission. Uncertain responses, malformed success
responses, and conflicts retain that pair and lock editing. Reopening the flow
replays the same body and key, with deterministic JSON encoding. Concurrent taps
cannot create separate attempts. Validation and quota rejections retain input
and allow editing. A closed gate or expired session retains the original attempt.
Unreadable stored drafts block submission until explicit local discard.

**Discard local draft** requires confirmation and removes only local input and
the retry key. It does not delete a server submission. The confirmation directs
the visitor to check prior submissions before starting another. A confirmed
success clears the local draft and opens its private status. If local cleanup
fails after server success, the original attempt remains safe to retry.

## Status and UI

The list displays 20 submissions with a 21-record lookahead and previous/more
navigation. Opening one fetches fresh detail. Received, Research in progress,
More research needed, Curator review, Accepted, and Declined map to the shared
lifecycle; future values display Status unavailable. Detail shows the curator's
decision or research reason, submitted notes and leads, update time, and version.
Accepted does not give the app publication authority.

The screen uses the existing Byl components, vertical scrolling, and labeled
fields. Optional fields expand for recovered input and validation errors. Form
fields lock while an attempt is pending. Gate errors can be retried independently
of private list reads. Detail and confirmation changes reset the scroll position.

## Evidence and limits

The synthetic contract fixture is
`backend/api/fixtures/route-briefs-v1/brief-response.json`; the iOS test resource
is byte-identical. On Soyuz, all 95 Swift package tests passed, including 14 new
suggestion tests covering DTO defaults and status mapping, request headers,
successful cleanup, exact replay after flow recreation, uncertain responses,
validation, quotas, gate failures, unauthorized responses, account/origin
partitioning, late account responses, owner checks, pagination, simultaneous
taps, and failed persistence. Backup exclusion is read back in the storage test.
An unsigned iOS app build also passed on Soyuz after the package tests.

The local T3 device session could not open: **The calling provider no longer owns
an active thread run.** This Linux host has no iOS simulator, and no device input
session was returned. The native walkthrough of sign-in, keyboard and large-text
layout, backgrounding, process termination, and draft resume remains unverified.
The package tests exercise model recreation over disk storage; they do not prove
device rendering or operating-system lifecycle behavior. Browser testing is a
separate web-client result.

The feature changes no database schema or identity provider, does not enable
production intake, and keeps the existing API address and application identity.
Hosted checks, merge, deployment, provider sign-in, and field-route evidence
remain separate qualification steps.
