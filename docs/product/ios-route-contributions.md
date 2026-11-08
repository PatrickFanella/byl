# Optional iOS route contributions

Issue #51 connects the shared uncalibrated detector to reviewed active trips.
The server gate remains disabled by default. This change does not activate
production collection or qualify real GPS behavior.

## Choice and lifecycle

The active trip's Route improvement sheet offers a separate choice for its
frozen revision and walk/bike mode. Eligible geometry must have reviewed signal
references, allowed access, an explicitly usable journey and bounded exclusions.
Missing or malformed observation metadata disables contributions without
preventing navigation. Transit trips are excluded.

The detector receives fresh authorized fixes only while the trip is visible in
the foreground. Backgrounding, permission loss, a stop arrival, revision switch,
account/server change, revocation and ending the trip discard the current raw
window. Arrival and explicit pause require an explicit continuation. Returning
to the foreground requires a fresh gate and consent check. Restarted trips
always start paused and require Resume. Cached fixes from before the choice
cannot begin a passage. Gate/consent checks expire after 60 seconds; visible
trips refresh every 30 seconds. Observation never requests extra permissions or
runs background collection.

Raw fix types are not Codable and never enter the private store or API. Only the
10-field reference summary can be queued. Existing navigation and narration
continue independently of optional storage, consent and upload failures.

## Retention and deletion

The original choice and idempotency key are saved before requesting consent;
the returned deletion capability is saved before sampling. The private
Application Support directory is excluded from backup before the first write,
with atomic protected iOS file writes and readback. Storage is bounded to 16
choices across all servers, 32 pending summaries per choice and 128 KiB total.
Each queued summary expires at midnight seven UTC days after observation.
Pruning rewrites the file before upload; retries preserve the original bodies,
keys and consent deadline. No bearer token or exact trip timestamp is stored.

Profile exposes retry and global Stop and delete, including after sign-out.
Global deletion invalidates pending choices across controllers, stops local
collection and removes queued summaries before network work. Late consent
responses retain their capability for withdrawal. Known capabilities are sent
without a bearer to their original server, even when intake is disabled. Lost
consent responses require the original signed-in identity and server to recover
the original receipt. A failed or 404 withdrawal remains pending; only success
or the original expiry removes its retained handle. Lost local handles and
other devices require separate handling. See [the ingestion contract](route-contributions.md)
for server retention, pseudonymous scopes and deployment custody requirements.

## Evidence and remaining qualification

On November 8, 2026, the shared 26-case JSON fixture was copied byte for byte
from the web reference. Swift tests cover its outcomes, minimal payload keys,
anonymous withdrawal, storage bounds and failures, UTC expiry, exact retries,
consent, identity/lifecycle controls and deletion during in-flight requests.
An unsigned iPhoneOS app build and the complete Swift package suite were run on
Soyuz. Exact final counts and hosted checks are recorded on the PR.

Native UI and actual iOS sampling are **unverified**. The connected device tool
exposes only the Linux host; Soyuz's installed simulators are not registered
there. No simulator was driven through SSH as a substitute. Before enabling
collection, use a supported iOS device session to record sampling gaps,
foreground/background and permission transitions, interrupted network retries,
request payload keys and signed-out deletion. Inspect accessibility and large
text on the contribution sheet and Profile. These checks remain distinct from
real-route calibration in #63 and from passing CI.
