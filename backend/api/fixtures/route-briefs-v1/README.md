# Private route brief fixture

`brief-response.json` is a synthetic owner-only suggestion in `needs_research`.
Its topic, unresolved place name and source link are unverified input, not source
evidence or a real approved route. The account and record UUIDs are fixture IDs.

The Go model test validates the shared contract. Android and iOS use byte-identical
`route_suggestion.json` resource copies for their core/package tests. Native status, retry and
ownership tests use this record without needing a sign-in provider or live source.
