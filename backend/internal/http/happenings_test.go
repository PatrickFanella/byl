package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
)

func (e *testEnv) happeningRequest(t *testing.T, method, path, token, key string, payload any) resp {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Body.Close() }()
	raw, err = io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	out := resp{status: r.StatusCode, raw: raw, header: r.Header}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func happeningHTTPFixture(t *testing.T, enabled bool) (*testEnv, string, string, models.HappeningWriteRequest) {
	t.Helper()
	e := newTestEnvWith(t, func(d *Deps) { d.HappeningsImport = enabled })
	curator := e.signIn(t, "happening-curator@example.com", models.AccountRoleCurator)
	visitor := e.signIn(t, "happening-visitor@example.com", "")
	actor := e.get(t, "/api/v1/me", curator).body["id"].(string)
	ctx := context.Background()
	tour, err := e.store.GetTour(ctx, testdb.SeedTourID, false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := e.store.CreateRouteBrief(ctx, actor, models.RouteBriefInput{Topic: "Synthetic organizer notice", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	proof, err := e.store.QueueSourceEvidence(ctx, b.ID, models.EvidenceRequest{Version: b.Version, URL: "https://organizer.example/recorded-notice", SourceKind: "page"}, "page", strings.Repeat("a", 64), "http-fixture")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	quote := "PRIVATE-HAPPENING-PASSAGE: Fixture Parks invites visitors to a synthetic history talk at Main Street Station on " + day.AddDate(0, 0, 2).Format("2006-01-02") + "."
	digest := sha256.Sum256([]byte(quote))
	result := models.EvidenceResult{Outcome: "inspected_page", FinalURL: proof.URL, Title: "Synthetic organizer announcement", Publisher: "Fixture Parks", RetrievedAt: now.Add(-time.Minute), HTTPStatus: 200, ContentSHA256: hex.EncodeToString(digest[:]), Passages: []models.EvidencePassage{{Reference: "p1", Text: quote}}, Untrusted: true, MediaReuse: "unresolved"}
	raw, _ := json.Marshal(result)
	if _, err := e.store.Pool().Exec(ctx, `UPDATE source_evidence SET result=$2 WHERE id=$1`, proof.ID, raw); err != nil {
		t.Fatal(err)
	}
	claims := []models.HappeningCitation{}
	for _, field := range []string{"title", "summary", "organizer", "occurrence", "place"} {
		claims = append(claims, models.HappeningCitation{Field: field, EvidenceID: proof.ID, Passage: "p1", Quote: quote})
	}
	in := models.HappeningWriteRequest{Input: models.HappeningInput{PlaceID: tour.Stops[0].Place.ID, Title: "Synthetic history talk", Summary: "A synthetic source-reviewed occurrence.", Organizer: "Fixture Parks", StartsOn: &models.Date{Time: day.AddDate(0, 0, 2)}, EndsOn: &models.Date{Time: day.AddDate(0, 0, 2)}, State: "scheduled", ReviewOn: &models.Date{Time: day.AddDate(0, 0, 1)}, ExpiresOn: &models.Date{Time: day.AddDate(0, 0, 3)}, EvidenceIDs: []string{proof.ID}, Claims: claims}}
	return e, curator, visitor, in
}

func TestHappeningHTTPDefaultOffAuthorizationAndPrivateReview(t *testing.T) {
	e, curator, visitor, in := happeningHTTPFixture(t, false)
	base := "/api/v1/admin/happenings"
	if r := e.happeningRequest(t, "POST", base, "", "happening-post-key", in); r.status != 401 {
		t.Fatal("anonymous import", r.status)
	}
	if r := e.happeningRequest(t, "POST", base, visitor, "happening-post-key", in); r.status != 403 {
		t.Fatal("direct visitor prose imported", r.status)
	}
	if r := e.happeningRequest(t, "POST", base, curator, "happening-post-key", in); r.status != 403 || r.body["error"].(map[string]any)["code"] != "happenings_import_disabled" {
		t.Fatal("default import gate", r.status)
	}
	if r := e.get(t, base+"/capabilities", curator); r.status != 200 || r.body["import_enabled"] != false || r.body["automatic_discovery"] != false {
		t.Fatal(r.status, r.body)
	}
	// Prepare only this isolated fixture through the repository, so that turning
	// import off does not prevent an existing curator draft's review/cancellation.
	actor := e.get(t, "/api/v1/me", curator).body["id"].(string)
	d, err := e.store.SaveHappeningDraft(context.Background(), "", actor, "disabled-existing-key", in, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", visitor} {
		if r := e.get(t, base+"/"+d.ID+"/review-package", token); r.status != 401 && r.status != 403 {
			t.Fatal("private proof leaked", r.status)
		}
	}
	r := e.get(t, base+"/"+d.ID+"/review-package", curator)
	if r.status != 200 || r.body["ready_to_publish"] != true {
		t.Fatalf("package %d %s", r.status, r.raw)
	}
	decision := e.happeningRequest(t, "POST", base+"/"+d.ID+"/review", curator, "", models.HappeningReviewRequest{Version: r.body["version"].(string), Action: "approve", Note: "Explicit synthetic source and occurrence review"})
	if decision.status != 200 {
		t.Fatalf("review while import off %d %s", decision.status, decision.raw)
	}
	public := e.get(t, "/api/v1/places/"+in.Input.PlaceID+"/happenings", "")
	if public.status != 200 || len(public.body["items"].([]any)) != 1 || public.header.Get("Cache-Control") != "no-store" || bytes.Contains(public.raw, []byte("PRIVATE-HAPPENING-PASSAGE")) {
		t.Fatal("public notice/privacy", public.status, string(public.raw))
	}
}

func TestHappeningHTTPStrictBodyStaleReviewAndPublicCursor(t *testing.T) {
	e, curator, _, in := happeningHTTPFixture(t, true)
	base := "/api/v1/admin/happenings"
	for _, payload := range []any{map[string]any{"version": 0, "input": in.Input, "unexpected": "PRIVATE-INPUT-MARKER"}, map[string]any{"input": strings.Repeat("PRIVATE-INPUT-MARKER", 2000)}} {
		r := e.happeningRequest(t, "POST", base, curator, "strict-happening-key", payload)
		if r.status != 400 || bytes.Contains(r.raw, []byte("PRIVATE-INPUT-MARKER")) {
			t.Fatal("strict input boundary", r.status)
		}
	}
	if r := e.happeningRequest(t, "POST", base+"?extra=private", curator, "strict-happening-key", in); r.status != 400 {
		t.Fatal("write query accepted", r.status)
	}
	if r := e.happeningRequest(t, "POST", base, curator, "", in); r.status != 422 {
		t.Fatal("missing retry identity accepted", r.status)
	}
	create := func(key string) string {
		r := e.happeningRequest(t, "POST", base, curator, key, in)
		if r.status != 201 {
			t.Fatalf("create %d %s", r.status, r.raw)
		}
		return r.body["id"].(string)
	}
	id := create("first-happening-key")
	p := e.get(t, base+"/"+id+"/review-package", curator)
	in.Version = 1
	in.Input.Title = "Curator's changed synthetic title"
	if r := e.happeningRequest(t, "PUT", base+"/"+id, curator, "updated-happening-key", in); r.status != 200 {
		t.Fatalf("update %d %s", r.status, r.raw)
	}
	if r := e.happeningRequest(t, "POST", base+"/"+id+"/review", curator, "", models.HappeningReviewRequest{Version: p.body["version"].(string), Action: "approve", Note: "Stale read set"}); r.status != 409 {
		t.Fatal("stale package published", r.status)
	}
	approve := func(id string) {
		r := e.get(t, base+"/"+id+"/review-package", curator)
		if r.status != 200 {
			t.Fatal(r.status)
		}
		r = e.happeningRequest(t, "POST", base+"/"+id+"/review", curator, "", models.HappeningReviewRequest{Version: r.body["version"].(string), Action: "approve", Note: "Explicit synthetic latest package review"})
		if r.status != 200 {
			t.Fatalf("approve %d %s", r.status, r.raw)
		}
	}
	approve(id)
	in.Version = 0
	other := create("second-happening-key")
	approve(other)
	path := "/api/v1/places/" + in.Input.PlaceID + "/happenings?limit=1"
	first := e.get(t, path, "")
	if first.status != 200 || first.body["next_cursor"] != "1" {
		t.Fatal("pagination", first.status, first.body)
	}
	second := e.get(t, path+"&cursor=1", "")
	if second.status != 200 || len(second.body["items"].([]any)) != 1 || second.body["items"].([]any)[0].(map[string]any)["id"] == first.body["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("cursor repeated/skipped page", second.status)
	}
	if r := e.get(t, path+"&cursor=private", ""); r.status != 422 {
		t.Fatal("invalid cursor accepted", r.status)
	}
}
