# Web route reports and curator review

Visitors can choose **Suggest a route change** on a tour or an active trip. The
form records the exact published revision and travel mode they saw. A retained,
ready path is required; a legacy pending route cannot be reported through this
form until it has a reviewed release.

Select a stop, entrance or actual path leg, then describe the issue. Reporting
alone is sufficient. An optional marked location or local alternative can be
entered on the map or with coordinates; neither action records a background
journey. The API bounds the points to the selected area and checks their retained
stop identities. A source link is an unverified lead.

The tab retains the draft and saves the exact request and retry key before
sending. An uncertain response locks the fields and offers an exact retry, even
after reload. If browser storage cannot save that identity, no request is sent.
Discarding the local draft does not cancel a request already received by the
server. Drafts and retries are scoped to the account, revision and mode.

**Routes → Your route reports** shows private submission outcomes and the
curator's reason. A correction offers **Open trip to review the update**. The
visitor still chooses **Check for route updates**, reviews the newer revision,
and explicitly adopts it. A started trip keeps its original revision and progress
until that choice.

## Curator workflow

The **Route reports** queue prioritizes unresolved closures, blocked paths and
unsafe areas. The owner's list stays chronological. Status filters and bounded
pages keep the queue navigable.

1. Open the original observation and inspect its retained revision, affected area,
   direction, date and source lead. Select at most 19 related observations for a
   retained investigation. Duplicate groups are not independent witnesses.
2. Queue the investigation, then refresh to read its retained result. Its proposed
   path is distinct from the working route prepared for publication. Inspect the
   signed distance/time changes, restrictions, observations and escaped source
   passages. Follow the evidence/story package workspace link for sourcing work.
   An uncertain queue response keeps its exact selection and key for replay.
3. If necessary, prepare an arrival proposal. Changing coordinates resets walking
   and cycling access to unknown. Review the two modes and their sources before
   separately approving the proposal against its captured arrival version.
   Alternatively, import a sourced corridor reference JSON file, or an
   `{intent, quality_evidence}` bundle, up to 256 KiB. Review its branches,
   direction, explicit connections, sources and restrictions before its separate
   versioned approval. Submitted report points are never promoted automatically.
4. Recompute affected working paths after an adjustment. Reload the release
   comparison and inspect the reported snapshot beside the prepared route, stop
   order, visitor stories, shared-reference impacts and retained path assessments.
   Unresolved blockers prevent release. A pending proposal in this editor also
   disables release review.
5. Record a reason visible to the reporter. Request information/research or decline
   through the versioned disposition API. Approving a correction requires a
   current, ready comparison and explicit review consent. The release submits its
   comparison version to the atomic correction API. A conflict clears consent,
   requires fresh inspection and keeps the curator's wording. Temporary detours
   require a future reassessment date; that date does not imply reopening.

Approving an arrival or corridor changes working navigation inputs. Only the
correction release publishes a new tour revision. Shared places may affect other
working tours; potential-impact links require manual assessment and do not
publish another tour.

## Verification and rollout limits

Browser tests submit reports and retain curator dispositions against an owned
PostgreSQL/API fixture, including lost-response retries and account boundaries.
A build-tagged, unlisted synthetic tour supplies a ready retained path without
claiming real access or a routing-provider inspection. The ordinary seed's
pending route remains unchanged.

Ready comparison, adjustment and publication browser responses are explicit
synthetic transport fixtures. Real PostgreSQL correction tests separately cover
stale reads, atomic publication, concurrency and retained revisions. Browser
coverage checks escaped source text, keyboard segment selection, explicit
consent, stale approval, decline reasons and explicit trip adoption. These checks
do not establish real-world route safety, provider availability, native device
behavior or a production release.

This UI adds no migration. It requires the investigation and correction APIs from
migrations 17 and 18. Production admission still requires the backup, migration
rehearsal and installed-manifest gates in [route corrections](route-corrections.md).
