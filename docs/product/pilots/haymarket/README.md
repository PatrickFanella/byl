# Haymarket and the eight-hour day: private pilot packet

This is issue #60's two-stop Chicago draft for curator review. It links the
meeting-site memorial with a former courthouse-site interpretation chapter.
Greif's Hall is an unpinned research lead, and the Forest Park cemetery monument
is a separate, unqualified extension. The packet publishes no content and
approves no present-day arrival or crossing.

`brief.json` reproduces the intake. Nine inspected source records are in
`sources.json`, retrieved November 8, 2026. `stops.json` separates location leads
from public arrivals and contains two supported short drafts. Both
`public_arrival` values are null. `research-gaps.json` records exclusions rather
than filling the route with unsupported nearby stops.

## Historical and location decisions

- The [Chicago History Museum overview](https://www.chicagohistory.org/haymarket/)
  supplies context for the eight-hour movement and May 4 meeting. The
  [City landmark record](https://webapps1.chicago.gov/landmarksweb/web/landmarkdetails.htm?lanId=1322)
  locates the meeting area north of Randolph. The
  [Illinois Labor History Society](https://www.illinoislaborhistory.org/chicago)
  distinguishes the Chicago memorial at 175 N Desplaines from the Forest Park
  monument at 863 Desplaines Avenue. Similar names do not make them one place.
- [Greif's testimony](https://www.chicagohistoryresources.org/hadc/transcript/volumei/301-350/I325-335.htm)
  and [Bushick's testimony](https://www.chicagohistoryresources.org/hadc/transcript/volumei/000-050/I002-018.htm)
  retain witness attribution, examination context and uncertainty. A historical
  street number is not sent to a modern geocoder as if it were unchanged.
  Without parcel and building-continuity evidence, Greif's Hall receives no pin.
- The [museum's courthouse caption](https://www.chicagohistoryresources.org/dramas/act3/anarchyOnTrial/courthouseArea_c.htm)
  distinguishes the demolished trial building from its replacement. The
  [City's Courthouse Place record](https://webapps1.chicago.gov/landmarksweb/web/landmarkdetails.htm?lanId=1283)
  identifies today's building. The draft uses its exterior as an interpretation
  anchor, without claiming a surviving 1886 courtroom. The historical
  [photograph catalogue](https://www.chicagohistoryresources.org/hadc/visuals/26V1380.htm)
  has a corner-direction conflict with another museum account. The exact
  execution courtyard stays unpinned. No photograph is copied.
- The [National Park Service monument record](https://www.nps.gov/places/haymarket-martyr-s-monument.htm)
  supports the cemetery monument's identity and dedication. The separate
  `forest-park-extension.json` supplies no walking line or invented connection
  from downtown Chicago. A transit chapter requires its own retained plan,
  current access review and explicit qualification.

Disputed testimony and differences between general accounts remain attributed;
this packet does not identify the bomber or silently settle conflicting casualty
counts. The source selection does not endorse every claim on a linked page.
Year precision normalizes to January 1 for storage without claiming an exact
day. Both narratives remain below the public story-length requirement.

## Computed comparison

`routing.json` records an actual pedestrian request to the Valhalla/FOSSGIS public
development demo. It returned **1,716 m** and **1,300.437 seconds** between the two
location leads. Adding explicit assumptions of 12 minutes of dwell and two
minutes of narration gives **35.67 conditional minutes**. This is not an observed
duration or a current sidewalk/crossing certification. It excludes Forest Park.

The original encoded shape and summaries are retained in
`valhalla-response.json`; hashes, retrieval, request and transformation are in
`routing.json`. Credit: Valhalla/FOSSGIS routing and
[© OpenStreetMap contributors](https://www.openstreetmap.org/copyright), under
[ODbL 1.0](https://opendatacommons.org/licenses/odbl/1-0/). The map data is not
relicensed under the repository's software license. No inspected network
reference, present-day public viewpoint or field evidence is asserted.

## Reproducible checks and release limits

`TestHaymarketPilotReplayReconcilesDraftsAndKeepsPublicationBlocked` creates an
isolated Chicago catalogue in disposable PostgreSQL and exercises real HTTP
brief intake, conservative reconciliation, draft approval, versioned identity
selection, recorded-manifest research and itinerary assembly. Retrying a
completed research job leaves the checkpoint and place count unchanged. The
recorded research stage is not live automated source discovery.

The real Valhalla adapter replays the captured two-point geometry. The real
curator package contains the two selected stories, sources, computed route and
remaining tasks. It stays private and blocked from publication. The browser
transport fixture is exported from this replay; repeated story bodies are
restored from `stops.json`. Its phone-width checks cover preview content,
geometry, source links, overflow and disabled publication.

A curator still needs full historical/identity review, complete supported
stories and illustration rights, inspected public exterior viewpoints and
arrivals, current crossing/access review and route-quality evidence. Native
device behavior and field walking remain separate checks. This PR changes no
schema, production catalogue, operational settings or publication authority.
