# Tour revision API fixtures, version 1

These JSON responses were captured from the isolated HTTP test harness with
development accounts and the seeded catalogue. The trip and its straight-line
routes are synthetic fixtures, not a real visit or a public-access route review.
The files contain no session credentials.

- `revision-1.json` is the initial two-stop release, with full selected stories,
  sources, media references, routing intent and walk/bike geometry.
- `revision-2.json` is a later release of the same trip with the stops reversed.
  The stop UUIDs remain stable while their positions change.
- `progress-1.json` records a visit to the first release's first stop. It stays
  scoped to that revision and stop UUID; clients must not copy position 1 into
  the next release.

The existing native fixtures continue to cover older responses without revision
fields. `TestRevisionClientFixtures` checks decoding and the relationship between
the retained visit and both releases in Go.

The iOS and Android revision clients use byte-identical test resource copies. Their tests
explicitly add synthetic usable-path and arrival assertions when exercising
start/update behavior. Those test assertions do not qualify the captured
straight-line routes or establish public access.
