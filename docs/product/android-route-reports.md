# Android route-change reporting

Tour detail offers a reporting action for each retained mode. A pinned active
trip offers **Report** with its next entrance initially selected. That sheet uses
the trip's cached revision, including offline, and keeps the existing explicit
revision-update flow. Legacy trips without a retained revision do not expose
reporting.

Choose a retained stop, entrance or actual path leg. Explanation-only reports
need no replacement. Optional fields accept a source link, observation date,
direction, expected duration, one explicitly entered problem point or a local
alternative of 2 to 64 longitude/latitude pairs. No navigation fix, raw journey
or progress history is attached. The server validates the references and bounds
the points to the selected area.

## Input, retries and private status

`RouteReports` is bound to its opening account, credential and API origin.
Account, token or origin changes dispose the screen controller; late responses
cannot populate the replacement screen. Lists and detail responses check the
owner ID. A foreign or unavailable report stays unreadable.

Drafts use `noBackupFilesDir/route-reports` through the checked atomic file store.
Keys include an origin hash, account, revision and mode. The original fields and
exact pending body/key are written before the POST. If that write fails, no
report is sent; the attempt remains in memory for retry. A lost response locks
the fields and restores the same body/key when the controller is recreated.

Validation rejection unlocks the fields only after persisting the cleared
attempt. Failed discard or cleanup retains the prior identity. Cancellation
during confirmed-response cleanup propagates without showing success and leaves
the persisted retry available. A 401 keeps input and requests sign-in. Discard
removes only local input; it does not cancel a report already saved by the server.

**Your routes → Your route reports** lists bounded pages of private submissions.
Open a report for the original observation, current disposition and curator's
reason. A correction-outcome 404 means no release has been recorded yet. A failed
outcome read keeps the report visible and offers refresh.

An approved outcome offers explicit review. In a started trip, the report sheet
opens the existing Trip review and checks for an update; the visitor still
chooses and confirms a revision switch. Other report screens link to tour detail
and explain how to review an active trip. The reporting controller never calls
trip adoption. A temporary review date requests reassessment and does not imply
that a closure reopened.

## Verification and limits

Kotlin tests cover immutable references, explicit closing returns, mismatched
IDs, report-only payloads, optional points, exact lost-response replay, disk
failure, failed discard/validation cleanup, cancellation, corrupt drafts,
account/server/revision/mode isolation, 401s, duplicate taps, bounded pages,
private decisions and correction-source identity. MockWebServer fixtures prove
client behavior; they do not replace the backend's actual PostgreSQL contract
tests or live qualification.

Run `:core:test`, `:app:assembleDebug` and `:app:lintDebug` with the Android SDK
before merging. Native interaction remains unverified: the Kvant device tool
lists the Android 15 emulator with sufficient host space, but its open operation
fails the provider's active-run ownership check. The selected-entrance, resumed
form and decision walkthrough needs that device path restored. No manual device
control was used.

This client adds no migration. Intake requires the schema-13 report API; approved
outcomes require schema 18. Production admission and field checks remain the
separate gates described in [route corrections](route-corrections.md).
