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
changed migration files or a schema version other than 7, then saves a PostgreSQL
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
