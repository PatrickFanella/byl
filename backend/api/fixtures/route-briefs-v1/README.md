# Private route brief fixture

`brief-response.json` is a synthetic owner-only suggestion in `needs_research`.
Its topic, unresolved place name and source link are unverified input, not source
evidence or a real approved route. The account and record UUIDs are fixture IDs.

The Go model test validates the shared contract. Android's `route_suggestion.json`
is a byte-identical copy checked by its core tests. Native status, retry and
ownership tests use this record without needing a sign-in provider or live source.
