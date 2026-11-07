# Illustrated place histories

The physical location is a **place**. A dated event or distinct historical thread
at that location is a **moment**. A place page shows its primary history in full
and lists the other published moments in date order. One complete story is enough
for a single-event place; several events do not require duplicate places.

## What intake must produce

Keep the place's name, address and coordinates separate from its historical
stories. Resolve an existing place before creating another record for the same
site. A nearby geocoded result is a review suggestion, not proof of identity.

Each story needs:

- A clear title and a substantial account of the event, its people and context,
  and its significance at this place. Aim for 300–500 words in short paragraphs.
  Publication requires at least 250 words and three paragraphs. Insufficient
  evidence means more research, not filler to reach the threshold.
- Named sources with a title, publisher, original URL and retrieval date. Prefer
  original records and the institution responsible for the site. Keep conflicting
  dates visible in review notes and cite the evidence behind the chosen account.
- Dates at the precision the evidence supports. An ongoing historical thread can
  use `circa`; do not invent a day to make a timeline entry look complete.
- At least one image with a caption, creator credit, original source URL, reusable
  license and license URL. Confirm what it depicts and when it was made. An
  archival view, map or family portrait can provide historical context if its
  caption says so; it must not be presented as a current site photograph.

A Wikipedia article's text license does not clear its thumbnail. Follow the
image to its own description and rights record. The current publication rules
accept public domain/CC0 and CC BY/BY-SA images. Noncommercial, no-derivatives,
unknown or rights-reserved images stay out of published stories. Illustrations retain their individual licenses; the repository license does not
relicense them. Original source links and license links remain visible to visitors. Keep a record of resizing or
other changes. Do not substitute generated reconstructions for documentary images.

## Photo collections, documents and video references

A featured illustration is only the starting point. Collect additional views of
the location, historical photographs, maps and scanned records when they explain
the place or event. Record the subject, date and viewpoint when known in the
caption. Distinguish a later photograph of the location from a photograph of the
event itself. Keep creator, original source and reuse rights with every copied
file. Prefer a useful, varied selection over repeated near-identical views.

Attach material about the physical site to the place. Attach a document or video
about a particular event to that event. The same library item can be attached to
both; it is stored once. Curators can select and order attachments in either
editor. Visitors see separate Photos, Documents and Videos groups, and can open
a full photograph from its image.

The media library accepts photos, PDFs, plain-text documents and audio up to
64 MiB per upload. Scanned documents can also be uploaded as images. PDFs and
text documents download as attachments. For another document format, or a record
without permission to copy, add a document link to its original location. A
document needs a descriptive title, creator/publisher, original source, and the
date or event context supported by that source. Hosted copies also need a
reusable license and its URL.

Videos are external references. Add the original watch-page URL, a descriptive
title explaining its relevance, creator/publisher and source page. Copying rights
are not required merely to link to an authorized publisher's page. Do not download
video files, copy thumbnails, embed a player, or infer that a linked video's
license permits redistribution. Video uploads are refused; imported video
candidates become external links and never enqueue a file download.

The drafting prompt records relevant attachment leads and exact supplied URLs in
review notes. It distinguishes a video transcript or description from watching
the video, and does not invent file rights or new source URLs. These leads still
need curator review and an explicit attachment.

Imported media candidates retain their explicit place/event target. Approval
attaches the item to that target; incomplete file metadata stays out of public
collections. Publication and attachment changes on published content require
complete provenance. External references use reference metadata checks rather
than the license checks used for hosted copies. Curator review still establishes
that the material is relevant and the publisher is the original or an authorized
source.

## Regional curated tours and shared themes

A curated tour can include places from several municipalities. Each place keeps
its own `city_id`; a tour does not move or duplicate a place to cross a boundary.
The tour retains its original `city_id` and slug for existing references, while
its `city_ids` lists the original city and all stop municipalities. City discovery
includes a public published tour in each represented municipality, once per tour.
Private and unlisted tours stay out of city discovery; direct-link access retains
the existing visibility rules.

Use one topic ID for a theme shared across the selected area. The curator topic
API accepts `city_ids` on creation or update, automatically retains the topic's
original city and supports up to thirty municipalities in total. Omitted/null
coverage leaves an existing topic unchanged; an empty array requests only its
original city. A municipality still used by the topic's moments or tours cannot
be removed. Add coverage before assigning that theme to a new municipality.
Topic coverage is public catalogue metadata; it is not a private tour boundary.

Original city/slug topic references retain precedence. When several shared
topics use the same slug in another municipality, use the topic ID rather than
guessing which theme was intended. Shared topics retain a total published-moment
count across their coverage; place and tour discovery remain filtered by the
requested city. The topic's original city, slug, title and description remain.

Older clients receive the same tour fields and full ordered stop list; each stop
still contains the place's real municipality and coordinates. They may label a
regional tour using its original city and ignore `city_ids`. Existing web/native
readers can follow the recorded tour by ID/slug. Personal-route builders retain
their single-city validation. Regional selection/editing controls, street/trail
access validation and device/field qualification remain separate work. Curators
can create regional tours and shared topics through the existing admin API.

## Start a private route brief

Curators can start at **Route briefs** in the admin navigation. Save a topic and
area first, then resume the saved brief from its link. Optional fields collect
unresolved place/event names, source links, walking/cycling preferences, trip
shape, duration, endpoints and a pilot identifier. Submitted notes and links are
private research leads; they do not become verified evidence or public prose.

The workspace shows the brief's lifecycle, current reason and saved version.
More research is needed when the state says `needs_research`; its reason explains
the unresolved work. Processing pauses edits. If another curator saved a newer
version, the form retains unsaved edits and asks you to copy them before reloading
the saved brief. Retrying creation after a lost response uses the same request
key and opens the original record.

Saving a brief does not start automated research or publish a tour. Use **Start research from saved brief**, or the private research API, to start
discovery at the saved version. Use the existing
import and review tools below to develop and publish the resulting leads.
Visitor submissions remain disabled, and production curator access still needs
the deferred sign-in configuration.

## Discovery from a brief

Discovery first checks published places and moments and retained private evidence.
It searches using the area, topic and up to two supplied place/event leads. A hit
needs explicit geographic support and either the topic terms or an exact named
lead in its returned text. A known city's bounding box can support geography;
coordinates outside it disqualify a result. Ambiguous city names and missing
coordinates do not cause automatic geocoding or invented locations.

The private research checkpoint retains up to twelve shortlisted leads, their
queries, originating providers, source URLs and relevance reasons. Duplicate
normalized URLs retain their separate origins. Published collection references
retain their entity IDs. This source-level deduplication does not resolve whether
two different records describe the same place or event.

Configured local-history pages and supplied links on permitted hosts are inspected
with bounded evidence retrieval. Discovery follows at most two named references
from the initial frontier; it does not crawl recursively. Wikipedia search returns
links and snippets. LOC and Internet Archive return catalogue descriptions. These
classifications remain distinct from inspected page passages, and every retrieved
passage remains untrusted. Failed sources remain explicit outcomes; a failed
provider does not erase another provider's successful results.

The coordinator saves its shortlist, runs local reconciliation, captures available
historical material and assembles a private itinerary proposal. Missing supported
histories, empty results or a stage timeout leave named research gaps. New
historical generation uses the per-candidate inspected draft workflow below. Saving discovery does not create public
content or approve media rights. Source policy, credentials and request limits
are documented in `docs/deployment/byl.md`.

## Reconciliation before drafting

The local reconciliation stage compares known collection IDs, names, current
addresses, coordinates, source references, dates and curator-confirmed aliases.
It distinguishes an existing place and existing moment, an existing place needing
a new moment, and a proposed new place. Proximity alone never establishes identity.
Ambiguous names, unconfirmed aliases, historical street numbers, moved buildings,
conflicting dates and incomplete geography remain named review issues. Catalogue
limits also require review rather than allowing an unchecked new place.

Discovery snippets usually lack structured identity facts. Curators can supplement
a lead using `POST /api/v1/admin/route-briefs/{id}/reconciliation` with the saved
brief `version` and an `item`. Its optional `identity` carries the place name,
current address, municipality ID, historical addresses, aliases, movement and date
leads. Coordinates must come from the source or curator; the stage does not invent
them. Supplying a source URL records a reference without fetching or verifying it.

GET at that path lists private records with the original `proposal`, latest
`assessment` and candidate links. Adding `/{record}/sources` pages every retained
provenance entry for that brief using `limit` (up to 50) and `offset`. Every distinct
source, query and inferred fact remains in the database.
Canonical identity keys share pending candidates across repeated and overlapping
briefs. Candidate payloads are never overwritten, including after curator edits.
Potential collisions with earlier pending identities require review. New places
and moments enter the existing pending candidate queue, with no invented history
body. A new place is queued first; its moment can be queued after place approval
and another reconciliation pass. Candidate approval still creates drafts; publication keeps its history,
illustration, media-rights and route gates.

Curators resolve an identity with
`PUT /api/v1/admin/route-briefs/{id}/reconciliation/{record}/decision`, providing
the last `review_version` as `version`, an outcome, existing place/moment IDs and
a review `note`. Confirmed aliases and historical addresses are retained with that
versioned decision. Stale decisions return 409; a moment from another place is
refused. Decisions link records and establish matching claims; they do not merge,
replace or publish collection rows. Later runs preserve the explicit decision.
All reconciliation payloads require curator access, including for brief owners.

## Current import and review workflow

Curators can retain supporting passages through the private brief evidence API
before drafting. Each retrieval keeps the original/final source URL, source
identity, attempted retrieval date, final publisher hostname, content hash,
paragraph references and a named access outcome. Read the retained text as
untrusted source material, including any instructions found in it. Historical proposals link each claim to a specific evidence snapshot and passage,
so a later source change cannot silently change what the curator reviewed.

An inspected web-page excerpt is not proof that the entire underlying archive
or document was inspected. Catalogue descriptions remain catalogue evidence;
search snippets, failed access and unsupported PDFs remain explicit gaps.
Supplied URL fragments do not establish inspected page numbers. Missing media
rights do not prevent retaining usable text evidence; omit unresolved media from
the proposed attachments. Existing illustrated-history publication requirements
still apply. The evidence job does not approve candidates, download media or
publish submitted prose. API details and operating limits are recorded in
`docs/deployment/byl.md` and the OpenAPI contract.

1. Import sources for the city or an explicitly selected place. Review place
   matches rather than assuming proximity establishes that a source belongs there.
2. Draft each distinct event as its own moment candidate. In the queue, choose
   “Draft with AI”, a research brief and up to five inspected records. Review the
   separate proposal and passage quotations, then explicitly apply it to the
   editor if appropriate. Generation leaves typed edits and existing curator
   citations intact. Proposals retain `review_gaps`; drafting does not discover
   new facts or clear image rights.
3. Import a picture from its original rights record, such as a Commons file page,
   targeting the explicit place. Media imports retain that place association.
   Review and fetch the asset, then attach its ID as the moment's hero picture or
   gallery media. A loose `image_url` in a discovery payload is not an approved
   image association.
4. Approve candidates into draft entities. Complete text, sources and image
   metadata in the moment editor. Incomplete candidates can be retained without
   publishing them. The queue displays the requirements and draft research gaps.
5. Publish the complete moment. The server checks length, paragraphs, citations
   and image provenance/license metadata. A new place requires a published,
   complete primary moment before the place itself can be published.
6. Review the actual place page, image and caption, source links, event selection
   and tour entry. Test both a one-story and a multiple-event location.

The server checks completeness, not historical truth. Curator review still needs
to verify that the sources support the prose and that the image depicts the
claimed subject. Existing import adapters, the draft job and publication checks
implement this contract; the broader automated route-intake roadmap remains a
separate project. Production drafting still needs an explicitly configured LLM
provider. This change does not enable a paid provider or public sign-in.

## Downers Grove pilot

[The reviewed histories](downers-grove/histories.json) and
[source register](downers-grove/sources.json) compile into migration 3 using
`python3 scripts/build-pilot-content.py`. CI's `--check` verifies the migration
and the image bytes against the [media receipt](downers-grove/media.json).
The seven published histories cover the existing four places; the station and
museum each have multiple stories. The unrelated in-review Pierce Downer record
remains unpublished.

Station and theatre illustrations are dated photographs. Fishel Park uses an
archival picture of the school formerly on its site. The museum uses a Smithsonian
portrait of Henry Williams Blodgett to accompany the family history. A reusable
photograph of the museum buildings and a current Fishel Park view are still
desirable additions; no permission to republish the available rights-reserved or
noncommercial photographs was inferred.

The seed illustrations are packaged with the API and served locally at
`/media/files/library/images/`; they do not depend on third-party hotlinks.
Uploaded media keeps its separate storage lifecycle. These pilot media URLs
target `https://byl.subcult.tv`; another installation must review its own public
base URL when importing the pilot records.

[The collection register](downers-grove/collections.json) adds three dated site
photographs, a hosted copy of the Interstate Commerce Commission's 12-page 1947
accident report, a link to the Historical Society's Blodgett House documentary,
and a linked Village preservation packet. It records file hashes, rights, source
URLs, retrieval dates and place/event associations. Migration 4 adds document and
place-collection storage; migration 5 installs these reviewed collection records.

## Historical sites and public arrival points

A place's historical coordinates remain separate from its public entrance or
viewpoint. Legacy places have `arrival_point: null`, `arrival_version: 0` and
unknown navigation access. A nearby road, shared driveway or the site centroid
never becomes a reviewed entrance automatically.

Curators propose a point using `POST /api/v1/admin/places/{id}/arrivals`, with the
current approved arrival `version` and a `point`. Each point names an `entrance`
or `viewpoint`, gives coordinates, and includes exactly one `walk` and one `bike`
assessment. Access is `unknown`, `allowed`, `restricted` or `closed`. Surface is
recorded separately and can also be unknown. Known access or surface requires a
source URL/title and a retrieval date; each mode retains its own evidence/context.
This is a source-based review, not an accessibility certification or field claim.

The proposal stays private and leaves the current approved point visible.
`POST /api/v1/admin/places/{id}/arrivals/{arrival}/review` takes the proposal's
`base_version` as `version`, an `approve` or `reject` action, and a private review
note. Approval advances the public arrival version atomically. A stale approval
returns 409, requiring a fresh proposal against the current version. Rejection
keeps the approved point. Changed locations or restrictions always need a new
proposal and review; completed inputs/reviews remain readable in private history.
Point and access notes are public after approval; keep private rationale in the
review note.

Navigation callers can use `GET /api/v1/places/{place}/arrival?mode=walk|bike`.
Only an allowed mode receives a `navigation_point`; unknown, restricted and closed
modes receive null. Historical coordinates remain in the ordinary place response.
Route recomputation uses reviewed entrances when any stop has an approved point
or the tour has approved corridor intent. Every stop then needs an allowed point
for the requested mode; unknown, restricted and closed fail explicitly. Review
an entrance change, then recompute: the old working cache cannot be published
against changed approval inputs. Already published revisions keep their captured
entrances and geometry. Client guidance and field qualification remain separate.
The Downer burial entrance used in
backend tests is a synthetic fixture lead, not an inspected real-world entrance.


## Publishing a fixed tour revision

A published tour now has an immutable revision containing its ordered stops,
selected full stories, citations and media references, reviewed arrival context,
routing inputs and computed geometry. Editing the working copy leaves that
revision visible until another publication succeeds. Standalone place and story
pages continue to show their current content; a client displaying a pinned tour
story must read it from the revision's `snapshot.moments` map. Media URLs and
rights metadata are retained as references, not a promise that a remote file
will remain available.

Curator editing endpoints return the working `draft_version`, `route_version`
and current `revision_id`. Send `draft_version` with edits to detect a stale
editor. Keep a stop's `stop_id` when reordering it; the server also retains IDs
for unchanged place/story pairs when older clients omit them. A different place
or explicitly selected story receives a new stop identity. Repeated visits can
use explicit IDs to distinguish them.

Publish through `POST /api/v1/admin/tours/{id}/publish`, supplying
`draft_version` and `current_revision_id` from the loaded working copy. The
server compares both under a tour lock, rechecks publication requirements and
creates the revision atomically. A stale request returns 409. Publication still
requires ready routes computed from the current ordered sites. Changed site
coordinates require recomputation even if no tour field changed. Curated tours
require explicit publication. A personal route's existing save/share behavior
retains an exact-version release request and fulfills it when its routes become
ready; `/api/v1/me/tours/{id}/publish` also supports explicit owner publication.

`GET /api/v1/tours/{tour}/revisions` lists release metadata;
`GET /api/v1/tours/{tour}/revisions/{revision}` returns the frozen content.
Current visibility and historical private visibility both restrict access.
Unpublishing, making a tour private or deleting it revokes public access to its
retained revisions. Deleting a previously published tour hides it while keeping
history; it does not erase snapshots or progress.

New clients store progress with stable `visited_stop_ids` through
`GET`/`PUT /api/v1/me/progress/{tour}/revisions/{revision}`. A new revision starts
with no inherited visits. Existing position-based progress endpoints project
positions against the trip's pinned revision. Authenticated tour and route reads
then resume that same revision until the visitor explicitly resets progress,
which moves the legacy trip to the current published revision. Clients must
attach their bearer token to those reads; anonymous reads show the current
public release. A personal route with its first routes still pending has no
published revision yet, so progress returns 409 until it is ready. Client update
prompts and full adoption of the revision endpoints remain separate work.


## Private route problems and improvement reports

Operational feedback is separate from historical research. A signed-in visitor
can submit `POST /api/v1/me/route-reports` for the immutable tour revision they
saw. Supply `tour_id`, `revision_id`, `mode`, a `reason`, and an `affected` object.
A minimal blocked entrance uses `affected.kind: "entrance"`, the captured
`affected.stop_id`, and `reason: "blocked"`. No source, alternate path, date or approved entrance ID
is required. Direction and duration default to `unknown`; do not invent an
observation date when it is unknown.

An affected location is a stable `stop_id`, an entrance at that stop with an
optional captured `arrival_id`, or a segment with `from_stop_id` and `to_stop_id`
from adjacent story positions or a declared closing return leg. Segment order
must agree with a known `forward` or `reverse`
direction. Mode, identities and optional geometry are checked against that frozen
revision, so later reorders cannot retarget a report. A historical private
revision does not become reportable by strangers when an owner later shares a
new version.

Reasons are `blocked`, `closed`, `unsafe`, `access`, `surface`, `improvement` or
`other`. Duration is `permanent`, `temporary` or `unknown`. For an improvement,
record `better` in concrete terms such as a safer crossing or smoother surface;
`other` needs explanatory details. `observed_on` is an optional calendar date.
An optional `source_url` is a reference, not inspected evidence, and this API
never fetches it. Do not copy a report or its unverified anecdote into visitor
history.

One optional `annotation` point marks the affected location. An optional
`alternative` is a bounded local LineString suggestion, not a recorded trip or
approved route. Points must stay near the frozen affected path and connect to
its anchors; the exact limits are in the OpenAPI/operator contract. Proximity
does not prove a legal entrance, connection at a crossing, or mode eligibility.
There is no photo upload or background location recording in this workflow.

Owner list/get endpoints are `/api/v1/me/route-reports` and `/{id}`. Curators use
`/api/v1/admin/route-reports`, `/{id}` and `POST /{id}/state` with the expected
`version`, disposition `status` and a reason. New reports enter private `review`.
The existing brief lifecycle controls subsequent decisions; accepting a report
records a disposition and changes no route or public story. The report and
reason remain visible only to its submitter and curators, even after the original
route is withdrawn. Submission input is immutable; a new observation is a new
report. Preserve the original retry key across transport retries to avoid
creating duplicates. Quotas and bounded bodies limit intake.

This adds API and client fixtures for later report forms. It leaves ordinary
new-tour public intake closed and does not configure public sign-in, create
historical drafts or grant contribution consent.


## Trail branches, connectors and shaping points

Reviewed corridor intent records where a curator wants the route to go without
turning every bend into a historical exhibit. Curators import a `CorridorIntent`
with source/version/retrieval and license metadata through
`POST /api/v1/admin/tours/{id}/corridors`. A proposal is private and leaves the
current published tour unchanged. Review source copying rights, mode permissions,
branch identity, topology and connectors before approving. Source notes become
public after tour publication; keep private rationale in the review note.

The reference consists of identified LineString segments. Each gives explicit
`from_node`/`to_node`, branch memberships and independently allowed modes. Empty
modes mean unresolved access. A road connector has kind `road_connector`, its own
source context, and must be listed in that mode's `allowed_connectors` before the
path may use it. Each mode independently selects a branch, direction and ordered
segment traversals, with explicit reversal where needed. Another branch, nearby
road or visual crossing cannot be substituted automatically. A connection exists
only through the declared endpoint identities; review grade separation rather
than equating coordinate overlap with a traversable junction.

A shaping point is tied to a particular traversal occurrence and lies between
adjacent stable story-stop IDs. It shapes routing without creating a
place, moment, narrated stop or arrival event. Its order is separate from the
story-stop list. A later stop reorder can invalidate these references; approve a
replacement or explicit removal before publishing that draft. Recompute preserves
the choices, and earlier published revisions retain their reference/selection.

`POST /api/v1/admin/tours/{id}/corridors/{proposal}/review` takes the proposal's
`base_version` as `version`, an `approve` or `reject` action and a private note.
Approval also compares the captured working draft version. A stale approval
returns 409 and requires a fresh proposal. Approval changes the working intent
and routing generation; it does not release a new public tour, including for a
personal route. Publish explicitly after reviewing the ready working routes.
Controlled navigation always requires explicit publication, including after a
later recompute or legacy metadata save. New personal routes remain owner-only
drafts until their first qualified publication; the owner can inspect working
intent while research continues.
Approved removals retain their intent version with null intent. Completed reviews
and input remain immutable.

The released tour's optional `corridor` holds full reference geometry and chosen
mode paths; `routing_intent.corridor` pins its ID/version/checksum. Private author
IDs and review notes are absent. The checksum covers canonical typed intent JSON,
not a remotely downloaded file. Import limits and coordinate tolerances are in
the OpenAPI/operator contract. Source links are recorded without fetching them.
The controlled Valhalla adapter uses story entrances as break points and reference
vertices/shaping points as via points. These retain one leg per story interval.
An explicitly closed node path includes a final travel leg from the last story
to the first; it adds no narrated stop. Open paths must end at the final approved
entrance. Repeated stop occurrences need an unambiguous ordered selection;
otherwise routing fails for review. A shape at a story boundary must be moved
strictly between its referenced stops. There is no straight-line or other-mode
fallback, automatic road substitution, reference simplification or provider-limit
retry with fewer points. At most 256 controls may be computed; providers can have
a lower limit. Use an explicitly reviewed bounded reference when a limit fails.

Routes without approved navigation inputs retain their historical-site behavior.
Production still uses the development provider, which cannot compute approved
navigation. A ready controlled result confirms geometry/protocol checks only;
current access, topology quality and on-site qualification need their own review.
The bounded development comparison is recorded in
[the routing evidence note](../routing/approved-path-comparison-2026-11-07.md).


## Historical proposals from inspected evidence

`POST /api/v1/admin/candidates/{id}/draft` now requires an `evidence_ids` array
with 1–5 distinct retained evidence UUIDs. The queue page selects these from a
brief's inspected pages or catalogue descriptions. Pending/failed retrievals,
search snippets and raw submitted/imported text cannot be used instead. Inspect
the relevant source through the brief evidence API first. No evidence is fetched
and no anecdote is forwarded as part of a model draft request.

The job captures the candidate payload hash and immutable evidence IDs. Only
bounded source passages enter the provider request: at most 12000 runes total,
shared across the selected records. Place labels and topic choices supply context,
not historical facts. Retained proposals include the evidence IDs, source URLs,
retrieval instants, content hashes, passage references and exact supporting
quotations. A source change requires a new retrieval record rather than changing
an earlier proposal's evidence.

Every body paragraph must be a cited claim, with 1–6 valid passage references and
exact quotations of at most 300 characters. Title and supplied dates also require
citations. Unknown references, invented quotations, uncited prose, extra response
fields and malformed responses are rejected. An attributed account names its
attribution in the paragraph. Conflicting dates retain two distinct passage
references in visible uncertainty, with empty date/display fields and `circa`
precision. The model cannot select a contested date as an asserted field.

These checks establish traceability and format. They cannot determine whether an
excerpt entails a claim, discover every disagreement, or certify a historical
interpretation. Curators must compare all claims, dates and testimony with the
actual retained evidence. The queue shows claim quotations, title/date references
and unresolved disagreement for that review. Source instructions are untrusted
data and give no approval or publication authority.

Generation appends private `generated_drafts` to the candidate without replacing
its title, body, dates or sources. A simultaneous edit or approval/rejection stops
the old result from committing. A stale queued job stops before calling a provider.
At most five proposals and 256 KiB of candidate payload are retained; review those
before creating another candidate. Each provider draft is at most 32 KiB. The
editor applies a proposal only on an explicit click, merges source links without
discarding existing curator citations, and keeps uncertainty in private draft
notes. Approval still creates an unpublished entity, followed by separate content,
image-rights and publication review. The proposal's claim trail describes its
generated text; subsequent curator rewrites need their own factual review.

Media and narration remain optional and require separate approval. Drafting
creates no attachments or playback jobs. Brief research can reuse completed
per-candidate proposals during local material capture and itinerary assembly, as
described below. Generating a new historical proposal still requires a configured
provider; local assembly enables neither a paid provider nor public sign-in.

## Path assessment before publishing a route

A computed line can be retained as a draft while its access and network evidence
are unresolved. New personal routes start as owner-only drafts; choosing unlisted
visibility describes access after publication. Existing published revisions remain
available while a curator develops a replacement.

For each selected corridor segment, record `network.ways` in traversal order,
including each way's digitized orientation, and the reference's `from_node_id` and
`to_node_id`. These are reviewed OpenStreetMap identities, not IDs inferred from
nearby geometry. Record an independent `access` decision for each mode: allowed,
unknown, restricted or closed. Select road connectors explicitly and retain their
own source context. A line crossing, bridge or tunnel does not establish a junction.

Submit private `quality_evidence` alongside the corridor proposal through
`POST /api/v1/admin/tours/{id}/corridors`. Each reference identifies the segment,
`purpose` (network or access), completed source `evidence_id`, passage `reference`
and an exact retained quote of at most 300 characters. Access references also need
`mode`; network references omit it. The existing inspected-evidence workflow must
retrieve the source first. Catalogue descriptions, a source URL and provider
assertions do not count as inspected page passages. A quote establishes provenance;
a curator must still establish that it supports the claimed identity or access.
References and normalized inspections are captured before review and retained
with that immutable proposal. Missing evidence remains a research gap.

After approving the arrivals and corridor, recompute each mode. The tour editor
shows whether path quality is blocked, needs research, out of date or ready for
curator review. **Inspect retained evidence** opens the private passages and provider
outcome. The private API is
`GET /api/v1/admin/tours/{id}/routes/quality?mode=walk` (or bike); `report_id` selects
a retained checkpoint. Curator authorization is required even for a tour's owner.
Public summaries contain findings and an opaque report ID, without private quotes
or source-evidence identifiers.

A passing assessment requires matching ordered network ways and directions,
connected nodes, permitted mode/access and inspected reference evidence. Strict
provider attribution must describe the computed path; a map-snap fallback cannot
substitute another route. Straight-line development geometry cannot pass.
Known closures block their mode before routing. Missing provider identities,
ambiguous access and failed attribution remain named gaps. Changing the routing
inputs or rewriting a route invalidates the assessment for publication.

Passing means eligible for curator review. It does not establish current field
conditions, grant access permission or publish automatically. Release requires
current passing assessments for every selected mode plus the existing content
checks and an explicit publication action. Keep historical stories editable while
research continues; do not invent access facts to complete an itinerary.

## Assemble a private itinerary proposal

The brief workspace now offers **Start research from saved brief** and **Inspect
itinerary proposal**. Save edits before starting. Starting research uses the saved
version and preserves unsaved form text; it does not silently reload the editor.
Inspection can be repeated after processing without changing the form. Curators
can also read `GET /api/v1/admin/route-briefs/{id}/itinerary`; optional `run_id`
selects a retained run belonging to that brief. This source-rich proposal is
curator-only, including when the brief's owner has an ordinary visitor account.
Owner research progress continues to omit stage output and source text.

After reconciliation, the local drafting-material stage captures sourced existing
historical records and current evidence-linked candidate proposals. It retains
place/event/candidate IDs, identity review versions, selected text, source URLs,
evidence IDs and content hashes. Unknown identities, archived records, missing
source material and generated proposals whose candidate input has changed remain
linked research gaps. No candidate is approved and no curator text is overwritten.
The B5 source/citation validator is run again before using a generated proposal.
An existing curated record and an evidence-linked generated draft remain visibly
different sources of material.

Assembly ranks the requested topic and event terms, chooses one featured event
per physical place and orders a compact selection. It does not add unrelated
stops to reach a count. A shared place can feature different histories for
different themes. Outlying supported material remains linked in exclusions;
fewer than two supported locations yields a useful partial proposal marked
needs research. A single-location history can still be developed independently.
Requested starts/ends must resolve to supported place names; unresolved or
incompatible endpoints remain explicit gaps.

Trip shape accepts loop, point-to-point and out-and-back, with flexible retained
for briefs that do not choose. The proposal separates story order from a travel
sequence: a loop closes to the first stop, and out-and-back retraces its stops
without creating duplicate narrated stories. Planned dwell is three minutes per
stop, with planned narration based on the retained text at 130 words per minute.
Actual travel time stays uncomputed. A deliberately optimistic straight-line
screen can exclude impractical geography; it does not establish route distance,
travel time, legal connectivity, access or publication quality. If no duration
was supplied, the proposal records a provisional 60-minute screen. Flexible shape
records a point-to-point planning assumption for review.

The introduction and transitions describe the selected theme and accounts without
new historical facts or navigation instructions. Stop narratives reuse retained
paragraphs/claims. What-to-notice entries are explicitly curator research questions
until current viewing guidance is supported. Research gaps, exclusions and
planning assumptions remain visible. This bounded deterministic assembly makes
no model request; local material/assembly stages retain deadlines, attempt limits,
cancellation and worker fences without reserving external requests or model tokens.
Completed checkpoints replay without creating another proposal or tour.

The result is a private proposal. It creates no tour row, publishes no content and
computes no route. Curator acceptance/materialization and the existing historical,
media-rights and path-quality publication gates remain separate steps. Full source
and identity records remain linked to the brief even when a bounded proposal omits
material. Start a fresh saved-version research run when the brief or research has
changed; inspect an older run when reviewing its original snapshot.


## Inspect the complete private review package

Curators can read `/api/v1/admin/route-briefs/{id}/review-package`, optionally with
`run_id` and a prepared curated `tour_id`. The response combines selected visitor
stories, evidence passages, current/new/shared dependencies, proposed differences,
geometry, path checks and unresolved tasks in one versioned snapshot. Processing
and needs-research results remain inspectable and cannot become publication-ready.
Reading creates no records and approves nothing. See [the package contract](CURATOR-PACKAGE.md)
for exact-version checks, readiness, bounds and the later approval/UI boundaries.
