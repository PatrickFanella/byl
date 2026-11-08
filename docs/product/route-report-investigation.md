# Private route-report investigation

Issue #33 adds a retained before/after investigation to the curator API. A report
remains an unverified observation, even when several accounts describe it. An
investigation neither accepts that report nor publishes its alternative.

## Workflow

1. Read `GET /api/v1/admin/route-reports/{id}/related`. The query matches the exact
   revision, mode, direction and affected stop/entrance/segment reference. It does
   not infer connectivity from nearby points or combine different directions.
2. Send `POST /api/v1/admin/route-reports/{id}/investigations` with the current
   `version` and up to 19 `related_report_ids`. Use an `Idempotency-Key` for retries.
   Include `research`, shaped like the existing route-brief input, when a new place
   or historical claim needs investigation. Place/event names and submitted prose
   enter that private sourcing workflow as leads, never as verified history.
3. Read the returned `job_id`, `input.brief_id` and `input.evidence_ids`. The queue
   transaction captures every selected report version, creates a draft sourcing
   brief and schedules one existing evidence-retrieval job per distinct safe URL.
   Brief quotas apply: 10 new briefs per 24 hours and 20 unresolved briefs per
   curator, with at most 20 distinct source URLs per investigation. A failed queue
   transaction leaves no partial brief, source job or investigation.
4. Read `GET /api/v1/admin/route-reports/{id}/investigations/{investigation}`. The
   result starts as `null`; the worker retains a complete comparison or explicit
   gaps. The list endpoint uses bounded `limit`/`offset` pagination. All endpoints
   require a curator role and return `Cache-Control: no-store` and
   `Vary: Authorization`. The reporter cannot read other accounts' submissions or
   the private research checkpoint through their own report endpoint.

For example, investigating a blocked crossing without adding historical claims:

```json
{"version": 1, "related_report_ids": []}
```

The response's `input` keeps each observation, its author, submitted values and
captured disposition/version. `duplicate_of` conservatively joins observations
sharing an account, normalized source URL, or matching submitted content. Groups
are transitive and never discard the original submissions. Distinct groups still
do not prove independent witnesses; `independent_corroboration` is always false.

## Computation and evidence

`before` is the reported revision's retained route for the reported mode. The
worker replaces only the affected published legs in a private `after` proposal.
Unchanged legs retain their geometry, costs and story boundaries. A segment's
submitted alternative supplies non-story shaping points between retained anchors;
a proposed entrance annotation supplies a tentative arrival for both adjacent
legs, including a closing return. Related reports' alternatives remain separate
leads. The worker does not blend them into an invented consensus path.

The comparison follows the published direction. Reverse and unknown reported
directions carry a named unresolved question rather than claiming directional
access was checked. It returns full before/after geometry, signed total distance
and time changes, affected stop IDs, frozen arrival eligibility and reviewed
corridor access, retained source snapshots, path-quality findings and unresolved
questions. Distance and time changes are null when no complete alternative exists.

Source lookups use the existing public-only retriever with its redirect, timeout,
size and private-address restrictions. A normalized URL is fetched once per
investigation; sharing it is not corroboration. Unsafe URLs remain original leads
and produce a review issue without a request. An inaccessible operator notice,
unreadable page or missing source also produces a named issue. Readable pages
retain their content checksum, passages, retrieval time and untrusted status;
access to a page grants neither factual approval nor media reuse rights.

The worker retries pending source snapshots within the existing bounded job
attempts, then retains partial research. Later source completion does not rewrite
that checkpoint: inspect the linked evidence record and request a new investigation
with a new retry key when a refreshed comparison is needed. The sourcing brief's
normal inspect/draft/review actions remain available for new claims.

Computation has a two-minute deadline. An unsupported shaping provider, missing
leg, malformed result, displaced endpoint, provider failure or excessive geometry
leaves an explicit unavailable-alternative finding. It does not silently replace
submitted controls with a different route. The existing strict path assessor
checks the combined proposal against retained corridor/network/access references.
An unexpected way, closed traversal, cycling staircase or other prohibited
shortcut remains a blocked finding. A new route may require a new reviewed
corridor/arrival proposal; source pages and a shorter computed path cannot approve
that change. Report controls are always labeled unreviewed.

Each investigation has immutable input and a first-write-only result, protected
by a database trigger. Job completion checks the current claim and attempt; a
reclaimed worker cannot overwrite a newer checkpoint. Job payloads and completion
notes contain IDs and safe codes, not submitted prose, URLs or provider responses.
No investigation writes tour stops, routes, moments, narration, arrivals, corridor
approvals, report dispositions or published pointers.

## Validation and rollout

The synthetic worker fixtures cover a better crossing, missing entrance, cycling
steps, unavailable operator notice, incomplete research and routing failure. They
also check unchanged legs, retained stories/narration, no auto-publication, restart
replay, source deduplication and new claims entering the sourcing brief. Repository
and HTTP coverage checks duplicate groups, exact reference matching, version/key
conflicts, atomic queueing, stale claims, private access and cache headers.

Migration 17 adds only the investigation table, two indexes and checkpoint-retention
trigger. A schema-16 forward-migration test compares hashes of every existing row
and the routing-input hash, and checks that automatic down migration is refused.
Application rollback retains the new private data. Older binaries can ignore the
new table, but must not consume an investigation job they do not support. Stop
new investigation intake and finish its jobs before rolling workers back to an
older binary; retain the checkpoints and schema rather than running migration down.

Production migration admission remains a separate gate. The installed Byl
pre-deploy hook admits the reviewed migration manifest and schema 16; a schema-17
image is refused until an operator rehearses/applies migration 17, verifies retained
rows and backups, and updates the installed manifest and hook together. The
repository hook now expects schema 17; copying it alone cannot admit the new
migration manifest. This PR does not migrate production or claim deployment, live provider
qualification, firsthand observations, current access, or a field-tested route.
Client report/comparison screens and publication of a reviewed correction remain
the separate roadmap issues #47–#49 and #45.
