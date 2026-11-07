# Immutable tour revisions — issue 17

One PR delivers the backend revision, progress and route-job foundation. Client
update prompts, route reports and telemetry remain separate issues.

- Keep the existing tour/stops/routes as the working draft. Persist draft and
  routing-input versions separately; metadata edits need not recompute geometry.
- Publish immutable snapshots of tour/stops, selected full reviewed stories and
  sources/media references, arrival context, routing intent and full routes.
  Compare expected draft version and current published revision under a tour lock.
- Curated publication remains explicit and checks existing publication blockers.
  Editing a published working copy does not replace its public snapshot.
- Legacy personal-route save/share requests retain their existing release intent.
  Persist that request for the exact draft version; fulfill it after all routes
  are ready. Superseded jobs/requests cannot release a different version.
- Backfill existing published tours and map legacy position progress to the stop
  IDs in their initial snapshot. Retain original progress rows for recovery.
  Legacy active trips pin a revision until explicit reset; new APIs use revision
  IDs and stable stop IDs. New revisions do not inherit visits by position.
- Preserve stop IDs across reorder when place/featured-story identity matches;
  explicit stop IDs disambiguate repeated visits. Removed stops remain in old
  snapshots. Published snapshot rows reject update/delete.
- Public latest-tour and route responses use published snapshots. Owner/admin
  editing endpoints expose working data and optimistic version fields. Explicit
  visibility revocation/deletion still controls access to retained content.
- Version route jobs at enqueue/read/commit and final-failure handling. A stale
  worker cannot overwrite routes after new inputs or recomputation.

Verify snapshot immutability, content/geometry retention, publication/edit races,
old-client latest views, progress across reorder/reset, stable IDs, stale workers,
privacy and migration/backfill on isolated PostgreSQL. Require all five hosted
jobs and a clean merge. Admit production SQL only after accepted-source rehearsal,
fresh backup/restore, compatibility checks and an exact manifest/hook review.
