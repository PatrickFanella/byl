# Illinois Prairie Path Main Stem: private pilot packet

This is issue #61's Villa Park–Wheaton bike draft for curator review. Three short
chapters interpret the Villa Avenue museum/station, Ardmore station and the
conversion campaign at a later commemorative location. The route follows the
Main Stem. The Elgin and Aurora branches, and the separate Great Western Trail,
are outside this itinerary. No content or arrival approval is published.

`brief.json` reproduces the intake. `sources.json` records nine inspected source
records retrieved November 8, 2026. `stops.json` separates historical building
locations from proposed on-trail exterior viewpoints. Each `public_arrival` is
null; proposed arrivals retain unknown access and need review of current
sightlines, stopping space, approaches and crossings.

## Historical anchors

- The [Village preservation account](https://www.invillapark.com/262/Historical-Preservation-Commission)
  distinguishes the two CA&E stations and the separate Great Western depot.
  A [2026 municipal newsletter](https://www.invillapark.com/CivicSend/ViewMessage/message/292980)
  also identifies the museum as housed in the 1929 CA&E station. Its event hours
  are not current visitor guidance. Museum entry and current business access
  remain outside this exterior draft.
- [Prairie Path history](https://ipp.org/history/) and
  [Openlands](https://openlands.org/regional-trails/illinois-prairie-path/)
  support a collective conversion-campaign chapter. The later Founders Park is
  an interpretation anchor, not the asserted site of the campaign's events.
  Its location is supported by the [Winter 2017 newsletter, page 3](https://www.ipp.org/wp-content/uploads/IPP-2017-DECEMBER-Newsletter-.pdf)
  and the [City council packet, PDF page 7](https://www.wheaton.il.us/AgendaCenter/ViewFile/Agenda/_06232025-2882?packet=true).
  The selected mapped trail point is within those described bounds; it is not
  claimed as a survey of the memorial boulder. A name-only geocoder query found
  no park result, so no purported park centroid was substituted.
- The [free map](https://ipp.org/pdf/IPP-free-map-color-updated.pdf) was visually
  inspected for branch orientation. Its 2009 update and coarse scale make it
  unsuitable for current topology or access approval. Openlands' longer combined
  loop and its timing are separate from this pilot.

The accounts remain supported short drafts, below public story requirements.
Year precision normalizes to January 1 without claiming an exact day. No map
page, historical photograph or present-day site image is copied into the packet.

## Actual computed route and reference comparison

`routing.json` records an actual bicycle request to the Valhalla/FOSSGIS public
development demo: three story breaks and seven non-story shaping points. The
response returned **12,372 m**, **3,111.007 seconds** and two story legs. Nine
minutes of planned dwell plus three of narration gives **63.85 conditional
minutes**. This is a point-to-point plan. It includes no return journey; visitors
would need a separately reviewed return arrangement. These figures are computed
planning estimates, not field timings.

The demo rejected a denser request because its bicycle limit is ten locations.
The bounded request retains all three story breaks and selects seven shaping
nodes. Those request controls do not replace the complete reference. They do not
claim qualification of the app's full corridor-controlled runtime plan.

`network-reference.json` retains the connected **401-node, 69-way** Main Stem
chain, ordered travel directions, partial-way bounds, source query, OSM base
timestamp and retrieval hash. The selected chain excludes branch and Great
Western ways. Its reference length is approximately 12,361 m.
[The ordered review table](network-review.md) lists every selected way, mapped
surface and crossing information. Explicit crossing-tag ways are also listed as
proposed crossing connectors. Absence of a tag proves no crossing condition.
No off-trail building-entry connector is selected. Such an approach would need
its own reference and access review.

`comparison.json` measures decoded provider vertices against the complete
reference, using local segment projections. The maximum observed separation is
under **0.07 m**. This one-way vertex comparison does not prove continuous
agreement, network identity, connectivity or permitted access. An adjacent road
can be geometrically close; the topology guard remains necessary.

Strict attribution remains incomplete. The first leg's response lacks the node
information required by the app, and exact `edge_walk` matching for the second
leg returned HTTP 400. Both actual responses are retained. No `walk_or_snap`
fallback, substitute shape or inferred missing node ID fills those gaps. The
Valhalla adapter replay must retain an explicit failure, and the curator package
must remain unqualified. This agrees with the earlier
[bounded pilot comparison](../../../routing/path-quality-comparison-2026-11-07.md).

The encoded response shapes and summary metrics are unchanged. Requests,
retrieval, hashes and transformation are in `routing.json`. Network data and
provider-derived map geometry are credited to
[© OpenStreetMap contributors](https://www.openstreetmap.org/copyright), under
[ODbL 1.0](https://opendatacommons.org/licenses/odbl/1-0/); routing is credited to
Valhalla/FOSSGIS. Map data is not relicensed under the software license.

## Access, software checks and release limits

The [closure page](https://ipp.org/construction-updates/) was inspected on the
retrieval date. Its Elmhurst interchange notice and Elgin Branch parking notes
do not establish this segment's condition. The April notice has no explicit year
in the inspected text. A fresh current-conditions check and field inspection are
still required. The [trail overview](https://ipp.org/about-the-trail/)'s broad
limestone description does not settle each mapped compacted, paved or asphalt
surface in this chain.

`TestPrairiePathPilotReplayReconcilesDraftsAndKeepsPublicationBlocked` exercises
real PostgreSQL/HTTP intake, separate municipality identities, candidate and
versioned curator decisions, recorded-manifest research/assembly and completed
job replay. The recorded discovery stage is not a claim of live automated web
research. Its actual ten-control adapter replay preserves three story breaks,
two legs and strict-attribution failures. The captured comparison is imported
into a disposable private preview, explicitly marked as unapproved proposed-
arrival geometry. It is not a production route-quality checkpoint.

`synthetic-checks.json` identifies the existing parallel-road and crossing
conformance fixtures. The quality suite rejects an identically located road with
a different way identity. A synthetic arrival correction in the PostgreSQL pilot
test leaves historical-site coordinates unchanged and unknown access ineligible.
These fixtures describe software behavior; they supply no local observation or
device calibration. No crossing control becomes another narrated exhibit.

Before release, a curator still needs complete histories and media rights,
current exterior arrivals and stopping places, crossing and access inspections,
qualified strict topology, full runtime corridor computation, and an explicit
publication decision. Native-device and field results remain separate. The
voluntary contribution rollout stays disabled pending issue #63's evidence.
