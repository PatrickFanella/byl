# Deviation rollout qualification: NO-GO

The November 8, 2026 decision for issue #63 is **NO-GO**. Collection remains
disabled. There are zero independently labelled field observations, so a false
flag rate cannot be estimated. Passing synthetic cases does not calibrate a
threshold. This record does not complete or close #63.

[results.json](results.json) retains the bounded fixture hashes, scenario list,
candidate aggregation thresholds, existing test results, live admission response
and the unrun field matrix. All three clients consume the same 26 synthetic
cases. Their retained inputs cover parallel roads, crossings, breaks, sustained
detours, return overlap and sparse observations. The real pilot packets provide
route context; they are not captured GPS samples and are not substituted for
field truth. Great Western is PR #126; Prairie Path is merged PR #125.

## Evidence and limits

| Surface | Observed result | Limit |
| --- | --- | --- |
| Web | 184 unit tests and 93 Chromium cases pass on the Great Western branch, including contribution controls, offline retry, deletion and navigation cases. | Disposable API and synthetic observation transports; phone-width rendering is not native-device execution. |
| Swift | 190 package tests pass on Soyuz against the current source, including detector, context, durable store, API and trip controller tests. | macOS package execution, not representative iOS GPS/background behavior. |
| Kotlin | Pinned Gradle container core suite builds and passes against d32fd20. | JVM tests, not Android location-service/device behavior. |
| Backend | Race-enabled HTTP, repository, config and model contribution suites pass with actual disposable PostgreSQL. | They prove software behavior for consent/replay, withdrawal, expiry, physical deletion, redaction, restore hold and review grouping. They do not inspect production copies or logs. |
| Current public gate | `/api/v1/route-contribution-intake` returns HTTP 200 with `enabled:false`, `calibrated:false`, withdrawal available. | Read-only snapshot; no admission or activation was performed. |
| Phone UI | Android simulator discovery succeeds; opening fails because the provider no longer owns an active T3 thread run. Linux offers no local iOS simulator. | Device qualification is blocked. No alternative device driver or fabricated result was used. |
| Actual field routes and custody | Not run / not established. | Required before any activation decision. |

## Candidate thresholds remain uncalibrated

The detector constants and their semantic rationale remain in
[the reference specification](../deviation-detector-v1.md#candidate-constants).
No distance, sample, uncertainty, elapsed-time or speed constant changes here.
The existing three-confident-sample/20-second departure rule is a conservative
candidate filter, not a measured sensitivity or false-positive guarantee.

Review grouping is separately versioned `uncalibrated-review-v1`: at least five
account scopes, three departure scopes and 30 percent departure/rejoin summaries
within a nonoverlapping Monday-based UTC week for one revision, segment, mode and
direction. The independent-count and rate gates prevent one repeat account or a
small share of observations from creating a review flag. They are not confidence
intervals, distinct-human proof or a representative sample of all visitors.
Withdrawal and expiry must remove both the live count and any flag on the next
read. No automatic closure, reroute or publication follows a flag.

## Required field evidence

Run the six scenarios in results.json on **both** trail pilots with web, iOS and
Android, representative phones/OS versions and explicit participant permission.
First qualify current allowed arrivals, crossings, precise corridor topology and
reviewed competitor/stop-area signal references. Do not mark an unsupported
segment eligible merely to produce detector output.

Use an independent observer's scenario labels and document device/OS, client
commit, route revision, mode/direction, sample availability and foreground,
background, lock, permission, offline and power states. Distinguish intentional
crossings and breaks from independently confirmed sustained detours. Record the
number of eligible passages, excluded/suppressed observations, false flags and
missed labelled detours for each scenario/platform. Keep the denominators and
uncertainty visible; no denominator means no rate. Do not send raw diagnostic
traces to the contribution API or put identifiable coordinates/timestamps in
repository fixtures. Any separately consented diagnostic collection needs its
own bounded custody, access and deletion decision first.

The actual field matrix is currently **not run** in every cell. Collection stays
off while that evidence is missing. A later reviewed calibration commit must
explain any changed constants, rerun all clients against identical bounded
fixtures and record measured tradeoffs; selecting convenient thresholds is not
calibration.

## Activation checklist: every gate is currently incomplete

- [ ] Approve actual field/device evidence and reviewed signal contexts for both pilots.
- [ ] Exercise separate consent, withdrawal after sign-out, deletion with intake off,
  expiry and offline queue limits on representative clients while navigation continues.
- [ ] Inventory actual database/WAL/volume/host/off-host copies and proxy/collector logs;
  establish retention, deletion ownership, cleanup monitoring and outage cleanup.
- [ ] Restore the filtered deployment backup in isolation; verify contribution and
  custody tables have no restored rows, unrelated data remains, and admission is closed.
- [ ] Admit only the exact reviewed schema/image and operator-managed custody settings,
  following [the existing procedure](../route-contributions.md#activation-backups-and-restore).
  Keep secrets private; never copy them into this report or a command log.
- [ ] Respect the two UTC date-boundary hold after custody admission. Record the actual
  receipt and time, recheck all gates, then require a separate operator activation.
- [ ] Verify the effective live gate, authenticated explicit consent, retained revision
  fences, minimized storage/logs, withdrawal and navigation in the activated release.

No box is checked merely because a matching fixture passes. Actual production
custody and representative client checks remain operator/field work.

## Rollback and disable checklist

1. Set `ROUTE_CONTRIBUTIONS_ENABLED=false` through the approved deployment path and
   read the effective intake response back as disabled. Confirm navigation and
   ordinary tour access continue.
2. Verify each active client stops observing when its gate lease closes; pending
   work must not upload through a closed gate. Retain explicit deletion status.
3. Keep the compatible withdrawal endpoint and schema-19 retention binary available.
   Verify withdrawal and physical cleanup while intake is disabled, including during
   an API outage under the separately owned cleanup schedule.
4. Keep filtered-backup exclusions and actual copy-expiry obligations in force.
   A restore must close admission. Never reactivate merely by restoring process flags.
5. Retain schema and deletion capabilities through image rollback; do not use a
   destructive down migration or a pre-19 API as the only erasure service.
6. Record the exact image/revision, live gate, navigation, withdrawal and cleanup
   evidence. Reopening requires a new explicit decision against the full checklist.

This PR changes documentation only. It enables no feature, emits no observation,
changes no threshold and claims no field qualification.
