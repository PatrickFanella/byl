# Great Western Trail: private out-and-back pilot

This is issue #62's bounded Villa Park–Lombard bike draft. Two short chapters
interpret Cortesi Park's CGW depot and a former Lombard CGW station in a municipal
photograph. The return is travel only; it adds no repeated story. The packet is
private and approves no current arrival, closure, crossing or property entry.

`brief.json` reproduces the intake. `sources.json` records seven inspected
historical/context sources. `stops.json` separates interpretation locations from
proposed mapped-path arrivals, each with unknown access and `public_arrival:null`.
The Cortesi coordinate is a park-memorial reference, not a depot-door survey. The
western map anchor is not an exact old footprint or a surviving-building claim.
Both precise exterior viewpoints still require review.

## Identities and inspected sources

- The [municipal depot record](https://www.invillapark.com/DocumentCenter/View/594/1926-Chicago-Great-Western-Railroad-Depot-PDF)
  supplies the depot and industry account. Actual PDF text was inspected through
  the web reader. Local byte retrieval and screenshot attempts failed with
  TLS/redirect errors; no local hash or photo inspection is claimed. Its
  historical uses and plans are not current access findings.
- The [district record, PDF pages 1–2](https://www.invillapark.com/DocumentCenter/View/561)
  distinguishes the CA&E museum/station from the CGW depot and describes their
  connecting path context. The [park facility record](https://www.invillapark.com/facilities/facility/details/Cortesi-Veterans-Memorial-Park-1)
  supplies a current address/hour lead. The independent
  [memorial location record](https://www.hmdb.org/m.asp?m=244906) helps locate the
  park; it does not identify an exact depot entrance. No photographs are copied.
- The [Village of Lombard StoryMap](https://storymaps.arcgis.com/stories/4440335cef514505ad70687dca5e6f3e)
  provides the western interpretation anchor at point 16. Public ArcGIS item/data
  were actually inspected and the municipal publisher checked. Its separately
  mapped CA&E point 42 is excluded. A photograph label supplies the 1949 year,
  without claiming that the depot was built then.
- The [2024 county plan, PDF page 11 / printed page 7](https://www.dupagecounty.gov/Transportation/Documents/2024-0212%20DUPAGE%20TRAILS%20PLAN%20ADOPTED.pdf)
  was directly downloaded and inspected as text and a rendered page. It supplies
  regional context, not detailed topology or present conditions. The
  [Lombard biking page](https://villageoflombard.org/411/Walking-Biking-in-Lombard)
  supplies further municipal planning references without establishing an all-clear.

`identity-exclusions.json` retains the separate Villa Avenue and Ardmore CA&E
stations, Lombard's CA&E crossing, the unrelated Elmhurst depot, and rejected
street-midpoint geocoder results. None becomes an extra stop. The two narratives
remain short supported drafts below public story requirements; images, complete
histories and exact current viewing positions remain curator work.

## Computed directions, return and connectors

`routing.json` records the actual seven-location bicycle request: two story
locations, four non-story shaping controls and a closing break at the start.
Valhalla/FOSSGIS returned two distinct legs:

| Direction | Computed distance | Computed cycling time |
| --- | ---: | ---: |
| Outbound to the western anchor | 4,135 m | 1,030.369 s |
| Return to Cortesi without repeated narration | 4,135 m | 1,031.332 s |
| Total travel | 8,270 m | 2,061.701 s |

Six minutes of planned dwell and four of narration give **44.36 conditional
minutes**. These are explicit planning assumptions, not observed ride times or
permission to use the route under present conditions.

`network-reference.json` retains the complete connected outbound reference:
64 actual OSM nodes and 25 ordered ways, approximately 4,132 m. The return selects
the reverse reference; the provider's return was separately computed. The
[ordered review table](network-review.md) identifies every way and its mapped
surface, including six explicit park/crossing connectors. A short way labelled
Illinois Prairie Path is part of this local connector, not permission to switch
the itinerary onto the CA&E Main Stem.

The named-trail capture initially had a 17 m gap at Westmore. A bounded source
inspection supplied the connected sidewalk, signal-crossing and sidewalk ways
1118220710, 1118220712 and 1118220714. The reference follows their actual shared
nodes. It inserts no straight crossing and treats no coincident line as a
junction. Mapped markings, bicycle designations and surfaces remain source
assertions awaiting current inspection.

Both directional comparisons have 65 decoded vertices, with maximum measured
separation below 0.74 m from the full reference. `comparison.json` records the
one-way vertex-to-line method and limits. Geometric proximity is not topology or
access evidence. Both strict `edge_walk` requests returned HTTP 200 but omit
required end-node identities; the quality assessor must retain that gap. There
is no inferred identity or approximate-match fallback, and the full runtime
corridor plan is not claimed qualified.

Original encoded shapes and metrics are unchanged. Requests, retrieval, hashes
and transformation are retained. Credit: Valhalla/FOSSGIS routing,
[© OpenStreetMap contributors](https://www.openstreetmap.org/copyright),
[ODbL 1.0](https://opendatacommons.org/licenses/odbl/1-0/). Map data is not
relicensed under the repository's software license. The small crossing/park map
sources have separate URLs and hashes in `network-reference.json`.

## Replays and release limits

The PostgreSQL/HTTP pilot replay checks separate municipalities, reviewed draft
identities, recorded-manifest research/assembly, stable completed-job retries,
the out-and-back sequence without repeated stories, all actual routing inputs,
reference continuity/hash/proximity, and a private blocked preview. The captured
comparison is explicitly imported as unapproved proposed-arrival geometry; it
does not fabricate a production quality checkpoint or live automated research.

`TestGreatWesternPilotTemporaryDetourSurvivesRecompute` uses a separate disposable
seed revision as a software stand-in. It submits a hypothetical temporary report,
reviews synthetic routing intent, releases it with a reassessment date, and
recomputes. The intent, temporary-review metadata and frozen released revision
must remain. This is not a report of an actual local closure, and the real pilot
is never published to perform the exercise.

Current closures, posted park conditions, crossings, precise exterior viewpoints,
permitted approaches and strict topology remain unqualified. A curator must
complete those checks, full stories and media rights before an explicit release.
Field and native-device results remain separate from fixtures and hosted CI.
Voluntary contribution collection stays disabled pending issue #63.
