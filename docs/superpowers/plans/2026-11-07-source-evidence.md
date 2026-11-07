# Retained source evidence — issue 24

Deliver one PR after the private brief/job foundation. Actual source discovery
is issue 26; historical drafting and the curator package workspace follow later.

1. Reuse the public-IP connection guard in a bounded evidence retriever. Reject
   unsafe URLs/redirects, cap the entire request time/body/excerpt, and retain
   only untrusted UTF-8 HTML/plain-text passages. Test private DNS, redirect
   targets, timeout/size limits and missing rights with deterministic providers.
2. Add private immutable retrieval snapshots and curator-only queue/read APIs.
   Serialize quota/idempotency checks with the brief, retain source identity and
   the original brief version, and use the existing durable worker with claim
   fencing. Test authorization, retries, interruption, stale workers and restored
   worker state against isolated PostgreSQL. A failed retrieval is an inspectable
   gap; it is never successful full-source evidence.
3. Update OpenAPI, intake and operator migration guidance. Run the backend suite
   with PostgreSQL/race detection, vet, formatting, API contract/type generation
   and web compatibility checks. Require hosted CI on the PR head and clean
   mergeability. Rehearse/admit migration 8 separately before any production
   deployment; image rollback retains evidence. Do not activate public intake,
   research discovery, model providers or automatic publication in this unit.
