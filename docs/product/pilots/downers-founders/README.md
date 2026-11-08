# Founding Downers Grove: private pilot packet

This is issue #57's source-backed draft for curator review. It is not a published
tour, a public-access approval or field qualification. The four accounts are short
supported drafts, below the public story-length requirement. Illustrations,
arrival approvals and inspected corridor access remain incomplete. Nothing in
this packet changes production content.

`brief.json` is the reproducible intake. `sources.json` records inspected sources,
compact passages, attributed claims and limits. `stops.json` separates location
leads from public arrivals, retains supported short accounts and names unresolved
work. Source retrieval was November 8, 2026. Source URLs should be rechecked before
a curator relies on their current access information.

## Research decisions

- Main Street Cemetery and the Downer family ground are separate places. The
  former has a cemetery location reference; its address needs reconciliation.
  The Historical Society lists 5190 Main Street. A municipal PDF search result
  lists 5158, but the large PDF was not inspectable in the browser at this
  checkpoint, so that lead is not treated as inspected municipal evidence.
  Address geocoding for both numbers returned a nearby building and was rejected
  as a cemetery position. The independent cemetery reference supplies a location
  lead, without proving its entrance. Its photographs require permission and
  were not copied.
- The Historical Society gives 1832 for Downer's arrival; the Park District gives
  1833. Both remain. Colonial-settlement descriptions do not imply the land was
  previously uninhabited. The family's flooded-creek burial account is attributed
  history, not present crossing guidance.
- The Park District describes dawn-to-dusk visits via a signed driveway shared
  with a neighboring home. This supports a specific access lead; it does not
  supply reviewed coordinates, current field conditions or permission to use
  adjacent private land.
- The house account and municipal landmark-review minutes distinguish the
  historic structure from alterations and changed entrance orientation. The
  early-1840s dating remains a range. Testimony and uncertain oral accounts are
  not promoted to independent findings. A landmarked residence is not a public
  attraction with automatic entry permission.
- The well account qualifies its proposed household use. Its text says historic
  recognition in 2013 while its approval field says 2003. Both remain unresolved.
  House and well address geocoding returned the same OSM way; they retain
  distinct physical identities. Neither result establishes a surveyed site,
  exact well position or safe viewing/crossing point.

Each date uses the precision supported by its source. January 1 in stored dates
normalizes year/circa database input; it does not claim an event happened that day.
No actual public-arrival object is supplied in `stops.json`.

## Computed comparison and timing

`routing.json` retains the exact request, retrieval time, original response hash,
transformation receipt and conditional measurements. The public Valhalla demo
returned three walking legs: 2.722 km and 1,989.352 seconds, about 33.16 minutes.
The request used the listed location leads, not approved entrances. Four explicit
three-minute dwells and four one-minute narration allowances produce a
conditional 49.16-minute total. This does not promise the proposed 60–90 minute
range. The brief leaves its target duration unset; the assembler's separate
60-minute optimistic geography screen is not a travel-time result.

The final house-to-well leg is about 218 m despite the close address leads. That
is a routing observation requiring map/crossing review, not proof of a legal
street crossing. No way/node connectivity or mode-specific access inspection
was captured. Approved arrivals or corridor changes require recomputation.

`valhalla-response.json` preserves the original encoded shapes and summary
metrics, omitting maneuvers and diagnostics. It is a deterministic offline replay
of a real computed response. It is not fabricated straight-line walking geometry.
The raw response is retained privately with the hash in `routing.json`.

The routing/geocoding data derives from OpenStreetMap and is supplied under
[ODbL 1.0](https://opendatacommons.org/licenses/odbl/1-0/), with attribution to
[OpenStreetMap contributors](https://www.openstreetmap.org/copyright).
Routing was computed by the Valhalla/FOSSGIS public development demo. The
repository's software license does not relicense those data. Historic-source
passages retain their original owners; their inclusion does not clear images.

## Disposable replay

From the repository root, set `TEST_DATABASE_URL` to a disposable PostgreSQL
database, then run:

```sh
test -n "${TEST_DATABASE_URL:-}"
(cd backend && go test -count=1 -v ./internal/http -run '^TestFoundersPilot')
```

The harness creates an isolated migrated schema and removes it after the run.
Discovery input is the recorded manifest; no live provider discovery or paid
model is invoked. The real worker runs reconciliation, material capture and
assembly. Curator controls approve draft identities and link supported short
accounts. Repeated reconciliation and redelivery of a completed research job
must not duplicate places or change the retained proposal.

The route adapter replays the captured provider response locally. Missing strict
attribution remains a quality gap. The curator package must show four draft
stories, computed geometry and unresolved tasks, with publication disabled. Its
private response remains unavailable to visitors. The browser transport fixture
`review-package.fixture.json` comes from this real replay, with repeated draft
body strings removed and restored from `stops.json` by the browser test. It
contains disposable generated identities, no account credentials or session
material. Browser authentication/brief loading use the disposable API; the
captured package is intercepted transport data, with database behavior checked
independently. A synthetic arrival correction
uses the real proposal/review controls, retains unknown access and leaves the
historical location unchanged. It is a software exercise and must never be copied
to production as an actual entrance approval.

A curator must still reconcile the named source conflicts, inspect current
entrances and crossings, establish allowed public viewpoints, complete histories
and image rights, and obtain current passing route assessments before considering
public release. Actual publication remains an explicit curator action.
