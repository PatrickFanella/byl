# Route readiness and trip estimates (#34)

Extend the read contract without changing route computation, published snapshots,
publication gates or the legacy travel-only fields. Use the existing quality
checkpoint and selected computed geometry. Missing proof stays explicit.

1. Add a versioned public route explanation with separate computation/quality
   readiness, nullable travel metrics, stated dwell/narration assumptions and an
   explicit included or unplanned return. Never substitute straight-line estimates
   for a usable path. Summaries retain their existing status and travel fields.
2. Project approved reference segments (trail/road connector, permitted modes,
   sourced surface when recorded, and public reference metadata) without private
   inspection IDs, passages, authors or provider attribution. Label reference
   information separately from selected computed geometry and metrics.
3. Apply the projection to working tour/route reads and immutable revision reads,
   using each revision's own stops, selected stories, routes and corridor. No joins
   to newer working content for retained trips. No database migration is needed.
4. Add API/PostgreSQL checks for passing, unreviewed, stale, pending, failed and
   missing modes; separate totals and return legs; frozen revision behavior; and
   private-evidence filtering. Add shared JSON fixtures and web/Swift/Kotlin decoder
   checks, including legacy fixtures. Downstream interface redesign stays outside
   this issue.
5. Run backend race tests with real isolated PostgreSQL, vet/build and changed-code
   lint; web check/lint/unit/browser checks; relevant native contract/build checks;
   require all five hosted jobs before merging and qualify the actual release.

Travel is the selected provider result, not a field timing measurement. Dwell and
narration are assumptions, with unavailable narration remaining unknown. A route
quality pass means eligible for curator review, not publication/access approval.
Open itineraries with no computed return show an unplanned return and cannot claim
an overall round-trip estimate. Transit estimates and new itinerary materialization
are outside this issue.
