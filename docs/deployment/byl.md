# Byl web releases

The public application is https://byl.subcult.tv. Almaz's existing wildcard
Cloudflare tunnel and Caddy route forward to the NUC at `10.0.0.56:3093`.
The isolated Compose project `byl` owns PostgreSQL, the Go API, nginx web client,
and its database/media volumes. The retired museum checkout and database volume
are outside this deployment.

## Automatic delivery

`.gitea/workflows/ci.yml` builds release images after backend, web/browser,
Android core and iOS build/package checks pass. Pull requests build and smoke-test
both Docker images without deployment secrets. Main pushes build the exact Git
revision, check migrations against a disposable database, verify static assets,
and transfer the images through a dedicated SSH key. A retry reuses any image
already accepted for that revision.

The pinned `fleet-deploy` receiver on NUC accepts only Byl's `api` and `web`
services. It requires current main, matching source/revision labels, unchanged
runtime settings and the operator-owned pre-deployment hook. The hook refuses
changed migration files or a schema version other than 16, then saves a PostgreSQL
custom-format dump and media archive. CI never applies production migrations.
An intentional schema change needs a separately reviewed migration and baseline
update on the host.

The repository hook now targets schema 17 for private route-report
investigations. The installed schema-16 hook and manifest continue to refuse that
image until migration admission is completed. See
[the investigation rollout gate](../product/route-report-investigation.md#validation-and-rollout)
for migration, retained-data and worker rollback constraints.

The receiver replaces only API/web images and checks Docker health, exact public
HTML and `/release.json` bytes from the image, API health, seeded city/tour
responses, and disabled development sign-in. Failed activation restores the
previous image IDs. A successful public check publishes a Gitea prerelease named
`main-<12-character revision>`, reflecting the app's current rebuild status.
This workflow does not distribute iOS or Android binaries.

## Host configuration and secrets

The operator installs `deploy/compose.yml` in `/srv/apps/projects/byl`, with a
mode-0600 `.env` containing the database credentials and image selections.
`deploy/byl.Caddyfile` is the dedicated Almaz route. PostgreSQL and API publish
no host ports. Port 3093 accepts ingress from Almaz through the host firewall.

Only `HOMELAB_DEPLOY_KEY` and `HOMELAB_KNOWN_HOSTS` belong in the repository's
encrypted Actions secrets. The app key is restricted to one fixed receiver/profile
and the CI source addresses; it grants no shell. The receiver revision is
`ecc088dad74351b536f156b351cf199e95e3f220`. Key custody belongs to the SUBCULT
homelab operator, with initial rotation due February 3, 2027. Retain unrelated
human and service credentials during rotation.

Set the Actions variable `VITE_GOOGLE_CLIENT_ID` and host `GOOGLE_CLIENT_IDS` to
the same registered Google web client ID, allowing `https://byl.subcult.tv` as
a JavaScript origin. Set `ADMIN_BOOTSTRAP_EMAILS` privately on the host for curator
access. Without a Google client ID, public browsing is available but web sign-in
is unavailable. Development sign-in stays disabled.

The initial stack uses filesystem media, no generated narration, and the existing
straight-line routing provider. Street-aware walking/cycling routing requires
Valhalla and its map tiles; the separate `backend/docker-compose.nuc.yml` describes
that larger setup. Straight-line routes must not be treated as street directions.

## Verification and recovery

Run `python3 scripts/verify_release.py <full-revision>` for public release checks.
`bash scripts/release.sh` builds and smoke-tests without deploying by default.

Host-only receiver state lives in
`~/.local/state/fleet-deploy/byl/releases/`. Each `receipt.json` records prior and
new image IDs and an exact rollback command. Check that a later deployment has
not superseded it before running that command as the host operator. CI refuses
to deploy an old revision. Do not run migration `down` to roll back an image.

Backups live in `/srv/apps/projects/byl/backups/` with private permissions.
Database dumps are transactionally consistent; the separately archived media can
change during a backup. Initial deployment must include an isolated database
restore check. There is no automatic backup/image pruning. The release hook
creates backups before deployments; it does not provide scheduled or off-host
backup coverage.

## Illustrated history content, migration 3

Migration `00003_illustrated_place_histories.sql` changes only the pilot content:
seven published histories at the existing four places, two new event records,
four illustrated assets, and their citations. It changes no table structure,
accounts, sessions, routes or progress records. The image bytes ship in the API
binary at `/media/files/library/images/`; no third-party hotlink or mutable media
volume copy is needed. Source records and rights evidence are under
`docs/content/downers-grove/`.

Before admitting migration 3, back up the live database and restore it into a
separate database. Apply all migrations to that copy with the candidate migrate
binary, verify place/event/media/source counts, confirm all original unrelated
rows and accounts/progress are unchanged, and check the history quality rules.
Then back up production again, apply the reviewed migration as operator, update
`migrations.sha256` from the same candidate image and install the matching
operator-owned `scripts/byl-pre-deploy.sh` (expected version 3). Keep the old
manifest/hook and database dump for recovery. The image receiver still refuses
unadmitted migrations. A guarded main run may be retried unchanged after admission.

Automatic migration down is deliberately refused: later curator edits must not
be overwritten by a blind content rollback. Restore only the affected pilot
content from the retained backup after reviewing subsequent writes. The two
new event IDs and four media IDs are fixed in the reviewed migration. Image
rollback alone remains possible because the data change is additive and keeps
the existing API fields and relationships.

## Place collections, migrations 4 and 5

Migration 4 adds the document media kind, an `external` flag defaulting to false,
and ordered place/media associations. Existing fields, records and event
attachments remain. Migration 5 adds the reviewed collection register: three
photos, one hosted PDF, one external video reference and one external document
reference. Its three additional event associations refer to the specific crash
or Blodgett histories; all six assets also appear in their place collections.

Rehearse both migrations against a restored production backup before admission.
Use two stages to avoid sending new document/video records to the previous web
client. First back up production, apply only migration 4, and admit the exact new
five-file checksum manifest with an operator-owned copy of the hook expecting
version 4. This is a temporary, explicit schema admission for the new image; the
receiver still applies no migration. The new API works with either version 4 or
5. Retain the prior hook and manifest.

Let normal main CI activate and verify the new API/web images. Then apply the
reviewed data-only migration 5 with that image's migration binary and install
the source hook expecting version 5. Verify collection membership, the hosted
file bytes and download headers, external links, and browser behavior. No video
file or third-party thumbnail should be transferred. Later automatic releases
use the final version-5 hook and exact migration manifest.

The previous web image cannot display the new document/video references. Before
an image rollback, review later curator edits and selectively unlink the three
new event associations if returning to that client. Retain the asset records,
files and place associations for recovery. Automatic down refuses to delete
curator collections; a database restore or selective rollback needs an operator
review of subsequent writes. Keep the per-release database and media backups.

## Private route briefs, migration 6

Migration 6 adds only `route_briefs` and its indexes. It does not alter existing
content or account records. Submitted notes, unresolved place/event names and
links live in the private `submitted` object; intake creates no evidence,
candidates, tours or public prose. Curators use `/api/v1/admin/route-briefs`.
Owners can read their own records through `/api/v1/me/route-briefs`. Visitor
creation is controlled by `PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED`, false by default;
visitor edits remain unavailable. The web suggestion/status flow and intake
rollout checks are described in [Web tour suggestions](../product/web-route-suggestions.md).
Enabling creation stores private research leads for curator triage; it does not
start research or grant publication authority.

Creation allows 10 new briefs per rolling 24 hours and 20 unresolved briefs per
account, serialized in PostgreSQL. An optional `Idempotency-Key` header (up to
128 bytes) makes retried creation safe. Use a new key for a different submission.
Edits and curator decisions require the latest `version`; conflicts return 409
and leave the saved record intact. Editing a disposition resets it to draft and
requires another review. Stop a processing brief before editing it. Accepting a
brief never publishes content.

Before deployment, restore a fresh production backup into an isolated database,
apply migration 6 and check the original rows, public catalogue and new private
API behavior. Back up production again, apply the reviewed additive migration as
operator, and admit the exact six-file checksum manifest with the source hook
expecting version 6. CI continues to refuse unadmitted migrations and applies no
production SQL. An older API/web image can run against version 6; it ignores the
new table. Retain private submissions during an image rollback. Automatic down
refuses to destroy them; any later schema removal needs a reviewed backup and
explicit data-retention decision.

## Resumable route research, migration 7

Migration 7 adds private research runs, stage checkpoints and attempt outcomes.
Each run freezes its submitted brief and moves through research, reconciliation,
drafting and assembly. Outputs and pending review candidates commit together.
Completed stages are skipped on retry, and candidate insertion never overwrites
curator edits. Nothing in this workflow approves candidates or publishes entities.

Curators start with `POST /api/v1/admin/route-briefs/{id}/research` and the latest
brief `version`. Retrying that source version returns its existing run. The GET
at the same path returns safe progress; owners have a read-only equivalent under
`/me`. Cancel and resume actions add `/cancel` or `/resume` and require both the
current brief version and `run_id`. Cancellation releases the brief for editing
and fences in-flight results. Resume requires unchanged submitted input and
retains checkpoints, charged usage, attempt counts and the original deadline.
Generic job retry refuses research jobs so it cannot reset those budgets.

Runs allow at most 12 processor operations, three attempts per stage, 20,000
reserved model tokens and 15 minutes from creation. Every operation is charged
before invocation and receives a 30-second context deadline and a 2,000-token
ceiling. Reservations survive interruption because crash usage can be unknown.
Each processor implementation must make at most one external request per
operation, honor the context and enforce the token ceiling. Failed calls use
safe reason codes; raw provider errors, source content and credentials must not
enter progress responses or routine logs. Attempt outcomes remain after a retry
succeeds. Stale job claims cannot commit results or finish a newer worker's job.

The initial migration-7 delivery left the processor unconfigured: a queued run
ended in `needs_research` with `processor_unavailable`, without external requests
or model charges. Later discovery, reconciliation and local material/assembly
stages are described below.
Stubbed-provider tests establish orchestration behavior; they do not qualify
real research quality, provider cost accounting or route safety.

Rehearse migration 7 on a restored production copy, verify existing records and
public responses, then take a fresh backup before operator application. Admit
the exact seven-file manifest and source hook expecting schema 7. CI applies no
production SQL. Older application images ignore the additive tables; retain the
checkpoints and private outputs during image rollback. Automatic down refuses
to delete them.

## Private source evidence, migration 8

Migration 8 adds only `source_evidence` and its indexes. Before deployment,
restore a fresh production snapshot, apply the exact reviewed migration and
compare existing rows/public responses. Take a fresh production database/media
backup, apply the additive migration as operator, and install the exact
eight-file manifest with the source hook expecting schema 8. CI applies no
production SQL and refuses an unadmitted manifest. Retain the previous images,
hook, manifest and backups. Application image rollback keeps schema 8 and later
private evidence; automatic down refuses to remove it.

Curators use `POST /api/v1/admin/route-briefs/{id}/evidence` with the saved brief
`version`, a source `url` and optional `source_kind` (`page` or `catalog`). The
optional `Idempotency-Key` header makes request retries safe. There are at most
20 new retrievals per brief per rolling 24 hours. GET at the same path lists
snapshots; adding `/{evidence}` returns one retained result and its passages.
All evidence APIs require the curator role, including for the brief's owner.
There is no public or owner source-payload endpoint. Source content remains
private, untrusted data. Jobs carry the evidence ID, never URLs or source text.

Each explicit curator job allows three attempts, each with a total 20-second
timeout, at most two redirects, a 2 MiB body limit and a 20,000-character excerpt
limit. There is no HTTP-layer retry. Each connection uses the existing public-IP
guard with direct IP dialing and no environment proxy; evidence ignores the
trusted-private-host option used by other archive/media operations. Credentials,
nonstandard ports, non-web redirects and HTTPS downgrades are refused. Completed
snapshots are immutable across retries and worker restarts. A new request key
creates a separate retrieval snapshot; the normalized URL keeps the same source
identity. Old snapshots retain their original brief version.

Only UTF-8 HTML/plain text is excerpted. A publisher field records the final
hostname, not an inferred institutional author. Paragraph references locate text
within the retained excerpt; the supplied fragment is stored separately and is
not proof of inspecting a PDF page. PDFs and other formats remain explicit
`unsupported_format` gaps until a reviewed extractor exists. Inaccessible,
oversized, timed-out and unreadable sources remain named outcomes. Catalogues
stay `catalog_only`; search snippets are never inspected full sources. This
does not bypass access restrictions or copy full archives into public content.

Media reuse stays `unresolved`: inspecting text or a page license does not clear
images, video or documents for copying. No media is downloaded by evidence jobs.
These manual curator retrieval limits are separate from the automatic research-
run budget described below. Retrieval does not generate historical prose; use the
per-candidate evidence-linked drafting path described below.

## Regional topics and curated tours, migration 9

Migration 9 adds `topic_cities` and its city lookup index. It backfills each
topic's original city and existing moment/tour municipality references; it changes
no existing place, topic, tour, stop, account or route row. Topic coverage is
public catalogue metadata. Review any existing cross-city topic references in a
restored snapshot before admitting their coverage. Current single-city seed
references keep their IDs, slugs and original municipalities.

Before activation, restore a fresh production snapshot, apply the exact reviewed
SQL and compare existing table fingerprints. Run the previous API against that
copy and compare representative public city/place/tour responses. Back up the
live database/media, apply migration 9 as operator under the receiver lock, then
install the exact nine-file migration manifest and version-9 source hook. CI
applies no SQL and refuses a release with unadmitted migrations. Retain the old
images, manifest/hook, backups and rollback receipt. Image rollback retains
schema 9 and later regional associations; automatic down refuses to remove them.

Tour `city_id` remains its original discovery anchor. New `city_ids` is derived
from that city plus current stop municipalities. Each stop's `place.city_id`
retains the actual location. Published public tours appear in every represented
city's discovery results without duplicates; private/unlisted rules still apply.
Only curated tours gain cross-city stop writes. Existing personal-route requests
and visitor builders keep their single-city limits.

Topic creation accepts optional `city_ids`; update replaces that coverage when
the array is supplied. The original city is always included. Omitted/null updates
leave coverage unchanged, and an empty array retains only the original city.
Unknown references, more than thirty municipalities, uncovered moment/tour
references and removing a municipality still in use return 422. Coverage edits
serialize with dependent writes and roll back together with scalar edits on
failure. Original local topic slugs retain precedence; ambiguous shared slugs
return 409 and require a topic ID. Shared topics keep a total published-moment
count across municipalities.

`city_ids` is an additive response field and is optional in the OpenAPI schema
for older API versions. Existing Swift Codable readers ignore unknown fields,
Android's JSON configuration ignores unknown keys, and the web readers continue
to use the unchanged ordered stops and IDs. An older client may show the original
city label while displaying the full spanning tour. This PR does not add regional
editing controls to those clients or qualify real route geometry/access. An old
API image can read the expanded schema but retains its prior city-limited tour
write behavior; regional editing requires the current API.

## Private brief discovery policy

Starting a saved brief's research run now executes discovery. It reuses relevant
published collection records and permitted retained brief evidence before new
requests. Local reconciliation and historical-material/assembly stages now follow
discovery as described below. Missing supported histories remain research gaps;
generating new candidate histories still needs the explicitly configured B5 drafter.

`BRIEF_DISCOVERY_PROVIDERS` selects up to three distinct providers from
`wikipedia_search,loc,internet_archive` (the default). Wikipedia uses its public
MediaWiki `list=search` API, restricted to article namespace, with twelve results
and no continuation. LOC and Internet Archive reuse the existing adapters. The
archive adapter requests catalogue metadata without fetching media-file details.
No provider requires a credential, and no model provider is enabled by discovery.

`BRIEF_DISCOVERY_SEED_URLS` replaces the default starting pages with up to four
HTTP(S) pages. Defaults are `https://dghistory.org/`, the Downers Grove Wikipedia
overview and the DuPage County Wikipedia overview. Hosts from these configured
pages permit submitted source links and followed references; off-policy pages and
page redirects remain blocked. `LOCAL_ARCHIVE_URLS` continues to configure manual
imports independently. A private-host bypass never applies to discovery.

Discovery considers at most four starting links (submitted links first), three
archive queries (area/topic, then up to two area/lead/topic queries) and two named
references from the initial page frontier. It does not recurse, request provider
pagination, copy media or use geocoding to guess missing locations. Results need
explicit area plus topic/lead support in their returned content, independent of
the query. A resolved city boundary rejects out-of-area coordinates. Conservative
matching can miss useful sources; absence from the shortlist is not proof that
the history does not exist.

Every HTTP request, including redirects, consumes a persisted research allowance
before it is sent. The first uses the allowance reserved when the research stage
began; additional source requests spend no model tokens. The existing twelve-
request run ceiling, stage retry ceiling, twenty-thousand-token ceiling and
fifteen-minute run deadline remain in force. The initial stage retains its
conservative 2,000-token reservation even when discovery uses no model or network.
There are no HTTP retries during discovery. Each call has at most two redirects,
a 20-second HTTP timeout, a 2 MiB body bound and the public-IP connection guard.
The research stage has a 30-second ceiling; a timeout checkpoints its successful
partial results with `provider_unavailable`. Worker cancellation and stale claims
still prevent commits. A depleted request allowance stops further calls and
retains completed provider results.

The private stage output retains at most twelve leads with their source, query,
relevance reasons and evidence classification, plus up to six retrieval snapshots
with 3,000-character excerpts and content hashes. A catalogue or search snippet
does not become inspected full-text evidence. Page excerpts stay untrusted and
media reuse stays unresolved. Source URLs and provider errors do not enter routine
job logs or owner progress responses. No new schema migration is needed beyond
the separately admitted migration 8. Image rollback retains private checkpoints.

## Private identity reconciliation, migration 10

Migration 10 adds `reconciliation_records`, `reconciliation_sources` (with its
ordering sequence) and append-only `reconciliation_decisions`. Existing collection,
account, research and candidate rows are unchanged. Original inferences and each
distinct source/query are retained; curator decisions use optimistic versions.
Stable candidate keys and a transaction advisory lock prevent competing briefs
from minting duplicate candidates. Candidate payloads and published content are
never replaced by this stage. The lock follows the brief/run/job claim fences;
cancelled or superseded workers cannot commit reconciliation records.

Before activating the accepted image, restore a fresh production snapshot, apply
the exact reviewed SQL, compare all existing table fingerprints, and run the
previous API against that copy. Back up live database/media under the receiver
lock, admit migration 10 as operator, then install the exact ten-file migration
manifest and source hook expecting schema 10. CI applies no SQL and refuses an
unadmitted manifest. Retain prior images, hook, manifest, backups and receipts.
Image rollback keeps schema 10 and later provenance; automatic down refuses to
remove it. Do not restore an old dump over later curator decisions.

Production enables local reconciliation after source discovery. Matching examines
at most 201 places, 101 moments per place, 21 source references per moment and
1001 latest curator claims; exceeding the corresponding 200/100/20/1000 bounds
requires review. Leads are at most 32768 encoded bytes, with at most 20 per stage;
the private checkpoint retains its 256 KiB cap. This stage makes no external
requests and uses no model provider. The coordinator's conservative stage budget
reservation remains charged. The later local historical-material and assembly
stages reuse retained records/proposals without another provider reservation;
missing historical drafts stay named research gaps.

## Reviewed public arrival points, migration 11

Migration 11 adds `place_arrival_proposals` and `place_arrival_current`. It changes
no existing place, account or tour row. Proposal input and completed reviews are
immutable, including at the database layer. The current pointer advances only
through an explicit curator approval under a place-row lock and expected version.
Concurrent proposals against an old version cannot replace a newer reviewed point.

Restore a fresh production snapshot and apply the exact accepted-image SQL first.
Compare existing table fingerprints and representative responses from the previous
API. Under the receiver lock, back up live database/media, admit migration 11 as
operator, then install the exact eleven-file manifest and source hook expecting
schema 11. CI applies no production SQL. Keep the prior images, hook, manifest,
backups and receipts. Image rollback retains this additive schema and subsequent
arrival decisions; automatic down refuses to remove retained evidence.

Existing clients continue to receive the historical `latitude`/`longitude` and
`arrival_radius_m`. Additive `arrival_point` is null and `arrival_version` is zero
until review. No existing centroid is backfilled as a legal entrance. The new
public `/api/v1/places/{place}/arrival?mode=walk|bike` endpoint selects only an
independently allowed mode and otherwise returns a null `navigation_point`.
Public responses exclude curator IDs and private review rationales. Source context
and public point/access notes are visible after approval. Source links are not
fetched by arrival APIs and do not prove an on-site inspection or accessibility.

This is the access-data and selection foundation for subsequent routing work.
The current route worker and existing web/native clients do not adopt these points
yet; route intent, recomputation fences and client adoption belong to their own
issues. Existing computed geometry is not certified as legal/public access.


## Immutable tour revisions, migration 12

Migration 12 adds immutable `tour_revisions`, `tour_revision_state`, route-input
digests, revision progress and legacy trip pins. Existing published tours receive
revision 1. Legacy progress positions map only to stop IDs in that snapshot;
out-of-range positions are omitted from the projection. Original `tour_progress`
rows remain unchanged for recovery, including their original timestamps and
invalid positions. Existing working tours, stops, routes and historical site
coordinates remain unchanged. Published snapshots retain content and geometry
when those working records change.

The pre-admission compatibility rehearsal found missing derived story/topic
fields in the first schema-12 candidate (#76). The corrected, still-unadmitted
migration captures topic IDs, published topic counts and narration availability
with the rest of the snapshot. Compare complete public tour/story projections,
including these joined fields, rather than only stored source columns. Compare
RFC 3339 timestamps by instant when their timezone strings differ. Migrations
1–11 are unchanged; do not admit the initial `ddedb1b` release candidate.

Before admission, restore a fresh schema-11 production backup into an isolated
database and apply the exact accepted migration. Compare all preexisting table
fingerprints, publication counts, snapshot content/geometry and progress mappings.
Check the previous API's catalogue reads against the expanded schema. Require
published routes to be ready and the production route queue to be quiescent
before capturing the backfill. Exercise concurrent publication/editing, stable
stop identities, progress isolation/reset and stale route success/failure commits
on disposable fixtures. Route jobs carry a routing version and a digest of actual
mode, ordered stop IDs and site coordinates; neither a superseded result nor its
last failure may overwrite the current working route.

Under the receiver lock, take and independently restore a fresh database backup,
archive media, then apply migration 12 as operator. Admit the exact twelve-file
manifest and source hook expecting schema 12. CI applies no production SQL.
Retain prior images, hook, manifest, table fingerprints, dumps and deployment
receipts. Check the final image's SQL hashes against that admitted source before
activation, then verify the live build, published snapshots, private API gates
and existing public assets.

Automatic down refuses to delete revision history. Retain schema 12 during image
rollback. The previous API can read the expanded working tables, but its writes
do not maintain revision pointers or progress: after revision-aware writes begin,
use a forward fix or restrict a rollback to read-only recovery. Do not serve an
old writable API as an equivalent revision-aware release or restore an older dump
over later curator data. Physical deletion of accounts/tours referenced by
immutable releases requires a separate reviewed retention/anonymization procedure;
the ordinary tour delete operation hides the retained record. This migration
does not activate public sign-in, telemetry, a routing provider or client update
prompts, and does not certify geometry as legal public access.


## Private versioned route reports, migration 13

Migration 13 adds only `route_reports`, its indexes and an input-immutability
trigger. It changes no existing tour, snapshot, progress, account or historical
place row. The report keeps a composite foreign key to its exact tour revision.
Submission/owner/target/hash/creation fields cannot be updated; curator disposition
uses an expected version and the existing private brief lifecycle. Acceptance
never edits route intent, entrance access, geometry or historical text.

Rehearse the exact accepted SQL on a fresh restored schema-12 production backup.
Compare every existing table fingerprint and representative public tour/revision,
story and geometry responses from the previous API. Independently restore the
actual operator database backup before SQL application; verify the media archive.
Under the receiver lock, recheck live fingerprints, apply as operator, then admit
the exact thirteen-file manifest and source hook expecting schema 13. The already
admitted migrations 1–12 must retain their checksums. CI applies no production
SQL. Keep previous images, manifest, hook, dump and receipts. Automatic down
refuses to delete private reports; image recovery retains schema 13 and later
submissions, with the schema-12 revision-write recovery constraints above.

`POST /api/v1/me/route-reports` requires a session and accepts operational
observations about a visible immutable tour revision. It does not open the
ordinary public route-brief intake or configure Google/Apple sign-in. Owner
list/get APIs restrict records in SQL; curator list/get/disposition APIs require
the curator role. Responses use no-store. Notes, dates, coordinates and decisions
remain private to the submitter and curators. New submissions on a withdrawn
route are refused; the submitter can still read an existing report or retry its
original key after withdrawal.

Bodies are at most 32 KiB, details 4000 characters, improvement meaning 1000,
source links 2000 bytes and retry keys 128 printable non-space ASCII bytes.
An account may create 10 reports per rolling 24 hours and retain at most 20
unresolved reports. An optional account-scoped `Idempotency-Key` replays the
original submission without another quota slot; changed input returns 409.
Missing keys represent new submissions. Dispositions do not change the original
submission hash. No background jobs, source retrieval, telemetry collection,
photo upload or automatic route mutation are started by these APIs.

A map annotation is one explicit WGS84 point near the affected frozen location.
An optional alternative is a 2–64-point LineString; all points stay within 1 km
of the affected stop/entrance or segment path. Segment endpoints must connect
within 100 m of its ordered frozen endpoints; a stop/entrance alternative needs
one end within 100 m of that anchor. When a revision lacks approved arrival data,
the historical location only bounds the private observation; it does not become
a navigation entrance. A nearby coordinate or visual crossing establishes no
access or traversable junction. Review is still required before any route change.


## Reviewed corridor intent, migration 14

Migration 14 adds `tour_corridor_proposals` and `tour_corridor_current`, with
immutable proposal inputs and completed reviews. It wraps the admitted snapshot
and route-input hash functions while retaining their prior implementations.
Existing snapshots and every existing table row remain unchanged. With no corridor
history, hashes remain byte-for-byte identical, preserving ready route caches.
Approved intent/removal versions enter the hash and routing generation fences.
Previously admitted migrations 1–13 retain their exact checksums.

Before admission, restore a fresh schema-13 production backup into isolated
PostgreSQL. Apply the exact accepted SQL, compare all existing table fingerprints,
ready input hashes, public catalogue/revision/story/geometry responses and previous
API reads. With no approved corridor, public responses omit the new optional field.
Independently restore the actual operator backup before production SQL, verify the
media archive and recheck live fingerprints under the receiver lock. Apply as
operator and admit the exact fourteen-file manifest/source hook expecting 14.
CI applies no production SQL. Retain backups, prior images, manifests and receipts.

Curators import reference data through
`POST /api/v1/admin/tours/{id}/corridors`, then review it through
`POST /{proposal}/review`. Listing/detail routes remain private. Input is at most
256 KiB, with 64 reference segments, 5000 coordinate pairs in total and two
independent mode selections. Each mode allows 64 traversals, 16 explicit road
connectors and 60 shaping points. Source metadata records URL/title, publisher,
version, retrieval date, license and license URL. Road connectors need their own
source context. The server retains a SHA-256 of canonical typed intent JSON;
this is not a claim that a remote source file was downloaded or independently
hash-verified. Importing an HTTP(S) reference never fetches it.

A selected path connects only through explicit endpoint node IDs. One node's
endpoints must agree within 20 m; equal/near coordinates with different node IDs,
interior crossings and adjacent roads imply no connection. Segments must name the
selected branch and independently allowed mode. An empty allowed-mode list remains
unresolved and cannot be selected. Reference direction is recorded explicitly;
ordered traversals encode orientation. The synthetic Prairie Path fixture is not
an official map or evidence of a traversable crossing.

Shaping points identify a selected traversal occurrence and adjacent ordered
story-stop UUIDs. They must lie within 50 m of their reference segment. They
never insert a place, story, tour stop or arrival event. Changing stops/modes may
make approved intent obsolete; publication then requires a reviewed replacement
or removal. A removal is proposed with `remove: true` and null/omitted `intent`.
Approval compares both the proposal's base intent version and captured draft
version, increments the working draft/route generations, requeues routes and
clears legacy personal auto-release. Explicit tour publication releases it.
Recompute and legacy saves cannot reopen automatic release until the current
intent version is explicitly published; an initial pending public view omits it.
Recompute retains the approved pointer; old published snapshots keep their full
reference geometry and choices. Snapshots expose no author IDs/private rationale.

Schema 14 provided durable reference/selection data while routing still used
historical sites. Schema 15 adds controlled routing as described below.
Source/mode labels and a curator approval are not a legal-access or field
certificate. Curators must review reference copying rights and access context
before publication; do not infer permission from a map link or crossing.

Automatic down refuses to discard intent/history. Retain schema 14 during image
recovery. Prior APIs can read existing snapshots with the additive schema, but do
not enforce the new working-intent reference guards. After intent-aware writes,
prefer a forward fix or restrict prior-image recovery to read-only operation.
Do not restore an older dump over later curator decisions. Never-published tours
with retained proposals are hidden on deletion rather than physically erased.
No region-scale import, street router, public sign-in or field validation is
activated by this migration.

## Approved navigation, schema 15

Migration 15 adds function wrappers without rewriting existing table rows, cached
routes or published snapshots. Migrations 1–14 keep their exact checksums. With
no approved arrival, the existing route-input hash remains identical; corridor
versions are already fenced by migration 14. Approved arrival identities,
versions and point/access JSON enter the hash. A result from an earlier approval
is rejected even when the tour's routing generation has not changed. The worker
reads the working snapshot and hash in one database statement, then rechecks the
hash under tour/place locks before committing its result.

Any approved arrival or non-null corridor intent requests controlled navigation.
Each stop must have an independently allowed arrival for that mode. The Valhalla
adapter sends story entrances as `break` and intermediate reference/shaping points
as `via`; via allows the U-turn needed by an out-and-back without creating a story
leg. Ordered directed traversals retain all reference vertices, selected connector
controls and reviewed shapes. Explicit matching start/end node identities declare
a return, with a final leg from the last story to the first. Open paths must start
and finish at their corresponding approved entrances. Ambiguous repeated stop
occurrences and reversed or boundary-coincident shapes fail for review.

The request is bounded to 256 controls without silent resampling. A provider can
reject a lower configured limit. Independent mode failures remain visible; no
other-mode, nearest-road or straight-line fallback is used. The development
provider deliberately lacks controlled capability. Entrances without a corridor
allow normal network paths between stops. With a corridor, geometry guards require
ordered controls and constrain returned vertices to a 50 m control envelope;
endpoints also have a 50 m bound. Requested travel cannot be shortened by more
than 10% plus 25 m, and provider totals must agree with story-leg totals within
rounding tolerance. These engineering bounds detect omitted controls and obvious
shortcuts; they are not calibrated field or topology certification. Issue #29
owns broader path-quality assessment, including parallel roads and crossings.

New controlled legs retain stable stop IDs, `navigation_source` and an optional
`return` flag in the existing JSON column. New snapshots label
`routing_intent.coordinate_source` as `approved_navigation`, retain arrival
coordinates in routing stops and retain the original historical coordinates as
`site_latitude`/`site_longitude`. Previously published snapshots keep their exact
content. Controlled computation never fulfills legacy automatic release intent;
curator/owner publication is explicit. Maps and totals include closing travel;
the existing visit/completion workflow still tracks narrated story stops. Web,
Android and iOS classify the closing leg as future travel while stories remain;
this does not introduce a separate guided return/completion phase.
Segment reports can target a declared closing leg in either travel direction.

Before admitting schema 15, independently restore a fresh actual production
backup, apply exact accepted SQL in isolation and compare all existing table
fingerprints, ready hashes, old API reads and public catalogue/revision/story/
geometry responses. Verify the actual operator backup and media archive before
production SQL. Recheck the live baseline under the receiver lock, apply as
operator and admit the exact fifteen-file manifest and source hook expecting 15.
CI never applies production SQL. Admission and live verification are separate
from code/CI acceptance; this section specifies the gate, not a deployment claim.

Down refuses to discard approved input fences/history. Retain schema 15, backup
and image receipts. Earlier APIs can read the additive schema but older workers
ignore controlled routing and its publication semantics. After navigation-aware
writes, use a forward fix or read-only prior-image recovery with old workers
stopped/drained. Do not restore an older dump over later curator work. This change
does not activate production Valhalla, download map tiles, configure public sign-in
or establish real-world access. The bounded development comparison is recorded in
[the routing evidence note](../routing/approved-path-comparison-2026-11-07.md).


## Inspected historical draft proposals, schema 14 unchanged

Historical drafting reuses immutable `source_evidence` records and private
candidate JSON. No migration or production model provider is added. Exact
migration checksums and the hook's schema-14 gate remain unchanged. Configure a
provider only through the existing private runtime settings when authorized;
provider-free deployments continue to return 503 for drafting.

The curator draft endpoint requires 1–5 inspected evidence IDs and queues only
those IDs, the candidate ID and its payload SHA-256. It accepts inspected pages
and catalogue descriptions with retained passages, a retrieval instant and a
content hash. Failed/uninspected sources are rejected. Provider input includes at
most 12000 passage runes in total, shared across records; no raw candidate text,
submitted anecdote or curator narrative is offered as evidence. There is no
external retrieval from the drafting job.

Both providers use the closed claim/citation schema and retry malformed model
output once. The worker independently validates custom provider results. Invalid
citations, unknown actions/fields, missing fields and uncited prose fail without
changing the candidate. Dates and testimony retain explicit uncertainty and
attribution. These are structural provenance checks, not historical truth or
semantic entailment certification. Curator inspection remains necessary.

The worker appends up to five private proposals, each bounded to 32 KiB before
provenance metadata, within a total 256 KiB candidate payload. It compares pending
status and the complete prior JSON payload atomically. New curator edits or review
win over stale completion. Proposals retain source IDs/URLs, retrieval timestamps,
content hashes and exact quote references; source text stays in its evidence
record. Editable prose and existing sources are unchanged by generation. Applying
a proposal in the editor is explicit; approval/publication remain separate.

Legacy jobs without inspected IDs and a captured payload hash fail without
mutating candidates. Before a prior-image recovery after draft-aware writes,
stop or drain draft processing: old workers do not enforce these evidence/edit
guards. Keep schema 14 and later candidate/evidence history; prefer a forward fix
or read-only recovery. Do not restore an older dump over curator work. No real
paid-model output or authenticated production drafting is certified by stubbed
provider, database and browser tests. Brief-stage material capture and assembly
can reuse these proposals as described below; new historical generation remains
an explicit curator job. Native clients and field-routing qualification remain
separate.

## Path quality checkpoints, schema 16

Migration 16 adds immutable private corridor-inspection input and per-mode quality
report history, plus a current-report pointer. Reports bind the exact routing
input version/hash and computed route fingerprint. Rewriting a route invalidates
its pointer, including writes from older applications. Publication requires current
passing quality for every selected mode; a database insert trigger also protects
against revision publication by a previous API. Previously published snapshots,
active trips and existing collection rows are not rewritten.

The worker requests bounded Valhalla `trace_attributes` with `shape_match=edge_walk`
on original story-leg shapes. It does not fall back to snapping, substitute a path
or turn provider assertions into inspected access evidence. Unavailable attributes
leave a research result, while known closures prevent routing that mode. See
[the two-pilot comparison](../routing/path-quality-comparison-2026-11-07.md) for the
observed development-provider limitation. No production routing provider is
activated by this change. New personal routes remain owner-only drafts until they
meet publication requirements. Existing published tours remain readable.

Before admission, independently restore a fresh actual schema-15 production backup
into isolated PostgreSQL, apply the exact accepted migration and compare existing
tables, snapshots and complete public responses. Rehearse the previous API's reads
against the expanded schema. Confirm both older plain route writes and stale
reports cannot authorize new publication, without changing retained production
rows. Check that the candidate image contains the exact admitted SQL. Under the
deployment lock, recheck the accepted head and live baseline before applying SQL;
admit the exact sixteen-file manifest and source hook expecting schema 16. CI
applies no production SQL and refuses an unadmitted manifest before activation.
Independently restore the actual release backup and verify the live revision and
public responses after release.

Automatic down refuses to delete the new evidence/history or remove the gate.
Retain schema 16, accepted source, image digests, backups and deployment receipts.
Earlier APIs can read retained snapshots but their route writers invalidate quality
and their revision inserts lack the new gate's requirements. Prefer a forward fix;
if an older image is needed for diagnosis, keep it read-only. Do not restore an
older dump over later curator work or resume older workers as a recovery shortcut.

## Local themed itinerary proposals, schema 16 unchanged

The API worker enables bounded local historical-material capture and itinerary
assembly after reconciliation. Existing curated source records and validated B5
candidate drafts can support a private proposal without a new model call. Missing
material remains a named research gap. Local stages retain claim/version,
cancellation, deadline and attempt fences, but reserve no external-request or
model budget. Existing optional research processors retain their provider-stage
reservation behavior.

Completed proposals use the existing private research checkpoint JSON; no new
migration, manifest or hook version is required beyond schema 16. Research
progress for owners still excludes source output. The curator itinerary endpoint
returns the retained run/source version; cross-brief run references are refused.
The brief workspace starts saved-version research and inspects proposals without
discarding unsaved form edits. No public tour, candidate approval, route computation
or publication is implied by assembly. Production sign-in/model/Valhalla settings
are not activated by this change.

Qualification must include actual PostgreSQL replay, cancellation, source-linked
candidate validation and curator/owner boundary checks. Published catalogue,
revision and route fields should match the prior release. Retain existing schema,
research snapshots, candidates and later curator work during image recovery;
new out-and-back brief values may be uneditable in an older client until the
current application is restored. Prefer a forward fix when later work exists.

## Web trips pinned to published revisions, schema 16 unchanged

The web trip reader loads one immutable revision containing the itinerary, paths
and complete selected stories. It stores the whole revision together and keeps
an active pointer per account and tour. Local and authenticated progress use
stable stop IDs within that revision. Legacy position records are retained but
are never copied into revision-aware progress or mapped onto reordered stops.
Saved tours link back to the active revision and its progress.

A newer release appears as an explicit update. Visitors can compare its stop
order and modes before adopting it with separate revision progress. Changed
stop notes from a newer published curated revision appear as updated reviewed
guidance while the active geometry remains pinned. This uses published tour
notes; it introduces no independent urgent-notice feed or automatic detour.

During an API outage, the reader can use its coherent saved revision. If no copy
exists, it asks for reconnection without substituting a newer route. An online
403 or 404 refuses cached fallback and clears the visible trip, retaining local
progress for recovery. Offline resume still requires the web application itself
to be available; this change does not add a service worker or cache remote map
tiles and media files. Referenced media may become unavailable independently.

Arrival, map pins and external directions use the retained public arrival point
when present. Path readiness, trail connectors, return assumptions and travel,
stop and narration times remain distinct. Blocked or unsupported travel modes
cannot open a guided trip. Older APIs without revision IDs retain the legacy
reader with an explicit warning that the trip is not pinned.

No migration or host admission change is required. OpenAPI now marks fields
already always emitted on revisions and snapshots as required. Rolling back the
web image leaves revision caches and stable-ID progress intact, but the older
client does not understand those local records. Do not copy their contents into
position-based keys to make an older client appear to resume the trip.

## Atomic curator package publication, schema 16 unchanged

Curators inspect a prepared tour through the private route-brief review package,
then POST `version`, `run_id`, `tour_id` and an optional `note` to
`/api/v1/admin/route-briefs/{id}/review-package/publish`. The version must match
the complete reviewed response. An incomplete package or changed dependency
returns 409; reload and review before trying again. A note becomes the brief's
decision reason and is visible to its owner.

One serializable transaction publishes selected prepared media, histories and
places, retains a new immutable tour revision, queues narration for newly
published histories and accepts the brief. A failure rolls everything back.
Previous revisions remain available. Candidate approval/materialization is an
earlier explicit draft-review step: this operation leaves contextual candidates
and proposed shared-content edits untouched. Unresolved, declined and
needs-research packages cannot be released.

The revision's existing JSON stores `_curator_package_approval` with the brief,
research run, reviewed version, reviewer and decision note. Publication time and
publisher also remain on the revision row. This private provenance is omitted
from the typed public snapshot and all revision responses; do not expose raw
snapshot JSON through a public endpoint. No migration or host admission change
is needed. No production content is approved by deploying this API.

The curator brief page provides package review beside the saved-brief editor.
Select a prepared tour, load the package, compare each story and picture with
its retained source passages and uncertainty, and inspect each mode's geometry
and path evidence. The affected-record list distinguishes selected prepared
dependencies from contextual candidates and unapplied shared edits. Editing
links open the existing record editors in separate tabs. Reload the package
after editing and review it again before confirming publication.

Targeted history research uses explicitly selected inspected sources for pending
candidates. Resuming an incomplete research run retains its completed checkpoints.
Neither action replaces editor text. A research request or stale-version response
requires another package load before a decision. Returning or declining a brief
requires a reason and uses the saved version; publication uses the exact package
version and an explicit review checkbox.

Saved-brief reloads retain unsaved form text. If another curator changed the
submitted fields, the editor retains its earlier expected version, so saving
cannot overwrite those changes. Copy any local text you need, then use
**Replace editor with saved brief** to adopt the newer fields. Status-only
refreshes keep the editor usable without discarding unsaved work.
