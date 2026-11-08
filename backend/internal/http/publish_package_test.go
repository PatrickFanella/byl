package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

// Use prepared drafts to exercise publishing dependencies, rather than only
// releasing a tour whose dependencies were already public.
func draftPackageDependencies(t *testing.T, f packageFixture) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`UPDATE media_assets SET status='draft'`,
		`UPDATE moments SET status='draft'`,
		`UPDATE places SET status='draft'`,
		`UPDATE tours SET status='draft' WHERE id=$1`,
	} {
		args := []any{}
		if strings.Contains(q, "$1") {
			args = append(args, testdb.SeedTourID)
		}
		if _, err := f.env.store.Pool().Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	f.env.qualifyFixtureRoutes(t, testdb.SeedTourID)
}

func TestPublishPackagePreparedDraftsAndLateFailureRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "late_failure"}[fail], func(t *testing.T) {
			f := newPackageFixture(t)
			draftPackageDependencies(t, f)
			e := f.env
			p := decodePackage(t, e.get(t, f.path, f.curator))
			if !p.ReadyToPublish {
				t.Fatalf("prepared draft not ready: %+v", p.Tasks)
			}
			var jobs, revisions int
			ctx := context.Background()
			if err := e.store.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM jobs),(SELECT count(*) FROM tour_revisions)`).Scan(&jobs, &revisions); err != nil {
				t.Fatal(err)
			}
			if fail {
				// Fail after dependencies and revision have been written. The whole
				// transaction, including queued narration, must roll back.
				_, err := e.store.Pool().Exec(ctx, `CREATE FUNCTION fail_package_acceptance() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected acceptance failure'; END $$;
				CREATE TRIGGER fail_acceptance BEFORE UPDATE ON route_briefs FOR EACH ROW WHEN (NEW.status='accepted') EXECUTE FUNCTION fail_package_acceptance()`)
				if err != nil {
					t.Fatal(err)
				}
			}
			in := models.PublishPackageRequest{Version: p.Version, RunID: f.run.ID, TourID: testdb.SeedTourID}
			r := e.do(t, "POST", "/api/v1/admin/route-briefs/"+f.brief.ID+"/review-package/publish", f.curator, in)
			if (!fail && r.status != 200) || (fail && r.status != 500) {
				t.Fatalf("publish %d %s", r.status, r.raw)
			}
			var status string
			if err := e.store.Pool().QueryRow(ctx, `SELECT status FROM places WHERE id=$1`, p.Tour.Stops[0].Place.ID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if fail {
				after := decodePackage(t, e.get(t, f.path, f.curator))
				var afterJobs, afterRevisions int
				if err := e.store.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM jobs),(SELECT count(*) FROM tour_revisions)`).Scan(&afterJobs, &afterRevisions); err != nil {
					t.Fatal(err)
				}
				if status != "draft" || p.Version != after.Version || jobs != afterJobs || revisions != afterRevisions {
					t.Fatalf("partial publication: %s jobs %d/%d revisions %d/%d", status, jobs, afterJobs, revisions, afterRevisions)
				}
			} else if status != "published" {
				t.Fatal("selected place stayed private")
			} else {
				for _, entity := range p.Entities {
					if !entity.Required || entity.Candidate != nil {
						continue
					}
					var current string
					var err error
					switch entity.Kind {
					case "place":
						err = e.store.Pool().QueryRow(ctx, `SELECT status FROM places WHERE id=$1`, entity.ID).Scan(&current)
					case "moment":
						err = e.store.Pool().QueryRow(ctx, `SELECT status FROM moments WHERE id=$1`, entity.ID).Scan(&current)
					case "media":
						err = e.store.Pool().QueryRow(ctx, `SELECT status FROM media_assets WHERE id=$1`, entity.ID).Scan(&current)
					}
					if err != nil || current != "published" {
						t.Fatalf("dependency %s: %s %v", entity.Key, current, err)
					}
				}
			}
		})
	}
}

func TestPublishPackageConcurrentApproval(t *testing.T) {
	f := newPackageFixture(t)
	p := decodePackage(t, f.env.get(t, f.path, f.curator))
	raw, err := json.Marshal(models.PublishPackageRequest{Version: p.Version, RunID: f.run.ID, TourID: testdb.SeedTourID})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	for range 2 {
		go func() {
			<-start
			req, err := http.NewRequest("POST", f.env.srv.URL+"/api/v1/admin/route-briefs/"+f.brief.ID+"/review-package/publish", bytes.NewReader(raw))
			if err != nil {
				results <- 0
				return
			}
			req.Header.Set("Authorization", "Bearer "+f.curator)
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- 0
				return
			}
			if err := res.Body.Close(); err != nil {
				results <- 0
				return
			}
			results <- res.StatusCode
		}()
	}
	close(start)
	counts := map[int]int{}
	for range 2 {
		counts[<-results]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent approvals: %v", counts)
	}
}

func TestPublishPackageExactReviewAndPrivateProvenance(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	p := decodePackage(t, e.get(t, f.path, f.curator))
	in := models.PublishPackageRequest{Version: p.Version, RunID: f.run.ID, TourID: testdb.SeedTourID, Note: "PRIVATE-APPROVAL-NOTE"}
	path := "/api/v1/admin/route-briefs/" + f.brief.ID + "/review-package/publish"
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {f.owner, 403}} {
		if r := e.do(t, "POST", path, tc.token, in); r.status != tc.status {
			t.Fatalf("authorization %d %s", r.status, r.raw)
		}
	}
	r := e.do(t, "POST", path, f.curator, in)
	if r.status != 200 {
		t.Fatalf("publish %d %s", r.status, r.raw)
	}
	if strings.Contains(string(r.raw), "PRIVATE-APPROVAL") || strings.Contains(string(r.raw), "_curator_package_approval") {
		t.Fatal("private approval leaked")
	}
	id := r.body["id"].(string)
	public := e.get(t, "/api/v1/tours/"+testdb.SeedTourID+"/revisions/"+id, "")
	if public.status != 200 || strings.Contains(string(public.raw), "PRIVATE-APPROVAL") || strings.Contains(string(public.raw), f.brief.ID) || strings.Contains(string(public.raw), "_curator_package_approval") {
		t.Fatalf("public revision exposed approval: %d %s", public.status, public.raw)
	}
	var version string
	if err := e.store.Pool().QueryRow(context.Background(), `SELECT snapshot->'_curator_package_approval'->>'version' FROM tour_revisions WHERE id=$1`, id).Scan(&version); err != nil || version != p.Version {
		t.Fatalf("provenance %s %v", version, err)
	}
	if r := e.do(t, "POST", path, f.curator, in); r.status != 409 {
		t.Fatalf("repeat %d %s", r.status, r.raw)
	}
	b, err := e.store.GetRouteBrief(context.Background(), f.brief.ID, "")
	if err != nil || b.Status != models.BriefAccepted {
		t.Fatalf("brief %+v %v", b, err)
	}
	if _, err := e.store.GetTourRevision(context.Background(), testdb.SeedTourID, *p.Tour.RevisionID); err != nil {
		t.Fatal("previous revision lost", err)
	}
}

func TestPublishPackageRejectsStaleOrIncompleteReview(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"changed story", `UPDATE moments SET title='Changed after review' WHERE id=$1`},
		{"missing rights", `UPDATE media_assets SET license='' WHERE id=(SELECT hero_media_id FROM moments WHERE id=$1)`},
		{"archived dependency", `UPDATE moments SET status='archived' WHERE id=$1`},
		{"failed route", `UPDATE tour_routes SET status='failed' WHERE tour_id=(SELECT tour_id FROM tour_stops WHERE featured_moment_id=$1 LIMIT 1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPackageFixture(t)
			e := f.env
			p := decodePackage(t, e.get(t, f.path, f.curator))
			if _, err := e.store.Pool().Exec(context.Background(), tc.sql, f.proposal.Stops[0].Material.MomentID); err != nil {
				t.Fatal(err)
			}
			path := "/api/v1/admin/route-briefs/" + f.brief.ID + "/review-package/publish"
			in := models.PublishPackageRequest{Version: p.Version, RunID: f.run.ID, TourID: testdb.SeedTourID}
			if r := e.do(t, "POST", path, f.curator, in); r.status != 409 {
				t.Fatalf("stale %d %s", r.status, r.raw)
			}
			p = decodePackage(t, e.get(t, f.path, f.curator))
			in.Version = p.Version
			if r := e.do(t, "POST", path, f.curator, in); r.status != 409 {
				t.Fatalf("incomplete %d %s", r.status, r.raw)
			}
		})
	}
}

func TestPublishPackageUnselectedSharedEditAndResearchStayPrivate(t *testing.T) {
	f := newPackageFixture(t)
	e := f.env
	ctx := context.Background()
	target := f.proposal.Stops[0].Material.MomentID
	candidate, err := e.store.CreateCandidate(ctx, models.Candidate{Kind: models.CandidateKindMoment, TargetMomentID: &target, Payload: json.RawMessage(`{"title":"Unselected shared replacement"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pool().Exec(ctx, `UPDATE reconciliation_records SET moment_candidate_id=$2 WHERE id=$1`, f.proposal.Stops[0].Material.RecordID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	p := decodePackage(t, e.get(t, f.path, f.curator))
	in := models.PublishPackageRequest{Version: p.Version, RunID: f.run.ID, TourID: testdb.SeedTourID}
	path := "/api/v1/admin/route-briefs/" + f.brief.ID + "/review-package/publish"
	if r := e.do(t, "POST", path, f.curator, in); r.status != 200 {
		t.Fatalf("publish %d %s", r.status, r.raw)
	}
	current, err := e.store.GetCandidate(ctx, candidate.ID)
	if err != nil || current.Status != models.CandidateStatusPending {
		t.Fatalf("unselected candidate changed: %+v %v", current, err)
	}
	moment, err := e.store.GetMoment(ctx, target, true)
	if err != nil || moment.Title != f.proposal.Stops[0].Material.MomentTitle {
		t.Fatalf("shared text overwritten: %+v %v", moment, err)
	}
	for _, status := range []string{"needs_research", "declined"} {
		if _, err := e.store.Pool().Exec(ctx, `UPDATE route_briefs SET status=$2 WHERE id=$1`, f.brief.ID, status); err != nil {
			t.Fatal(err)
		}
		p = decodePackage(t, e.get(t, f.path, f.curator))
		in.Version = p.Version
		if r := e.do(t, "POST", path, f.curator, in); r.status != 409 {
			t.Fatalf("%s package published: %d %s", status, r.status, r.raw)
		}
	}
}
