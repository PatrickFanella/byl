# Backend-emitted scheduled-transit fixture

Generated November 8, 2026 from the real public revision and transit-plan HTTP
handlers at main `2d501158720f31fde4a8c910e34734cd980c3cf3`, in a temporary
PostgreSQL `museum_test` database. The existing `newTransitHTTPFixture` and
`transitFixturePlanner` supply synthetic reviewed choices, walking results and
scheduled service. No external CTA archive was fetched and no real train,
station walk or accessibility qualification is claimed. The repeated-a source
hash is a deliberate synthetic provider value, not a real archive hash.

The plan response is retained semantically unchanged. The revision response has
only its unrelated `snapshot.moments` content replaced with an empty object to
avoid duplicating story bodies; stops, approved choice, walking legs and route
metadata remain as emitted. UUIDs and timestamps are synthetic test artifacts.
Tests use recorded context, not today's clock, for validation.

Original complete wire-response SHA-256 values:

- Revision: `a077d7c3f3fd1e6dc06c9c690a78ac4135c461feff15a486b9e7c2d5749d4eed`
- Plan: `7fe2dc733f28b2918f62c68b0e4d229ac42160decaa9341902d760ab330a28cb`

This corpus checks the client against the actual Go response rather than only
a hand-authored browser mock, including optional stop IDs, projected walking
seconds and RFC3339 timestamp formatting. The temporary export test and database
were removed after generating the fixture; the ordinary backend source is unchanged.
