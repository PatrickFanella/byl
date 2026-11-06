# Private route brief development

Implement issues #18, #19 and #20 as separate PRs. Each layer must pass its own
checks and remain useful before later research adapters exist. Public submission
and automatic publication stay closed throughout this batch.

1. **#18: persist private briefs.** Add the submitted input, explicit lifecycle,
   reasons, optimistic versions, owner-scoped reads, curator writes, request
   limits and idempotent creation. Check minimal intake, field errors, ownership,
   role checks, duplicate concurrent requests, quota races and stale decisions
   using the existing HTTP/repository tests against PostgreSQL. Document the API
   and additive migration 6. Rehearse migration against a restored production
   copy before operator admission; application rollback retains private data.
2. **#19: curator workspace (depends on #18).** Add one navigation entry and a
   form for saving/resuming briefs, optional leads and travel preferences. Show
   the API lifecycle, unresolved work and reasons. Verify API field errors,
   saved edits, role gating, keyboard navigation and labels with Playwright,
   then run the web checks. Do not build tour-package review here.
3. **#20: resumable orchestration (depends on #18).** Extend the existing durable
   job worker with per-brief stage checkpoints, bounded work, progress,
   cancellation and stale-worker recovery. Use stubbed stage providers to test
   interruption, retry idempotency, exhausted budgets and preservation of curator
   edits. Source discovery and adapters are separate roadmap issues; incomplete
   research must end with a named needs-research reason, never public content.

Hosted checks on each PR head and clean mergeability are separate evidence from
local tests. A production claim additionally needs exact image/source readback,
the admitted migration baseline and public runtime checks. Curator sign-in is
still deferred until Google OAuth is configured; no development auth is enabled
on production for this work.
