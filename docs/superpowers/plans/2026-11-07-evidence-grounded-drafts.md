# Inspected evidence and protected historical drafts — issue 30

One PR changes moment drafting to use 1–5 immutable inspected evidence records.
Raw candidate text and submitted anecdotes are not model evidence. Bound the
offered passages to 12000 runes and retain their IDs, references and source hashes.

1. Extend the existing provider contract with paragraph claims, exact passage
   citations, title/date citations and visible uncertainty. Strictly decode
   provider output, reject invented references/quotes and uncited prose, retain
   attributed testimony, and leave conflicting dates unresolved. These checks
   validate structure and traceability; a curator still checks historical meaning.
2. Capture the candidate payload hash when queuing. Save bounded private proposal
   history only while its pending payload still matches, using an atomic compare.
   Never replace curator title/body/dates/sources or create public entities.
3. Let the queue page select inspected sources from a brief and show a generated
   proposal separately. Applying it to the editor is explicit; polling must leave
   unsaved edits intact. Media and narration remain separate optional review work.
4. Verify invalid citations, conflicting dates, testimony, source instructions,
   unsupported anecdotes, concurrent edits/review and reruns with deterministic
   providers and PostgreSQL. Verify the editor flow in the browser, then run the
   matching backend/web checks and require all hosted jobs on the PR head.

Reuse source_evidence and candidate JSON; no schema change or new production
provider is required. Legacy draft jobs without inspected references must fail
without changing their candidate. Keep schema 14 and existing public content.
