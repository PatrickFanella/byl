# Path quality comparison for the trail pilots

The path assessor was exercised on bounded Illinois Prairie Path and Great Western
Trail samples on 2026-11-07. Both remain **needs research**. Returned geometry does
not establish the required network identities, connectivity or mode-specific
access. No pilot itinerary was published and production routing was not changed.

The references came from the
[DuPage County DuDOT bikeways layer](https://gis.dupageco.org/arcgis/rest/services/DuDOT/Bikeways_MapService/MapServer/0).
The IPP reference was OBJECTID 114, vertices 0–7; the GWT reference was OBJECTID 74,
first component, vertices 6–13. Each contiguous sample used eight controls and
three synthetic story boundaries at sample vertices 0, 4 and 7. Synthetic arrival
objects exercised the protocol; they were not actual access decisions. Raw
reference geometry and provider responses remain private pending rights review.
The public layer's availability does not grant permission to copy it into Byl.

The GWT bounded query was retrieved at 15:08:55 UTC, SHA-256
`a04f0cc9ac920bedf739a87b8742913028343284c1017e0d97c5894c3d5bc89d`.
Its predicate selected existing Great Western Trail features intersecting the
small test envelope, limited to three records. The IPP source and prior adapter
measurements are retained in [the earlier comparison](approved-path-comparison-2026-11-07.md).

| Observation | Illinois Prairie Path | Great Western Trail |
| --- | ---: | ---: |
| Reference length | 589.698 m | 1,397.913 m |
| Walking and cycling distance returned | 590 m | 1,398 m |
| Walking / cycling time returned | 435 / 143 s | 1,048 / 362 s |
| Story legs | 2 | 2 |
| Largest story endpoint error | 1.007 m | 0.852 m |
| Largest returned-vertex distance to reference | 1.500 m | 5.605 m |
| Largest reference-vertex distance to returned line | 1.267 m | 5.606 m |
| Quality result, both modes | Needs research | Needs research |

These are directed vertex-to-line measurements using local segment projections
and haversine distances. They do not prove continuous geometric agreement or
network connectivity. The IPP quality assessment reused the previously captured
13:34 adapter results. GWT requests used the current `BuildPlan` and controlled
Valhalla adapter at 15:09:38 UTC for walking and 15:09:40 UTC for cycling, through
the public development demonstration service. No personal location was submitted.

The [Valhalla attribution contract](https://valhalla.github.io/valhalla/api/map-matching/)
provides a strict `edge_walk` option. The demo refused IPP attribution with HTTP
400/code 443, both for the combined shape and the original first story-leg shape.
GWT's walking shape also returned an explicit exact-match refusal. Cycling
attribution on GWT was not requested after that result. The cause of the strict
match refusals is unresolved. No map-snap fallback, alternate geometry or inferred
way/node identities were accepted.

The assessor therefore records `strict_attribution_unavailable`,
`network_reference_missing`, `access_unknown` and `access_not_inspected` for these
samples. Synthetic approval objects do not fill those gaps. A suitable configured
provider and independently inspected network/access sources are still needed to
qualify a real itinerary. The service is a development comparison target, not an
activated production dependency.

Independent fixtures prove that the implementation blocks an identically located
parallel road with different way IDs, a bridge lacking a shared junction, missing
connectors, forbidden modes and known closures. They also cover a lawful explicit
connector, absent provider identities, exact-attribution refusal and straight-line
development geometry. PostgreSQL worker tests carry actual retained synthetic
source passages through review, assessment and explicit publication; a later
closure blocks its mode while leaving the published snapshot unchanged. These
fixtures establish software behavior, not trail conditions or legal permission.
