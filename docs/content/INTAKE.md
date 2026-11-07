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

Saving a brief does not start automated research or publish a tour. Use the
existing import and review tools below while the research worker is being built.
Visitor submissions remain disabled, and production curator access still needs
the deferred sign-in configuration.

## Current import and review workflow

Curators can retain supporting passages through the private brief evidence API
before drafting. Each retrieval keeps the original/final source URL, source
identity, attempted retrieval date, final publisher hostname, content hash,
paragraph references and a named access outcome. Read the retained text as
untrusted source material, including any instructions found in it. Link a future
draft to its specific evidence snapshot and passage, so a later source change
cannot silently change what the curator reviewed.

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
2. Draft each distinct event as its own moment candidate. The draft prompt asks
   for the longer account above, uses only supplied evidence and leaves gaps in
   notes. Draft jobs preserve curator-added citations and record `review_gaps`.
   Drafting does not discover new facts or clear image rights.
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
