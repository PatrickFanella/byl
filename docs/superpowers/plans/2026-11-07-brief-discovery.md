# Brief discovery — issue #26

Delivery: one PR on the merged source-evidence implementation (#70).

1. Select published places and moments in the requested area before requesting
   sources. Reuse retained brief evidence where the source policy permits it.
2. Inspect configured starting pages, follow a small reference frontier, and
   coordinate Wikipedia text search with the existing LOC and Internet Archive
   adapters. Persist an allowance before each HTTP hop; preserve the existing
   run deadline, claim fence, stage retry limit and public-address checks.
3. Save a private shortlist with query, provider, source, relevance reasons and
   evidence classification. Deduplicate normalized source URLs and collection
   IDs. Reject results without explicit geography and topic or named-lead support.
4. Verify founders, similarly named trails, unrelated hits, duplicate sources,
   collection reuse, partial failures, redirect accounting and durable recovery
   using fixtures and PostgreSQL. Run the backend race suite, vet and changed-code
   lint, then hosted CI on the PR head.

No schema migration or public API change is required. Discovery does not reconcile
entities, generate historical prose, assemble routes, fetch media or publish.
Those stages remain paused until their roadmap issues supply processors. An
image rollback can retain the additive schema introduced by #70.

Source inspection: the Downers Grove Historical Society entry point exposes
research, biographies, buildings, timeline and history-walk resources. The two
Wikipedia overview pages expose history and references. These are starting leads,
not evidence of route access or permission to copy photographs. The bounded new
mechanism uses the documented MediaWiki `list=search` API:
https://www.mediawiki.org/wiki/API:Search.
