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
changed migration files or a schema version other than 9, then saves a PostgreSQL
custom-format dump and media archive. CI never applies production migrations.
An intentional schema change needs a separately reviewed migration and baseline
update on the host.

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
Owners can read their own records through `/api/v1/me/route-briefs`, but visitor
creation and edits return `403 intake_disabled`. There is deliberately no
environment switch that opens public intake before the publication flow is ready.

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

The processor is intentionally unconfigured in this delivery. A queued run ends
in `needs_research` with `processor_unavailable`, without external requests or
model charges. Source discovery and real stage adapters are later roadmap work.
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
run budget described below. Historical drafting remains a separate roadmap issue.

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
requests. The remaining reconciliation, drafting and assembly processors remain
unconfigured; a successful discovery checkpoint therefore pauses with
`processor_unavailable` rather than claiming to have drafted a tour.

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
