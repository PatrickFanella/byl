# Deviation detector reference, uncalibrated v1

Issue #31 defines a pure reference detector and synthetic fixtures for the web,
iOS and Android implementations. The TypeScript implementation is
`web/src/lib/tour/deviation.ts`. It is not connected to GPS, navigation, storage
or an upload endpoint. `calibrated` is always false. Collection stays disabled
until issue #63 records the required field, consent and data-lifetime evidence.

A summary means that this detector observed enough qualified samples for one
logical traversal under its candidate rules. It does not establish legal access,
a better route, a distinct visitor, a verified closure or representative usage.
No threshold below is a field-calibrated product recommendation.

## Inputs and upstream obligations

Create one `DeviationDetector` for one immutable revision, logical segment,
mode and direction. Its context includes a reviewed reference polyline, known
competing paths and excluded stop-access areas. Reverse travel reverses the
reference before progress is measured. Context is copied on construction;
changing a revision or segment requires another detector.

`eligibility: reviewed` and `ambiguityReviewed: true` require upstream evidence.
They cannot turn missing map/network attribution into a reviewed result. A caller
must supply known parallel roads and overlapping traversals as competing paths,
and stop-access spurs as exclusion areas. Trim a logical traversal's endpoints
outside those areas before assigning its geometry. Otherwise the detector may
never observe a qualified start. Unknown context stays unsupported. Native and
web trip integration belongs to #50–#52, not this reference PR.

The constructor bounds work to 512 vertices per path, eight competing paths and
32 stop areas. A supported path is 50–5,000 metres long. Points must be finite,
within latitude ±85 and longitude ±180, and within 0.25 degrees of the original
reference origin. Stop-area radii are finite, nonnegative and at most 500 metres.
Invalid, oversized or unsupported context exposes `supported: false` and emits
nothing. These local geographic limits are not a regional matching algorithm.

Fixes supply latitude, longitude, horizontal accuracy in metres, and integer
epoch milliseconds. Adapters must normalize platform timestamps to that unit.
Unknown, negative or nonfinite accuracy is unusable. Exact coordinates and times
are used only in bounded in-memory detection state, not in a summary.

## Candidate constants

The versioned constants in `DEVIATION_RULES` are fixed for this reference:

| Rule | Candidate value |
| --- | --- |
| Maximum usable horizontal accuracy | 20 m |
| Effective minimum accuracy | 3 m |
| Confidently inside | distance + effective accuracy ≤ 20 m |
| Confidently outside | distance − effective accuracy ≥ 35 m |
| Known ambiguity margin | effective accuracy + 10 m |
| Maximum gap between usable samples | 20 s |
| Maximum continuous uncertainty bridge | 20 s |
| Maximum passage evidence lifetime | 15 min, within one UTC day |
| Maximum plausible movement | 4 m/s walking; 15 m/s cycling |
| Sustained outside evidence | ≥3 confident outside samples over ≥20 s |
| Outside displacement lower bound | ≥30 m |
| Rejoin | ≥2 consecutive confident inside samples, ≥30 m beyond departure |
| Passage coverage | start in first 10%, finish in last 10% |
| Passage evidence | ≥5 confident samples, ≥30 s, ≥30 m displacement lower bound |

The existing 65 m arrival tolerance is not used. Arrival remains a separate
navigation behavior. These stricter candidate rules intentionally omit many
observations. Their omissions, false positives and platform effects still need
measurement under #63.

## Geometry and event semantics

Project coordinates onto a local tangent plane with Earth radius 6,371,000 m:

```text
x = (longitude - origin_longitude) * pi / 180 * R * cos(origin_latitude * pi / 180)
y = (latitude - origin_latitude) * pi / 180 * R
```

The origin is the original reference's first point, including for reverse travel.
For each nonzero edge, clamp its scalar projection to [0, 1]. Record distance to
the edge and cumulative along-path distance. The closest projection defines
progress. This is a local approximation, not geodesic routing or network proof.

Apply these rules in order:

1. A pause, permission loss or manual arrival discards the current episode and
   suspends detection. Only an explicit resume permits fresh evidence. End is
   terminal for the instance. None of these controls emits a deviation.
2. Ignore malformed, out-of-range, duplicate or reversed timestamps. A timestamp
   must fit years 1970–9999. Invalid coordinates or accuracy clear the episode.
   A gap over 20 seconds or implausible movement clears it and discards that fix.
   Plausibility uses `max(0, distance - previous_accuracy - current_accuracy)`
   divided by elapsed seconds, with both accuracies floored at 3 metres.
3. Clear evidence at a UTC-day boundary or after 15 minutes. A later observation
   must establish a new eligible start; it cannot bridge missing evidence.
4. Suppress a fix if another projection lies within the ambiguity margin and
   differs in along-path distance by more than `max(20 m, 2 * accuracy)`.
   This catches intersections and repeated/out-and-back geometry. Any supplied
   competing path within the margin, or a stop area within radius plus accuracy,
   also clears the episode. Geometry not supplied cannot be detected by magic.
5. Clear evidence on backward progress over 30 metres. A fix between the inside
   and outside bands is uncertain. It breaks the inside streak and discards an
   unqualified outside candidate. A qualified departure may bridge at most
   20 seconds of uncertainty before rejoining; longer uncertainty clears it.
6. A confident observation in the first 10% establishes a passage start.
   Two consecutive confident inside samples arm departure detection. An outside
   episode needs the sample, elapsed-time and displacement bounds above before
   becoming a candidate. Its displacement lower bound is endpoint distance
   minus the two effective accuracies, not an accumulated raw trace.
7. Two consecutive confident inside samples clear the outside episode. Set its
   deviation flag only if the outside episode qualified and rejoin progress is
   at least 30 metres beyond its departure. A stationary break or brief crossing
   therefore does not qualify merely by being away from the line.
8. Emit only after the complete passage coverage and evidence bounds pass, with
   no pending outside candidate. A partial departure/rejoin has no summary.
   Release sampled coordinate/time state after emission. An instance emits at
   most one summary, even if every fix or the entire trace is replayed.

Qualified passage displacement is also an endpoint lower bound. Curved travel,
loops with nearby endpoints, sparse samples and broad accuracy bounds can fail
this conservative screen even when a visitor actually traversed the segment.
The shared fixtures include a nonoverlapping loop that qualifies and overlapping
loop/out-and-back cases that do not. Do not describe a suppressed result as proof
that no visitor deviation occurred.

## Minimal output and privacy boundary

Each `DeviationSummary` contains only schema/detector versions, `calibrated:
false`, revision/segment/mode/direction, one UTC observation day,
`qualified_passages: 1`, and `sustained_departure_rejoins: 0 | 1`. It contains no
coordinates, exact timestamps, account/device IDs, persistent trip identifier,
distance trace or historical narrative. Pause and end release pending samples.
There is no filesystem, browser storage, logging, consent or upload side effect.

The single-instance replay latch is not server deduplication or independent
corroboration. Replaying data through a new instance can reproduce its summary.
The disabled-by-default backend contract in #23 must provide scoped idempotency,
separate consent, withdrawal, retention and deletion. #53 must avoid counting
duplicates as independent reports. Navigation must continue when contribution
collection is unavailable or disabled.

## Shared fixture format and verification

`web/src/lib/tour/fixtures/deviation-v1.json` is the portable source of 26 entirely
synthetic cases. Its origin is a coordinate conversion frame, not a captured
visitor trace. `epoch` supplies a fixed UTC time for deterministic output.
Reference and competing geometry use `[x_metres, y_metres]`. Stop areas use
`[x_metres, y_metres, radius_metres]`. Fix rows use
`[elapsed_seconds, x_metres, y_metres, accuracy_metres]`; convert the offset to
integer epoch milliseconds. Control rows contain an explicit `action`.

Use the inverse of the projection above to create fixture coordinates. Ports
must consume the same ordered rows, including duplicate and decreasing times.
`expected_deviation: null` means no summary. False means one qualified passage
with zero sustained departure/rejoins. True means one passage with one such
departure/rejoin. All remaining output fields are fixed by the fixture context.

Cases cover positive forward/reverse passages, sustained walking/cycling
detours, poor accuracy, stationary jitter/breaks, a short crossing, stop access,
parallel roads, pauses, permission loss, manual arrival, terminal end, gaps,
implausible jumps, repeated geometry, unsupported context, replay, uncertainty
bridges, partial observations and a simple loop. Additional tests check context
copying, malformed geometry, output minimization and control boundaries.

Run `npm run test:unit -- --run src/lib/tour/deviation.test.ts` from `web`, then
the repository's web checks and production build. Swift/Kotlin ports should run
these same synthetic semantics under #51/#52. Passing fixtures establishes
reference conformance only. It neither calibrates the thresholds nor enables
collection, publishes route changes or supplies device/field evidence.
