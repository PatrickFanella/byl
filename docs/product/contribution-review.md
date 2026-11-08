# Live curator tasks from contribution patterns

Issue #53 adds a curator-only review queue at `/admin/queue/contributions`.
It reads live, minimal contribution summaries and never changes a public tour.
No intake setting, custody admission, publication rule or field calibration is
changed by this feature.

## Sample and threshold policy

Policy `uncalibrated-review-v1` requires all of:

- At least five distinct account scopes with eligible observations.
- At least three distinct account scopes with a sustained departure/rejoin.
- Sustained departure/rejoins in at least 30 percent of eligible observed passages.

These are explicit initial review thresholds, not calibrated safety or closure
criteria. An account scope is distinct within a tour/revision/mode, not proof
of a distinct person. Multiple accounts, repeated visits, client sampling and
opt-in selection bias limit any interpretation.

The numerator sums the accepted zero/one departure/rejoin value. The denominator
counts accepted qualified passage summaries only. A missing GPS episode, day,
visitor or upload contributes neither a departure nor a clean passage. The
backend's unique scope/segment/direction/UTC-day constraint and idempotency
contract prevent upload replays from adding observations. Repeat visits across
days can add passages but never create additional independent account scopes.

Each group keeps the immutable revision, logical corridor segment, walk/bike
mode, direction and UTC Monday week separate. Windows are seven UTC days with
an exclusive end boundary. The current week and preceding three weeks are
considered independently. The current week is incomplete; the response shows
its observation-through day. No segment, direction, mode or revision fallback
supplies a missing denominator.

## Live tasks and deletion

The queue is a read-only projection in a repeatable-read database snapshot.
It persists no rollup, task snapshot, contributor list or generated visitor
report. Revoked/expired consent and expired summaries are excluded before
aggregation, even if physical retention cleanup has not run. Future observations
and observations preceding consent creation are excluded too.

Withdrawal and expiry therefore update the next read. A selected task's deep
link returns current counts and a cleared flag when its threshold no longer
holds, including zero after its final observation expires. A deleted tour or
missing retained reference returns unavailable. There is no background task
history requiring separate erasure. Responses are `no-store`, vary on
Authorization and expose counts without consent IDs, account IDs or scope hashes.
Only curator-authorized requests can read them.

## Curator workflow

The task displays numerator, denominator, independent scope counts, window,
threshold policy and the affected frozen reference geometry. Its related-report
links identify explicit reports on the same revision and mode. Reports describe
stop-to-stop legs or arrivals rather than corridor IDs, so their relationship to
the logical section remains unverified; they are not automatic corroboration.
The response includes only report ID, status, reason and affected location, with
the newest 20 shown and an explicit truncation notice.

A curator can inspect a real report through the existing retained investigation,
working-input review, correction comparison and guarded publication workflow.
Without a report, the task links to the working tour for ordinary evidence and
navigation review. It does not fabricate a report or infer a replacement path.
The working draft may differ from the flagged revision. Existing access,
independent source, quality, review and publication gates still apply.

## Verification

Synthetic PostgreSQL tests cover one repeated visitor, a genuine repeated
pattern, sufficient independent scopes with a low departure share, separate
modes/directions/revisions/weeks, missing observations, withdrawal and expiry.
HTTP tests cover role enforcement, replay deduplication, private response shape,
related explicit reports and live cleared-task reads. Browser tests use a clearly
synthetic aggregate response with a real report and exercise queue/detail counts,
sample caveats, reload after threshold clearance, error/empty states and entry
into the normal report review workflow. This is software qualification, not a
claim about a real route or production collection.
