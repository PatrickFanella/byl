# Durable route corrections

Issue #45 adds an explicit correction release to the curator API. Reports remain private observations. An investigation computes a comparison and retains gaps; it grants no authority to publish its proposed line.

## Prepare and inspect

1. Investigate the report through the existing retained investigation API. Its selected report versions must still match, and its original tour revision must remain the current published release. After another release, submit/investigate an observation on that release instead of approving an old comparison.
2. Prepare the working tour through existing arrival, corridor and content review actions. Approve a persistent corridor with an explicit selection for the reported travel mode. Resolve access/network findings and recompute every required mode. This API never inserts a reported alternative, copies a generated history, or silently approves an entrance.
3. Read `GET /api/v1/admin/route-reports/{id}/correction-comparison?investigation_id={investigation}`. The response includes the retained original revision and investigation, the complete prepared snapshot and stories, current private path-quality reports, blockers and potentially affected tours. Inspect the original investigation proposal alongside the actual prepared path; a curator adjustment can differ from the submitted line.
4. Approve only after `ready_to_publish` is true, using the exact returned `version`:

Send `POST /api/v1/admin/route-reports/{id}/corrections` with the returned `version`, selected `investigation_id`, a decision `note`, and optional `temporary`/`review_on` fields. The note becomes the report owner's decision reason. A permanent correction omits `review_on`; a temporary correction needs a future UTC calendar date within one year on first approval. Request bodies allow at most 16 KiB and reject unknown fields.

## Release and compatibility

The comparison uses one repeatable read. Approval rechecks its full read set under serializable isolation while locking the tour and report. Changed report versions, published pointers, draft content, geometry, current path evidence or shared-reference impacts invalidate approval. Existing atomic publication checks still enforce public dependencies, exact route inputs and passing path quality.

One transaction creates the new immutable tour revision, advances its published pointer, records retained private correction provenance and accepts the seed report. A late failure rolls all of these back. Replaying the exact body by the same curator returns the original result, including after its review date passes. Another body or reviewer cannot reuse that approval. Related reports keep their own dispositions.

The retained corridor contains reviewed crossings, connectors and shaping controls. Recompute consumes that intent through the existing controlled-routing provider. An unsupported provider, invalidated anchors or failed path inspection leaves a visible pending/failed/blocked route; it cannot silently publish an unconstrained replacement. Recompute does not rewrite the corrected or previous published revisions, and revision-scoped visits stay on their original release. Visitors choose an explicit update through their existing client flow.

The owner reads the accepted status and decision reason through the existing report endpoint. `GET /api/v1/me/route-reports/{id}/correction` returns only the old/new revision IDs and temporary review context. Other accounts receive 404. It does not expose investigators, private source snapshots or other-tour identifiers. All correction endpoints return `Cache-Control: no-store` and `Vary: Authorization`; curator reads/releases require the curator role.

## Reassessment and shared references

`GET /api/v1/admin/route-corrections?due=true` lists retained releases whose review date has arrived. `reassessment_required` is a task flag. The recorded detour and date stay in history; the system never infers that a closure reopened. A new source/access review and explicit release are required for a changed route. The list accepts bounded `limit`/`offset` pagination and retains historical due dates for audit.

The comparison flags other current tour revisions sharing the affected stop's place identity or a selected reviewed OSM-way identity. These are potential impacts requiring manual assessment, not inferred geometric connections or evidence of a closure. A corrected segment does not modify another tour, and impact flags remain private. More than 200 matching tours blocks release so the review stays bounded.

## Validation and rollout

Repository tests use real PostgreSQL to cover stale drafts, published revisions, report dispositions and route quality; concurrent approval and exact replay; previous snapshot retention; late rollback; immutable provenance; persistent intent after recompute; temporary-date expiry without reopening; and shared-stop impact without another publication. HTTP checks cover curator authorization, owner/foreign outcomes, cache headers, stale approval, date validation and public/private boundaries. These synthetic persistence fixtures do not establish real-world access or field readiness.

Migration 18 adds only `route_corrections`, its review-date index and a retention trigger. It changes no previous rows or route-input hash; a forward-migration test checks those invariants. Automatic down migration is refused. Application rollback retains correction records and both revisions. Investigation-job rollback constraints from migration 17 still apply.

The repository hook expects schema 18. Production must first admit migrations 17 and 18 with a fresh database/media backup, an independently restored forward-migration rehearsal, matching pre/post row fingerprints and reviewed installed manifest/hook. No production admission or device/field qualification is implied by this source change. Web/native reporting and comparison screens remain issues #47–#49.
