# Curator review package

`GET /api/v1/admin/route-briefs/{id}/review-package` returns one private review
response. Only curators and admins may read it; owning the brief grants no access
to source passages or candidate payloads. Responses use `Cache-Control: no-store`
and `Vary: Authorization`.

Select a retained `run_id`, or omit it for the latest run. An optional `tour_id`
selects an existing curated working tour for geometry and path-quality inspection.
The prepared tour must match the retained proposal's ordered places, selected
moments, modes and trip shape. A mismatched tour remains inspectable with tasks;
an invalid tour or a run belonging to another brief returns 404. Personal routes
have a separate owner workflow and are not eligible for package publication.

The package includes:

- The current private brief, frozen run input, research stage outcomes and retained
  itinerary, including exclusions, uncertainty and source/candidate identities.
- Full selected visitor stories when their captured content fingerprints still
  match, or a clearly labeled captured partial paragraph when material is stale.
- Required place, moment and media dependencies, separately from contextual leads.
  Existing published content, unpublished drafts, new candidates and proposed
  shared/draft edits have distinct dispositions. Proposed public fields show
  before/after values; generated proposals are labeled as unapplied.
- Retained evidence passages and retrieval outcomes, plus prepared route geometry,
  per-mode journey estimates and private path inspection reports when available.
- Named blocking tasks for missing evidence, changed identities/stories, incomplete
  historical/media requirements, unprepared or unusable paths, duration overruns
  and a return shape that does not match the proposal.

A brief without a run returns an inspectable `not_started` package. Active work is
`processing`; incomplete work remains `needs_research`. Changed captured inputs
produce `stale`. Only completed current research in curator review, with all
selected dependencies and usable matching routes, can be `ready_for_review` with
`ready_to_publish: true`. This is eligibility for a later explicit approval,
not an approval, factual certification or current access guarantee. Required
pending candidates must first become approved draft entities; ordinary review
never approves or publishes contextual leads.

Each response carries `schema_version: 1` and a deterministic SHA-256 `version`
over the complete response, excluding that token itself. The read uses one
read-only repeatable-read PostgreSQL transaction, so fields share a coherent
snapshot. Supplying `expected_version` makes any changed read set return 409.
The version is not a write lock: the separate publication transaction must
reconstruct and fence the exact reviewed package under appropriate write locks.

Bounds are 20 reconciliation records, 64 research-stage candidates, 64 source
snapshots and 8 MiB serialized. Larger sets return named limits or a 422 requiring
a narrower package. Existing per-record endpoints remain available for deeper
inspection. A completed source retrieval does not establish image-reuse rights;
media and history publication checks remain separate.

The GET makes no provider requests, approves no candidate, creates no entity or
package row and changes no publication. Schema 16 and its migration manifest/hook
are unchanged. Atomic package approval is issue #37; the unified web workspace is
#38. Production sign-in, generation and a qualified routing provider are not
activated by this read contract.
