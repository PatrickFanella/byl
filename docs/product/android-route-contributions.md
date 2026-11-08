# Optional Android route contributions

Issue #52 adds a separate choice for a frozen active trip, revision and walk/bike
mode. Collection stays behind the existing default-off server gate. Navigation,
progress and narration do not depend on consent or contribution IO.

## Observation and lifecycle

The Kotlin detector uses the unchanged 26-case shared reference fixture. Only
explicitly reviewed signal references in an allowed, usable retained journey
reach the detector. Transit, malformed observation metadata and unsupported
geometry disable contribution eligibility without invalidating navigation.

The trip sheet offers explicit consent and Pause/Resume. A restored controller
always starts paused. Summary uploads also require an explicit choice or Resume
on the matching active trip; Profile can retry deletion without uploading summaries. No new location permission or background service is added.
Only fresh authorized fixes from the existing foreground source can enter the
detector, after durable consent and a fresh server recheck. App/screen departure,
arrival, permission loss, identity/server changes, revocation, revision replacement
and trip end discard the current raw episode. A modal trip review/report or end
confirmation also suspends observation. Returning requires a fresh gate/consent
check; manual arrival and explicit pause require explicit continuation. Gate and
receipt checks expire after 60 seconds, with a 30-second foreground refresh.
Cached fixes preceding the choice cannot start a passage.

Raw Fix and Point types have no serializer. Only the 10-field reference summary
can be queued or uploaded. Contributions are uncalibrated, pseudonymous counts;
they do not establish distinct people, legal access, closures or route safety.

## Private retry and deletion

The app supplies an application-private `noBackupFilesDir/route-contributions`
leaf, independent of the navigation cache. One strict JSON file is limited to
128 KiB, 16 retained choices across all origins and 32 summaries per choice.
Writes synchronize the temporary file, atomically replace the destination and
read it back. Storage errors stop collection and uploads; there is no cache or
memory fallback that loses erasure capabilities.

The original choice and idempotency key are saved before requesting consent;
the receipt/capability is saved before observation. Exact retries preserve the
body, key and original expiry. Queues expire physically on rewrite at midnight
seven UTC days after observation, before upload. Consent receipts expire on their
original server deadline; lost receipts remain recoverable for at most 31 UTC
days from the original requested date. Bearer tokens and exact trip times are
never persisted here.

Profile exposes checks/retries and global Stop and delete after sign-out too.
Global deletion invalidates pending choices across controllers, clears local
queues before networking and withdraws each retained capability at its original
server without a bearer token. Late grant receipts remain available for deletion.
Unknown receipts need the original account/server to recover the same choice;
404 or failed withdrawal stays pending, rather than claiming deletion. Other
devices and lost handles need separate handling. The [server contract](route-contributions.md)
describes retention, scope deduplication and deployment custody.

## Evidence and qualification limits

Tests cover the shared fixture, UTC expiry, lifecycle/identity controls, consent
and summary retries, storage limits/failures, minimal payload keys, anonymous
withdrawal to a different server and deletion during in-flight requests. Exact
local test/build/lint counts and hosted checks are recorded in the PR.

Native rendering and actual Android sampling are **unverified**. The earlier
owned emulator disconnected; the prescribed device CLI retry also reported it
missing, and the T3 tools subsequently rejected calls because the provider no
longer owns an active run. No manual emulator UI fallback was used. Before
activation, use a supported device session to inspect large text/accessibility,
sampling gaps, foreground/background/permission transitions, interrupted
network retries, minimal request bodies and signed-out erasure. Emulator playback
is separate from real-route calibration in #63. Production collection remains
disabled.
