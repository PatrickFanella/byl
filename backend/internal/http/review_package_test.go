package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

type packageFixture struct {
	env            *testEnv
	curator, owner string
	brief          *models.RouteBrief
	run            *models.ResearchRun
	proposal       models.ItineraryProposal
	evidence       string
	path           string
}

// Retained assembly and qualifying path reports are explicit persistence fixtures;
// real source capture/assembly/assessment have independent worker integration tests.
func newPackageFixture(t *testing.T) packageFixture {
	t.Helper()
	e := newTestEnv(t)
	e.drain(t)
	ctx := context.Background()
	curator := e.signIn(t, "package-curator@example.com", models.AccountRoleCurator)
	owner := e.signIn(t, "package-owner@example.com", "")
	ownerID := e.get(t, "/api/v1/me", owner).body["id"].(string)
	b, _, err := e.store.CreateRouteBrief(ctx, ownerID, models.RouteBriefInput{Topic: "Memory", Area: "Downers Grove", TripShape: "point_to_point"}, "")
	if err != nil {
		t.Fatal(err)
	}
	tour, err := e.store.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	proposal := models.ItineraryProposal{Version: 1, Status: "draft", CreatedAt: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Title: "Memory", Introduction: "Source-linked private visitor draft", TripShape: "point_to_point", RequestedShape: "point_to_point", Modes: tour.Modes, Stops: []models.ItineraryStop{}, TravelSequence: []int{}, ResearchGaps: []models.ItineraryGap{}, Excluded: []models.ItineraryExclusion{}, Assumptions: []string{"Synthetic transport fixture, not a qualified real route."}}
	for i, stop := range tour.Stops {
		if stop.FeaturedMoment == nil {
			t.Fatal("fixture lacks selected story")
		}
		moment, err := e.store.GetMoment(ctx, stop.FeaturedMoment.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		record, err := e.store.ReconcileBrief(ctx, b.ID, models.ReconciliationRequest{Version: b.Version, Item: models.DiscoveryItem{Kind: models.CandidateKindMoment, EntityID: moment.ID, Evidence: "published_collection", Title: moment.Title, Text: "Retained curated record", Area: "Downers Grove", Origins: []models.DiscoveryOrigin{}}})
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256([]byte(moment.Body))
		urls := []string{}
		for _, source := range moment.Sources {
			urls = append(urls, source.URL)
		}
		para, _, _ := strings.Cut(moment.Body, "\n\n")
		proposal.Stops = append(proposal.Stops, models.ItineraryStop{Material: models.ItineraryMaterial{RecordID: record.ID, ReviewVersion: record.ReviewVersion, PlaceID: stop.Place.ID, PlaceName: stop.Place.Name, CityID: stop.Place.CityID, Latitude: stop.Place.Latitude, Longitude: stop.Place.Longitude, MomentID: moment.ID, MomentTitle: moment.Title, Narrative: para, EvidenceKind: "curated_record", ContentSHA256: hex.EncodeToString(h[:]), SourceURLs: urls, EvidenceIDs: []string{}}, PlannedDwellMinutes: 3, PlannedNarrationMinutes: 2})
		proposal.TravelSequence = append(proposal.TravelSequence, i)
	}
	evidence, err := e.store.QueueSourceEvidence(ctx, b.ID, models.EvidenceRequest{Version: b.Version, URL: "https://example.org/package-proof", SourceKind: "page"}, "package-fixture", strings.Repeat("b", 64), "")
	if err != nil {
		t.Fatal(err)
	}
	result := models.EvidenceResult{Outcome: "inspected_page", FinalURL: "https://example.org/package-proof", Title: "PRIVATE-PACKAGE-EVIDENCE", Publisher: "Fixture", HTTPStatus: 200, ContentSHA256: strings.Repeat("a", 64), RetrievedAt: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Passages: []models.EvidencePassage{{Reference: "p1", Text: "PRIVATE-INSPECTED-PASSAGE"}}, Untrusted: true, MediaReuse: "unresolved"}
	raw, _ := json.Marshal(result)
	if _, err := e.store.Pool().Exec(ctx, `UPDATE source_evidence SET result=$2 WHERE id=$1`, evidence.ID, raw); err != nil {
		t.Fatal(err)
	}
	proposal.Stops[0].Material.EvidenceIDs = []string{evidence.ID}
	run, err := e.store.StartBriefResearch(ctx, b.ID, b.Version)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(proposal)
	for _, q := range []string{`UPDATE brief_research_stages SET status='completed' WHERE run_id=$1`, `UPDATE brief_research_runs SET status='completed' WHERE id=$1`} {
		if _, err := e.store.Pool().Exec(ctx, q, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE brief_research_stages SET output=$2 WHERE run_id=$1 AND stage='assembly'`, run.ID, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE route_briefs SET status='review',version=version+1 WHERE id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	e.qualifyFixtureRoutes(t, testdb.SeedTourID)
	return packageFixture{env: e, curator: curator, owner: owner, brief: b, run: run, proposal: proposal, evidence: evidence.ID, path: "/api/v1/admin/route-briefs/" + b.ID + "/review-package?tour_id=" + testdb.SeedTourID}
}
func decodePackage(t *testing.T, r resp) models.CuratorReviewPackage {
	t.Helper()
	if r.status != 200 {
		t.Fatalf("package HTTP %d %s", r.status, r.raw)
	}
	var p models.CuratorReviewPackage
	if err := json.Unmarshal(r.raw, &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func hasPackageTask(p models.CuratorReviewPackage, code string) bool {
	for _, task := range p.Tasks {
		if task.Code == code {
			return true
		}
	}
	return false
}

func TestCuratorPackageCompletePrivateStableAndReadOnly(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	counts := func() string {
		var s string
		if err := e.store.Pool().QueryRow(context.Background(), `SELECT concat((SELECT count(*) FROM tours),':',(SELECT count(*) FROM tour_revisions),':',(SELECT count(*) FROM candidates),':',(SELECT count(*) FROM source_evidence),':',(SELECT count(*) FROM jobs))`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := counts()
	r := e.get(t, f.path, f.curator)
	p := decodePackage(t, r)
	if !p.ReadyToPublish || p.State != "ready_for_review" || len(p.Version) != 64 || len(p.Tasks) != 0 || len(p.VisitorStories) != 4 || p.Routes["walk"].Geometry == nil || len(p.Evidence[0].Result.Passages) != 1 {
		t.Fatalf("incomplete package %+v", p)
	}
	if p.VisitorStories[0].TextSource != "curated_record" || len(strings.Fields(p.VisitorStories[0].Body)) < 250 || p.VisitorStories[0].Position != 1 {
		t.Fatal("full selected narrative missing")
	}
	if r.header.Get("Cache-Control") != "no-store" || r.header.Get("Vary") != "Authorization" {
		t.Fatal("private cache boundary absent")
	}
	repeated := decodePackage(t, e.get(t, f.path+"&expected_version="+p.Version, f.curator))
	if repeated.Version != p.Version || counts() != before {
		t.Fatal("GET mutated data or unstable token")
	}
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {f.owner, 403}, {e.signIn(t, "package-other@example.com", ""), 403}} {
		if got := e.get(t, f.path, tc.token); got.status != tc.status || strings.Contains(string(got.raw), "PRIVATE-") {
			t.Fatalf("private boundary %d %s", got.status, got.raw)
		}
	}
	admin := e.signIn(t, "admin@example.com", models.AccountRoleAdmin)
	if got := e.get(t, f.path, admin); got.status != 200 {
		t.Fatal("admin denied", got.status)
	}
	progress := e.get(t, "/api/v1/me/route-briefs/"+f.brief.ID+"/research", f.owner)
	if progress.status != 200 || strings.Contains(string(progress.raw), "PRIVATE-") || strings.Contains(string(progress.raw), "Source-linked private") {
		t.Fatal("package leaked into owner progress")
	}
}

func TestCuratorPackageMissingEvidenceAndStaleVersion(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	p := decodePackage(t, e.get(t, f.path, f.curator))
	ctx := context.Background()
	if _, err := e.store.Pool().Exec(ctx, `UPDATE moments SET title='Later curator title' WHERE id=$1`, f.proposal.Stops[0].Material.MomentID); err != nil {
		t.Fatal(err)
	}
	if got := e.get(t, f.path+"&expected_version="+p.Version, f.curator); got.status != 409 {
		t.Fatalf("stale exact version %d %s", got.status, got.raw)
	}
	changed := decodePackage(t, e.get(t, f.path, f.curator))
	if changed.ReadyToPublish || changed.State != "stale" || !hasPackageTask(changed, "story_changed") || changed.VisitorStories[0].TextSource != "captured_partial" {
		t.Fatal("stale story not visible", changed.Tasks)
	}
	f.proposal.Stops[0].Material.EvidenceIDs = []string{"00000000-0000-4000-8000-00000000ffff"}
	raw, _ := json.Marshal(f.proposal)
	if _, err := e.store.Pool().Exec(ctx, `UPDATE brief_research_stages SET output=$2 WHERE run_id=$1 AND stage='assembly'`, f.run.ID, raw); err != nil {
		t.Fatal(err)
	}
	missing := decodePackage(t, e.get(t, f.path, f.curator))
	if missing.ReadyToPublish || !hasPackageTask(missing, "evidence_missing") {
		t.Fatal("missing proof became ready")
	}
}

func TestCuratorPackageShowsNewEntitiesAndSharedContentDiffs(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	ctx := context.Background()
	target := f.proposal.Stops[0].Material.MomentID
	edit, err := e.store.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindMoment, TargetMomentID: &target, Payload: json.RawMessage(`{"title":"Proposed shared title","body":"Proposed replacement text remains private."}`)})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := e.store.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindPlace, Payload: json.RawMessage(`{"name":"New unselected research place"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE reconciliation_records SET moment_candidate_id=$2,place_candidate_id=$3 WHERE id=$1`, f.proposal.Stops[0].Material.RecordID, edit.ID, fresh.ID); err != nil {
		t.Fatal(err)
	}
	p := decodePackage(t, e.get(t, f.path, f.curator))
	var shared, newEntity bool
	for _, entry := range p.Entities {
		if entry.ID == edit.ID {
			shared = entry.Disposition == "proposed_shared_edit" && !entry.Required && len(entry.Changes) == 2 && string(entry.Current) != "null"
		}
		if entry.ID == fresh.ID {
			newEntity = entry.Disposition == "new_entity" && !entry.Required && entry.Candidate != nil
		}
	}
	if !shared || !newEntity || !p.ReadyToPublish {
		t.Fatalf("selected and contextual entities confused %+v", p)
	}
	unchanged, err := e.store.GetMoment(ctx, target, true)
	if err != nil || unchanged.Title == "Proposed shared title" {
		t.Fatal("GET applied a shared edit")
	}
}

func TestCuratorPackagePendingNoRunForeignRunAndMismatchedTour(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	ctx := context.Background()
	other, _, err := e.store.CreateRouteBrief(ctx, f.brief.OwnerAccountID, models.RouteBriefInput{Topic: "Other", Area: "Chicago"}, "")
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/route-briefs/" + other.ID + "/review-package"
	noRun := decodePackage(t, e.get(t, base, f.curator))
	if noRun.State != "not_started" || noRun.ReadyToPublish {
		t.Fatal("no-run falsely ready")
	}
	run, err := e.store.StartBriefResearch(ctx, other.ID, other.Version)
	if err != nil {
		t.Fatal(err)
	}
	pending := decodePackage(t, e.get(t, base, f.curator))
	if pending.State != "processing" || pending.ReadyToPublish || pending.Research.ID != run.ID {
		t.Fatal("processing package lost")
	}
	if got := e.get(t, base+"?run_id="+f.run.ID, f.curator); got.status != 404 {
		t.Fatal("foreign run associated", got.status)
	}
	if got := e.get(t, f.path+"&expected_version=bad", f.curator); got.status != 422 {
		t.Fatal(got.status)
	}
	if got := e.get(t, f.path+"&run_id=bad", f.curator); got.status != 404 {
		t.Fatal(got.status)
	}
	// A curator may inspect a mismatched existing tour, but it cannot be eligible.
	if _, err := e.store.Pool().Exec(ctx, `UPDATE tour_stops SET featured_moment_id=NULL WHERE tour_id=$1 AND position=1`, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	// Change the proposal's selected identity rather than allowing primary-story fallback to mask it.
	f.proposal.Stops[0].Material.MomentID = testdb.SeedMomentTalkiesID
	raw, _ := json.Marshal(f.proposal)
	if _, err := e.store.Pool().Exec(ctx, `UPDATE brief_research_stages SET output=$2 WHERE run_id=$1 AND stage='assembly'`, f.run.ID, raw); err != nil {
		t.Fatal(err)
	}
	mismatched := decodePackage(t, e.get(t, f.path, f.curator))
	if mismatched.ReadyToPublish || !hasPackageTask(mismatched, "tour_itinerary_mismatch") {
		t.Fatal("mismatched tour ready")
	}
}

func TestCuratorPackageBlocksWrongShapeFailedModesAndMediaChanges(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	ctx := context.Background()
	before := decodePackage(t, e.get(t, f.path, f.curator))
	f.proposal.TripShape = "loop"
	raw, _ := json.Marshal(f.proposal)
	if _, err := e.store.Pool().Exec(ctx, `UPDATE brief_research_stages SET output=$2 WHERE run_id=$1 AND stage='assembly'`, f.run.ID, raw); err != nil {
		t.Fatal(err)
	}
	loop := decodePackage(t, e.get(t, f.path, f.curator))
	if loop.ReadyToPublish || !hasPackageTask(loop, "tour_shape_mismatch") {
		t.Fatal("open geometry passed as a loop")
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE tour_routes SET status='failed' WHERE tour_id=$1 AND mode='bike'`, testdb.SeedTourID); err != nil {
		t.Fatal(err)
	}
	failed := decodePackage(t, e.get(t, f.path, f.curator))
	if !hasPackageTask(failed, "route_not_usable") || failed.Routes[models.TravelModeBike].Journey.TravelDurationS != nil {
		t.Fatal("failed mode retained a usable estimate")
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE media_assets SET license='rights-reserved' WHERE id=(SELECT hero_media_id FROM moments WHERE id=$1)`, f.proposal.Stops[0].Material.MomentID); err != nil {
		t.Fatal(err)
	}
	rights := decodePackage(t, e.get(t, f.path, f.curator))
	if !hasPackageTask(rights, "media_rights_incomplete") || rights.ReadyToPublish || rights.Version == before.Version {
		t.Fatal("changed media rights were ignored")
	}
}

func TestCuratorPackageInspectedProofAndCurrentBriefRequired(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	ctx := context.Background()
	if _, err := e.store.Pool().Exec(ctx, `UPDATE source_evidence SET result=jsonb_set(result,'{http_status}','500') WHERE id=$1`, f.evidence); err != nil {
		t.Fatal(err)
	}
	proof := decodePackage(t, e.get(t, f.path, f.curator))
	if !hasPackageTask(proof, "evidence_not_inspected") || proof.ReadyToPublish {
		t.Fatal("failed retrieval became inspected evidence")
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE route_briefs SET submitted=jsonb_set(submitted,'{topic}','"Later brief"'),version=version+1 WHERE id=$1`, f.brief.ID); err != nil {
		t.Fatal(err)
	}
	stale := decodePackage(t, e.get(t, f.path, f.curator))
	if stale.State != "stale" || !hasPackageTask(stale, "brief_changed") || stale.Submitted.Topic != "Memory" {
		t.Fatal("retained input replaced by a later brief")
	}
}

func TestCuratorPackageModeOrderDoesNotInvalidateSelection(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	f.proposal.Modes = []models.TravelMode{models.TravelModeBike, models.TravelModeWalk}
	raw, _ := json.Marshal(f.proposal)
	if _, err := e.store.Pool().Exec(context.Background(), `UPDATE brief_research_stages SET output=$2 WHERE run_id=$1 AND stage='assembly'`, f.run.ID, raw); err != nil {
		t.Fatal(err)
	}
	p := decodePackage(t, e.get(t, f.path, f.curator))
	if !p.ReadyToPublish || hasPackageTask(p, "tour_modes_mismatch") {
		t.Fatal("mode order changed mode availability", p.Tasks)
	}
}

func TestCuratorPackageChangedIdentityDecisionIsStale(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	if _, err := e.store.Pool().Exec(context.Background(), `UPDATE reconciliation_records SET review_version=review_version+1 WHERE id=$1`, f.proposal.Stops[0].Material.RecordID); err != nil {
		t.Fatal(err)
	}
	p := decodePackage(t, e.get(t, f.path, f.curator))
	if p.State != "stale" || p.ReadyToPublish || !hasPackageTask(p, "identity_changed") {
		t.Fatal("changed decision stayed ready")
	}
}

func TestCuratorPackagePreservesOptionalNarrationPublicationContract(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	ctx := context.Background()
	audio, err := e.store.CreateMedia(ctx, models.MediaAsset{Kind: models.MediaKindAudio, StorageKey: "package-fixture-narration", PublicURL: "https://example.org/generated.mp3", MimeType: "audio/mpeg", Caption: "Generated narration", Credit: "Fixture narration", License: "all-rights-reserved", Status: models.EntityStatusPublished})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetMomentNarration(ctx, f.proposal.Stops[0].Material.MomentID, &audio.ID); err != nil {
		t.Fatal(err)
	}
	e.qualifyFixtureRoutes(t, testdb.SeedTourID)
	p := decodePackage(t, e.get(t, f.path, f.curator))
	if !p.ReadyToPublish || hasPackageTask(p, "media_rights_incomplete") {
		t.Fatal("optional generated audio acquired a new image-reuse gate", p.Tasks)
	}
	found := false
	for _, entry := range p.Entities {
		if entry.Key == "media:"+audio.ID {
			found = !entry.Required && len(entry.Current) > 0
		}
	}
	if !found {
		t.Fatal("optional narration is not inspectable")
	}
}
