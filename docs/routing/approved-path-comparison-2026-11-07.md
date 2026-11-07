# Bounded approved-path adapter comparison

On 2026-11-07 the development adapter was compared with one 589.698 m section of
the Illinois Prairie Path Elgin Branch. This establishes support for the tested
break/via request and returned geometry on that section. It does not establish
current public access, crossing topology, corridor-wide quality or field behavior.
Production routing was not changed.

The reference came from the public
[DuPage County DuDOT bikeways layer](https://gis.dupageco.org/arcgis/rest/services/DuDOT/Bikeways_MapService/MapServer/0).
A bounded query selected existing Illinois Prairie Path records in a small area
and returned three features. The comparison used OBJECTID 114, consecutive vertices
0–7. The source response was retrieved at 13:05:33 UTC with SHA-256
`6f1832c2328eaac8d9a3d868b5b1cf7d18bcfcbd6801fc97be50f02a80cc078e`.
Raw reference geometry and provider responses remain in private qualification
artifacts; they are not a licensed public import. No reference rights or access
status was inferred from the layer's availability.

The app's `BuildPlan` and controlled Valhalla adapter sent eight points: story
breaks at reference vertices 0, 4 and 7, with five via controls including an
explicit shape at vertex 2. Synthetic approval objects supplied those entrances
for protocol testing; they were not curator decisions or database writes. The
[Valhalla routing contract](https://valhalla.github.io/valhalla/api/route/api-reference/)
distinguishes story `break` locations from non-leg-creating `via` locations. Via
permits a U-turn, needed by out-and-back traversal fixtures.

One request per mode was sent to the public FOSSGIS Valhalla demonstration service
using a development qualification User-Agent/client identifier, with requests
spaced more than one second apart. The service is identified by the
[Valhalla project](https://github.com/valhalla/valhalla#demo-server); the
[service policy](https://routing.openstreetmap.de/about.html) limits request rates
and heavy use. This bounded comparison does not configure that service for app
traffic.

| Adapter result | Walk, 13:34:06 UTC | Bike, 13:34:30 UTC |
| --- | ---: | ---: |
| Story legs for three story boundaries | 2 | 2 |
| Provider distance | 590 m | 590 m |
| Provider travel time | 435 s | 143 s |
| Largest story endpoint error | 1.007 m | 1.007 m |
| Largest returned-vertex distance to reference segments | 1.500 m | 1.500 m |
| Largest reference-vertex distance to returned segments | 1.267 m | 1.267 m |

Lengths use WGS84 haversine distances. Vertex-to-line deviations use a local
projection onto each segment followed by haversine distance to the projected
point. Both directed vertex comparisons were measured; these are not a continuous
Hausdorff-distance or network-topology proof. Duplicate story-leg boundary
coordinates do not affect the measurements.

An earlier direct request with all 16 source vertices failed explicitly with
HTTP 400, `Exceeded max locations: 10`. The later comparison chose a contiguous
bounded section; the app never retries a failed request by silently dropping
controls. Its own 256-control ceiling does not override provider configuration.
Synthetic CI fixtures cover omitted controls, wrong leg counts, missing geometry,
restricted/missing modes, ordered shapes, loop closure and full out-and-back
distance. They make no claims about a real trail's current access or legal status.
