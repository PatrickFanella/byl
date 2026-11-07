# Complete curator review package (#36)

1. Add one curator-only GET on a brief, with optional retained run and prepared
   working-tour selection. Keep processing, no-run and needs-research packages
   inspectable. No approval, materialization or UI enters this issue.
2. Build the entire read in one read-only repeatable-read PostgreSQL transaction.
   Return the retained visitor proposal, frozen submitted brief, current linked
   identities/candidates, selected story/place/media dependencies, field-level
   proposed shared changes, retained passages and prepared geometry/quality.
3. Label required versus contextual entities. Never mistake an unselected lead
   for permission to publish it. Missing/stale evidence, identity changes, invalid
   media/history, mismatched tour stops or unavailable route modes become tasks.
4. Hash the complete deterministic response (excluding the hash itself) into an
   exact review version. Optional expected_version returns 409 after any relevant
   dependency changes. A token grants no publication authority.
5. Add real-PostgreSQL handler/repository tests for complete and partial packages,
   missing proof, shared-content differences, stale dependencies/versions, foreign
   run/tour selection and all non-curator denials. Verify GET creates no rows and
   performs no external requests. No schema or deployment-hook change is needed.
6. Run backend race suite, vet/build and changed-code lint, OpenAPI generation and
   web checks; require all five hosted jobs before merge. Continue #37, #38, #42
   and #57 under the existing merge authorization after release verification.

Production sign-in/model/routing configuration stays deferred. Ready means a
complete package can enter explicit curator approval, not automatic publication
or field/access qualification. The later approval transaction must reconstruct
and recheck the exact version under write fences; a prior GET is no write lock.
