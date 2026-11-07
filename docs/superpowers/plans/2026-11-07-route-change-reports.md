# Versioned route-change reports — issue 22

One PR adds private operational reports, using the existing brief disposition
states. Historical intake, public reviews, photo uploads, trip recording and
automatic route edits remain separate work.

- Store an immutable submission tied to a tour revision and mode. Identify a
  stable stop, its entrance, or adjacent stop pair; retain direction, reason,
  optional observation date, duration and what an improvement would improve.
- Accept a minimal blocked-entrance report without sources or an alternative.
  Allow one affected-location map point and a bounded optional alternative line.
  Validate annotations against the affected frozen route, not the current draft.
  Geometry is an unverified suggestion, never proof of access or a new junction.
- Add authenticated owner create/list/get APIs and curator list/get/disposition
  APIs. Derive ownership from the session, enforce SQL ownership restrictions,
  no-store responses, expected disposition versions, idempotent retries and
  account quotas. Keep submitted input fixed after review starts.
- Add an additive migration, OpenAPI, client fixtures and intake/operator notes.
  The ordinary new-tour intake stays closed. These operational reports require
  sign-in; this change configures no identity provider.

Verify minimal input, alternatives, unknown duration, revision/stop/mode/direction
references, nearby-only annotations, private ownership, curator gates, concurrent
retry deduplication, quotas and stale dispositions with real PostgreSQL. Require
all five hosted jobs and a clean merge. Rehearse production migration separately
against a fresh restored backup before operator admission; CI applies no SQL.
