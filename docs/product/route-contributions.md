# Voluntary route contribution ingestion

Issue #23 adds a backend contract for minimal, explicitly uncalibrated route
summaries. It does not connect navigation or GPS to an uploader. Client consent,
detector adapters and opt-out controls belong to #50–#52; aggregation and field
qualification remain #53 and #63. Collection is disabled by default and must
stay off while backup, restore or snapshot custody is unresolved.

## Consent and accepted data

`GET /api/v1/route-contribution-intake` reports the effective server gate. Browsing,
navigation and progress do not create consent. An authenticated visitor makes a
separate version-1 choice through `POST /api/v1/me/route-contributions/consent`,
for one tour, immutable revision and travel mode. The request includes an explicit
`accepted: true` and the UTC date of that choice. A new consent accepts today or
yesterday; an exact retained retry can recover the original response until its
original expiry. Changed input with the same `Idempotency-Key` conflicts.

The response contains a random consent ID, a 256-bit deletion capability and an
expiry date. Clients must save the original choice and retry key before sending,
then save the capability in private storage excluded from device/cloud backups.
Keep it available after sign-out. Do not put capabilities in URLs, analytics,
screenshots or logs. Losing the capability removes signed-out erasure access;
the retained rows still expire. Never silently create consent again after
withdrawal, expiry or a lost local record.

`POST /api/v1/me/route-contributions` accepts a consent ID and exactly the
[reference detector's minimal summary](deviation-detector-v1.md): schema 1,
`detector_version: uncalibrated-v1`, `calibrated: false`, retained revision and
logical segment, walk/bike mode, forward/reverse direction, one UTC observation
date, `qualified_passages: 1` and `sustained_departure_rejoins: 0 | 1`.
The segment and direction must occur in a usable retained journey with allowed
access. The date must be within the last 30 UTC dates and no earlier than the
server's consent date. This coarse date check does not prove when consent
preceded a passage; clients must enforce consent before collecting samples.

Both POSTs require a 16–128-character printable, space-free idempotency key.
Bodies are limited to 4 KiB. Unknown fields, raw coordinates/traces, exact trip
timestamps, extra JSON objects and query parameters are rejected without echoing
the submitted values or unexpected field names. Responses use `no-store` and
vary on Authorization. The API does not log request bodies or capabilities;
operators must also prohibit body/header/query capture at proxies and collectors.

## Scoped deduplication and withdrawal

The server derives an HMAC scope from the authenticated account, tour, retained
revision and mode. The stored rows contain the opaque scope, with no account,
device or trip identifier and no precise receipt/trip time. The scope is stable
only within that route/revision/mode. It is pseudonymous, not anonymous: an
operator holding the secret and account database could derive it.

An exact summary-key/body replay returns the same receipt and original expiry
without consuming quota. Changed input conflicts. A second key for the same
scope, logical segment, direction and UTC date returns `already_contributed`,
including across overlapping consents. An expired retained receipt cannot be
replayed through a newer active consent. Per account, new grants are limited to
10 and new summaries to 50 per UTC day. Exact retries are free. Abuse counters
use a separate day-scoped HMAC, store no route/account identifier and expire
after two UTC days. These limits are not proof of distinct people.

`POST /api/v1/route-contributions/withdraw` authenticates the consent ID and
deletion capability in the JSON body. It works after sign-out and with collection
disabled, independently of the scope secret. The first valid call deletes all
summaries and revokes all currently existing grants in that scope. An exact
withdrawal replay returns 204 and cannot revoke a later fresh explicit choice.
Minimal revoked grant metadata remains until the original expiry so a lost
response cannot recreate consent. A wrong or missing capability returns 404.
Clients implementing a global opt-out must stop local collection/uploads first
and withdraw every locally retained scope handle; one handle covers one scope.

The internal `route_contribution_counts` view filters revoked or expired rows
and immediately reflects deletion. It has no public endpoint, persisted rollup
or automatic publication effect. One passage is not independent corroboration,
legal access evidence, a closure finding or permission to change a route.

## Retention and physical deletion

Consent expires at UTC midnight 30 calendar days after its server creation date.
A summary expires at the earlier of that deadline and midnight 30 days after its
observation date. A retry never extends either deadline. Expiry immediately
removes rows from the live counts view and prevents further use of the grant.
Physical cleanup runs at API startup and hourly, even with collection disabled
or `JOB_WORKERS=0`. It deletes expired consent rows and dependent summaries, any
remaining expired summaries, and expired abuse counters. It skips locked rows
for a later pass. Downtime or persistent contention can delay physical removal;
an operational 30-day deletion commitment therefore requires monitoring and an
independent cleanup schedule while the API is unavailable.

The shipped `/contribution-retention` binary runs one cleanup using only
`DATABASE_URL`; it needs neither provider credentials, the scope secret nor an
enabled feature. Its output contains counts, never rows or connection strings.
Keep a tested schema-19 cleanup image and compatible withdrawal endpoint
available during an application rollback. A pre-19 API cannot perform erasure:
either retain the compatible service or erase all private contribution records
under explicit operator authority before removing it. Migration down refuses
automatic storage removal. Do not retain expired rows in diagnostic exports.

## Activation, backups and restore

Activation requires all of the following, separately from merging this source:

1. Rehearse migration 19 and the exact image against a private restored baseline,
   verify existing data, and admit the matching migration manifest/operator hook.
2. Resolve custody for deployment dumps, PostgreSQL/WAL/volume/host snapshots,
   off-host copies, proxy/collector logging and deletion/expiry of those copies.
   No shared-host snapshot policy is established by this change. Keep collection
   off until each actual copy has an enforceable retention/deletion procedure.
3. Provision a private random 32-byte secret as 64 hexadecimal characters in
   `ROUTE_CONTRIBUTION_SCOPE_KEY`, and explicitly set
   `ROUTE_CONTRIBUTION_CUSTODY_APPROVED=true` after that review. Never print it.
4. Run the exact image's `/contribution-retention --admit-custody` against the
   intended database with those operator-managed settings. This is a separate
   operator action; startup and deployment never grant admission automatically.
5. Wait until two UTC date boundaries after admission, qualify consent/client
   behavior and the required field evidence, then explicitly set
   `ROUTE_CONTRIBUTIONS_ENABLED=true`. Check the intake endpoint and authenticated
   flows. Configuration alone cannot open an unadmitted database.

Repeated admission with the same key retains its original date. A different key
is refused while any unexpired consent records remain, including revoked
tombstones. To rotate, shut collection off, retain withdrawal and cleanup, wait
for the old records to expire and be purged, provision the new key, admit it and
observe the two-day hold. The old key immediately fails the new admission check.
Waiting for physical purge is an operator requirement; the admission command
enforces expiry of all prior consents, not completion of their physical purge.
Do not rotate to bypass daily deduplication or resurrect an expired choice.

The repository's schema-19 backup hook excludes table data for
`route_contribution_consents`, `route_contributions`,
`route_contribution_limits` and `route_contribution_custody`. It retains their
schema and unrelated application data. Voluntary observations are deliberately
disposable; restoring them from deployment backups is unsupported. The custody
record is also excluded, so restoring the filtered dump closes the effective
gate even if the process still has its former enable settings. The release
smoke test restores this dump and checks both the empty tables and closed API.
Cleanup does not delete the non-personal custody record during ordinary use.

For a filtered restore, keep collection off, repeat custody review/admission and
wait two UTC date boundaries. Because new grants accept only today or yesterday,
client requests authored before the restore cannot recreate deleted consent
after this hold. For any unfiltered dump, filesystem/volume snapshot or replica
restore, **disable collection before connecting the API and clear
`route_contribution_custody` as an operator before re-admission**. Such copies can
contain prior admission, observations and revocation state; the application
cannot detect an external snapshot restore. Remove private data from the restored
copy and enforce deletion in every other copy before reopening collection.

This source change neither changes installed production settings nor prunes
existing backups. Existing and external snapshot custody remains an activation
gate. It makes no deployed, device-tested, field-calibrated or anonymity claim.

## Web foreground integration

The web active-trip panel makes a separate explicit choice for the signed-in
account, exact retained revision and travel mode. Starting navigation, granting
location permission or signing in never creates consent. Unsupported/legacy
revisions and mixed transit trips do not sample. A changed account requires
reopening the trip; changing revisions requires a separate choice. Navigation
does not depend on the contribution controller or its network requests.

Eligibility additionally needs optional `signal_references` on the selected
approved corridor segment for that mode. These use the existing proposal,
review and frozen revision path; no database migration is added. The curator
must explicitly review parallel/crossing/overlapping alternatives and stop-access
exclusions, retain their source/version and provide explicit arrays, including
empty arrays when that absence was reviewed. The canonical reference is bounded
to 50–5000 m and 512 vertices; at most eight competing paths and 32 stop areas
are accepted. The client adds frozen story-stop arrival areas, rejects more than
32 combined exclusions and checks allowed selected traversal/direction against
the retained usable journey. Quality approval or computed route geometry never
substitutes for this signal review. Existing routes without it remain valid for
navigation and unsupported for observations.

The controller receives fixes only from the active foreground browser watcher.
It keeps the pure detector's bounded episode state in memory; no fix, accuracy,
exact sample time, token or persistent trip identifier enters its storage or
summary transport. Pause, hidden document, permission loss, manual/automatic
arrival, end, revision change and account loss discard incomplete observations.
Resuming starts fresh. Fixes older than 30 seconds are excluded. Admission is
checked on foreground/online/retry and every 30 seconds while the trip is open;
an unchecked gate older than one minute stops observation. A failed gate check
also stops observation. This deliberately limits offline sampling; already
completed minimal summaries can wait offline.

Browser storage is scoped to the exact API base URL and account/revision/mode.
It holds at most 16 consent/deletion records, 32 pending summaries per record and
128 KiB total. The exact consent body/key is durably saved before the first POST;
sampling waits for its deletion capability to be durably saved. Lost responses
reuse the original body and key, never a new request day or renewed grant.
Before upload the client rechecks its saved choice, captured identity, admission
and the original server consent. Only the ten-field summary allowlist can be
persisted or uploaded. A server daily-duplicate response clears the corresponding
pending item without creating another key.

Pending summaries become unusable at midnight seven UTC calendar days after
their observation day, sooner than seven elapsed days for later observations.
Cleanup runs at app/controller start, foreground/retry and before upload. A
closed or suspended browser cannot physically erase local storage on a timer;
expired data is removed at the next such opportunity and never uploaded. Consent
and deletion records with receipts are removed by their original expiry, with no
automatic renewal. An uncertain grant request is retained for at most 31 UTC
calendar days after its request day, because creation can occur on that day or
the next day; it contains no queued summary after withdrawal. Clearing
browser/site storage also loses local erasure capabilities;
server expiry still applies.

The active-trip and profile controls stop observation, clear every queued summary
for this browser/server and withdraw each retained capability, including after
sign-out and while intake is off. Withdrawal sends no bearer token. A request
whose consent response was lost first needs an exact replay to recover its
capability; that requires the original account and an admitted service. The UI
keeps this deletion pending instead of claiming it completed. Other browsers and
devices retain their own controls. Storage failure prevents new work; a failed
local deletion write is reported as stopped/unavailable rather than successful
erasure. No activation setting or production custody record changes here.

## Verification evidence

Repository/API tests use disposable PostgreSQL and explicitly synthetic retained
journeys. They cover separate consent/default shutdown, admission/restore hold,
scope ownership, retained segment/direction checks, concurrent exact retries,
daily duplicates, withdrawal/revocation, quota, expired replay, live count repair,
physical deletion, rejected raw/precise payloads and log redaction. Migration
tests compare pre-existing rows and hashes across 18→19. The image smoke test
executes the shipped cleanup/admission binaries and filtered dump/restore path.
These checks establish persistence and API behavior, not real route access or
detector performance. Hosted CI on the exact PR head remains required.

Web controller tests cover default-off/absent consent, exact retry material,
storage failure, account/revision fences, withdrawal during an unresolved grant,
late admission responses, expiry and lifecycle suspension. Browser cases inspect
minimal payloads and storage, offline retries, pause, profile expiry, signed-out
capability deletion and enlarged-text reflow. Their synthetic observation and
consent transports are declared in the fixtures; server persistence and frozen
review checks use actual PostgreSQL separately. These are engineering fixtures,
not field calibration or phone-simulator evidence.
