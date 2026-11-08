# iOS route-change reporting

Tour detail offers a report action for each retained travel mode. A signed-in
active trip offers **Report**, initially selecting its next entrance. The form
uses that trip's exact cached revision, including offline; it does not substitute
the latest tour. Legacy trips without a revision do not expose this action.

Choose a stop, entrance or actual retained path leg and describe the problem.
An explanation-only report is sufficient. Optional fields accept a source link,
observation date, direction, expected duration, one explicitly entered problem
point or a local alternative of 2 to 64 longitude/latitude pairs. No location
service or progress history is attached. The API checks the reported references
and bounds the points to the selected area.

## Pending input and account boundaries

`RouteReportsModel` captures the opening account, credential and API origin.
Changing any of them invalidates late responses and replaces the screen's
controller. Owner IDs are checked on submission, list and detail responses. A
foreign or unavailable report stays unreadable.

Draft files contain the unfinished fields and exact pending request/key. Their
names include an origin hash, account, revision and mode. They use the existing
private cache writer, are excluded from device backups and use iOS file
protection. No report is sent until this write succeeds. A failed write keeps
the attempt in memory and clearly says that no report was sent.

An uncertain response locks the input. Closing and reopening the form restores
the same request body and idempotency key. Validation rejection unlocks the form
only after persisting the cleared attempt. Failed discard or cleanup keeps the
prior input and retry identity. A 401 retains input and asks for sign-in; it never
retries with another account's credential. Discard removes only local input,
not an observation already retained by the server.

## Decisions and explicit updates

**Your routes → Your route reports** lists private reports in bounded pages. Open
a report to read the original observation, current disposition and curator's
reason. Report and correction-outcome reads are separate: no correction yet is
normal, while an outcome-read error keeps the report visible with a retry.

An approved outcome identifies a new public revision. The reporting controller
does not mutate an active trip. From an active trip's report sheet, **Review route
update** closes the sheet and opens the existing Trip review. The visitor checks
and explicitly adopts a newer revision through the pinned-trip flow. Other
report screens link to tour detail and explain how to review a started trip.
Temporary review dates request reassessment; the client does not infer reopening.

## Verification and limits

Swift reporting tests cover retained stop/entrance/segment references, explicit
closing returns, mismatched IDs, report-only payloads, optional coordinates,
exact retry after recreation, disk failure, failed discard/validation cleanup,
unreadable drafts, 401s, account/origin/mode scope, bounded paging, owner-only
decisions and correction-source matching. Existing pinned-trip tests cover
explicit revision adoption independently.

Package tests and an unsigned iPhoneOS build are required before merging. They
do not establish simulator interaction, live API compatibility or real-world
route safety. The current Kvant device tool lists an Android emulator and no
available iOS simulator; its open operation also fails the provider's active-run
ownership check. The requested entrance/resume/decision simulator walkthrough
remains unverified. No manual device-control fallback was used.

This client adds no migration. Reports require the retained-report API from
schema 13; approved outcomes require schema 18. Production admission and live
qualification remain the separate gates documented in
[route corrections](route-corrections.md).
