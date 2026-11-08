# Optional current happenings

Current notices are a separate layer linked to an existing place. They never
become historical moments, move a tour stop, change an approved tour revision or
write trip progress. Occurrences have date-only precision in the place's city
timezone. A source that gives no usable exact time does not become an invented
scheduled departure or appointment.

New import defaults off through `HAPPENINGS_IMPORT_ENABLED=false`. An operator can
enable import after migration admission; a curator then selects retained source evidence. This
feature starts no crawler, social feed or automatic event publication. Existing
bounded source-evidence retrieval accepts relevant organizer/social links as
untrusted source material; an inaccessible announcement stays unavailable.
Direct visitor prose has no import or publication authority.

The source workflow is:

1. Retain an actual inspected announcement using the existing curator evidence
   workflow. Search snippets, catalogue hints, user wording and failed captures
   cannot support publication.
2. Create a private draft with the exact existing place, title, summary,
   organizer, occurrence date range, declared state, next review and expiry.
   Select one to five immutable evidence records and exact captured passage
   citations for the public wording, organizer, venue and dates. A cancellation
   or conflict declaration additionally needs status support.
3. Inspect `GET /api/v1/admin/happenings/{id}/review-package`. It shows the full
   retained read set, latest known source attempts, existing place, previous
   approved snapshot, visitor preview and blocking tasks. Quotation matching
   establishes traceability; the curator must judge what the passages support.
4. Approve or reject that exact package version with an explicit note. Approval
   runs serializably, rechecks its sources/place/calendar date and writes one
   immutable decision and public snapshot atomically. Rejection leaves any
   previous public notice intact. Subsequent changes require an appended draft
   and another review; no worker or successful retrieval approves a notice.

Draft writes use a durably retained printable 16–128-character idempotency key
and the exact body. Exact retries retain their original draft, while changed
input with that key conflicts. Approval retries similarly retain the original
version/action/note result and never move a later public pointer backward. Writes
are strict 32 KiB JSON with no query parameters. Import is limited to 40 drafts
per curator per rolling 24 hours and 100 unexpired, non-rejected drafts or public
notices per place. Expired history does not consume current capacity. Review and
edits of existing notices remain available when new import is off.

An occurrence spans at most 31 days, starts within 180 days and ends no more than
30 days before drafting. The next review date must be in the next 14 city dates;
expiry cannot precede review or fall more than seven days after the occurrence
ends. The review date and expiry are exclusive city-midnight boundaries. Source
checks need to be less than seven elapsed days old. Missing, stale, uncited,
catalogue-only, changed, pending or unavailable proof blocks new approval.

Visitors read `GET /api/v1/places/{place}/happenings` separately from a historical
tour snapshot. Responses use `no-store`, a bounded page size and a decimal
`next_cursor`. Only published places and approved snapshots are returned. Public
responses include dates/timezone, organizer, captured source URLs/titles/
publishers/hashes, last-check time and explicit review/expiry decisions. Private
passages, reviewer identities, notes and complete review packages stay private.
Inspection grants no media-reuse rights; citation license remains unresolved.

The visible clock states are upcoming, active, ended, expired or needs review.
Declared cancellation and conflicting-date notices remain explicit. A newer
failed retrieval marks a scheduled notice source unavailable; a pending attempt
or different content hash makes it need review. An unchanged captured hash can
refresh the check time. None of those checks silently replaces approved wording
or occurrence dates. There is no external retrieval on a visitor request, and
expiry performs no write to the notice snapshot or underlying historical story.

Schema 21 adds separate happening entities, append-only drafts/reviews/public
revisions, current pointers and a source-key/latest-attempt index. Mutation
triggers retain historical decisions. Down migration refuses automatic removal;
image rollback keeps the notice tables and history. Production remains behind
its installed schema-16 hook and manifest until an exact-image baseline/restore
rehearsal and operator admission. Updating the repository hook to 21 does not
install it, migrate production or enable import.

Clock-controlled model tests cover city-date boundaries, active/ended/expired
states, cancellation, source failure and staleness. Disposable PostgreSQL tests
exercise exact retries, review/source/draft fencing, immutable history,
cancellation/rejection, public/private boundaries and unchanged historical rows.
HTTP tests cover default-off import, anonymous/visitor rejection, strict input,
stale approval and paging. Recorded synthetic source fixtures prove engineering
behavior, not that an organizer's real event is current or that a route was
visited. A merged change is not a deployment or field-verification claim.

## Optional client presentation

Web, SwiftUI and Compose place and arrival views start with notices hidden.
`Show current notices` explicitly fetches the public, anonymous endpoint; hiding
it discards the view's data and invalidates any pending response. Refresh and
paging are explicit. An unavailable endpoint, including an older API returning
404, stays inside the notice panel. It does not sign out the visitor or replace
the historical story, stop order, progress, retained revision or narration.
Started native trips fetch notices from the trip's original API origin.

Cards keep the occurrence dates and city timezone, organizer, source link,
captured date, last source check in UTC, review deadline and expiry visible.
`Scheduled for these dates` describes the source's date precision; it does not
claim that an event is live. Cancelled, expired, conflicting, stale and unavailable
notices have explicit labels. While the panel is open, a local minute clock
rechecks the city date and source age without fetching or changing the notice.
Unsafe source URLs are not links. Source citation grants no media-reuse rights.

The canonical response fixture is
`backend/api/fixtures/happenings-v1/responses.json`, copied byte-for-byte to the
three client test resources. A web test enforces equality. Its 17 cases include
synthetic active/future/cancelled/expired/empty responses and named malformed,
unknown-state, source-health and clock fallback cases. They are frozen contract
fixtures, not captured real announcements. Browser tests additionally publish a
synthetic notice through the real review package in an isolated PostgreSQL
database. Package tests and unsigned native builds remain distinct from device
interaction and field qualification.
