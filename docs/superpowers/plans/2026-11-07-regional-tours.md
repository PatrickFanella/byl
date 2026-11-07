# Regional tours and shared topics — issue #16

Delivery: one PR. This supplies the municipal model required by reconciliation
issue #28; it does not implement reconciliation or route-version generation.

1. Retain a tour's existing `city_id`, ID and slug. Derive `city_ids` from that
   original city and each stop's unchanged place municipality. Permit cross-city
   stops only for curated tours. Use existence queries for city discovery so a
   tour with several local stops appears once; retain visibility filtering.
2. Add `topic_cities`, backfill existing topic references and expose explicit
   topic coverage. Preserve original city/slug references, use shared IDs across
   municipalities and reject ambiguous shared slugs. Validate coverage and lock
   it against concurrent moment/tour writes. Keep failed edits atomic.
3. Verify repository/API cases for cross-city success, unknown references,
   featured-moment ownership, shared themes, visibility, stable legacy fields,
   personal-route restrictions, concurrent edits and seed compatibility. Send
   topic IDs from the web filter and verify same-slug themes in the browser.
4. Restore a fresh production snapshot locally, apply migration 9, compare all
   existing table fingerprints and representative public responses using the
   currently deployed API. Confirm automatic down refuses to remove associations.
5. Run backend race tests, vet, changed-code lint, OpenAPI generation and web
   checks. Open one PR that closes #16 and require hosted checks on its head.

Migration 9 is additive. Do not admit or apply it on production before source is
accepted. After merge, rehearse the accepted bytes, take a fresh database/media
backup, apply as operator under the deployment lock, then install the exact
nine-file manifest and version-9 hook. CI continues to apply no production SQL.
Image rollback retains the expanded schema and regional associations.

The existing visitor builders and native models remain compatible readers; they
do not gain regional editing controls in this PR. Regional route/access quality,
transit and field/device qualification remain separate work.
