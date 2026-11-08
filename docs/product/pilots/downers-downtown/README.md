# Downtown Downers Grove: private pilot packet

This is issue #59's architecture-and-commerce draft for curator review. The
five-stop selection reuses the existing station and Tivoli identities, retains
the Tivoli story unchanged, and proposes four short, sourced accounts. It remains
private. Current arrivals, crossings, illustrations and network attribution need
review before publication.

`brief.json` reproduces the intake. `sources.json` records seven inspected source
records, passages and uncertainties retrieved November 8, 2026. `stops.json`
contains location leads, identity links, story selection and gaps. Each
`public_arrival` is null. None of the historical addresses or geocoded business
points establishes an accessible entrance or public viewing position.

## Selection and identity decisions

- The [station architecture account](https://dghistory.org/history-walk-12-main-street-train-station/)
  distinguishes the 1911 Classical Revival building from the earlier 1864
  station. The [municipal minutes, page 3](https://agendadocs.downers.us/public/docs/agendas/2016/05-17-16/MIN_2016-6827.pdf)
  use 1000 Burlington Avenue; the existing catalogue uses 5001 Main Street.
  The pilot retains one place and selects a new building-history draft, rather
  than automatically featuring the existing railroad-arrival narrative.
- The [Tivoli account](https://dghistory.org/tivoli-theater/) supports the 1928
  building's mixed entertainment, commercial and residential uses. The existing
  place and “Built for the Talkies” story are reused. The `body` in this stop's
  manifest is a selection note, explicitly marked as such, and must never replace
  its published prose. The retained story's word count and SHA-256 are recorded.
- [Mochel](https://dghistory.org/mochel-hardware-store/) retains the old 36 S Main
  address alongside 5122 Main. Opening, later expansions and storefront changes
  remain distinct. The [bank](https://dghistory.org/farmers-and-merchants-bank/)
  account supports 5135 Main, an 1892 opening and later changes including removal
  of the turret. [Penner](https://dghistory.org/penner-building/) retains circa
  1894 construction separately from the harness business's move around 1920.
- The [brick-street account](https://dghistory.org/historic-brick-streets/) was
  investigated but supplies no reviewed exact segment for this packet. The
  selection is excluded in `exclusions.json` until material, identity and public
  viewing conditions are supported. The route is not padded to include it.

Year and circa dates normalize to January 1 for storage; they do not assert an
exact day. The four new accounts are supported short drafts, below publication
length. No historical photos have been copied or present business-entry
permission inferred.

## Computed loop and conditional timing

`routing.json` records the actual pedestrian request to the Valhalla/FOSSGIS
public development demo. The five story locations are followed by the first
station coordinate as a sixth routing break. The response returned **1,246 m**,
**886.065 seconds** of walking and five legs. The final leg returns to the station
without introducing another story.

The explicit planning assumptions add 15 minutes of dwell and seven minutes of
narration: three for the retained 333-word Tivoli story and one for each short
draft. The resulting **36.77 minutes** is conditional arithmetic, not an observed
duration or a promise that these unreviewed approaches are suitable.

`valhalla-response.json` preserves the original encoded shapes and summary
metrics. Original and fixture hashes, request, retrieval time, and transformation
are in `routing.json`. Routing is credited to Valhalla/FOSSGIS; map data is
[© OpenStreetMap contributors](https://www.openstreetmap.org/copyright), under
[ODbL 1.0](https://opendatacommons.org/licenses/odbl/1-0/). This data is not
relicensed by the repository's software license. Captured geometry is a computed
comparison, not inspected sidewalk, crossing, network or legal-access evidence.

## Reproducible software checks

`TestDowntownPilotReusesStoriesComputesLoopAndRetainsCrossingReport` runs against
disposable PostgreSQL and real HTTP handlers. It checks reconciliation retries,
two reused places and exactly three new draft places, versioned address/identity
decisions, retained Tivoli prose, recorded-manifest research and curator preview.
The recorded discovery stage does not claim live automated web research.

The loop portion replays the actual six-break request through the Valhalla
adapter, then imports that comparison into the disposable curator preview. Its
five story-bound legs include a non-narrated return and are explicitly marked
`unapproved_computed_comparison`. There is no approved-arrival or corridor
fixture, network attribution or passing quality report. Production controlled
routing rejects a naive straight reference for this geometry; the test does not
weaken or bypass that guard for a public release. Computing a comparison and
reviewing it are distinct from approving it for navigation.
A separate frozen seed revision stands in for the crossing-report exercise.
The test submits and accepts an explicitly synthetic improvement report, then
checks the public snapshot is unchanged. Accepting a report supplies no current
crossing evidence and applies no route alteration. The new pilot stays private.

The browser fixture is exported from this HTTP replay, with repeated draft
bodies stored once in `stops.json`. Its phone-width preview checks stories,
source links, computed route and disabled publication. It demonstrates the web
review surface; it is not a native-device or field result.

Before a public release, a curator must complete historical and identity review,
write complete supported stories, clear image rights, approve actual exterior
viewpoints and arrivals, inspect crossings and permitted access, compare the
route with an inspected network reference, and explicitly publish a qualified
revision. This packet performs none of those production actions.
