package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestVisitorSuggestionIntakeAndPrivateStatus(t *testing.T) {
	e := newTestEnvWith(t, func(d *Deps) { d.BriefIntake = true })
	owner := e.signIn(t, "suggestion-owner@example.com", "")
	other := e.signIn(t, "suggestion-other@example.com", "")
	curator := e.signIn(t, "suggestion-curator@example.com", models.AccountRoleCurator)
	if r := e.get(t, "/api/v1/route-brief-intake", ""); r.status != 200 || r.body["enabled"] != true || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("capability %s", r.raw)
	}
	create := func(token, key, body string) (int, map[string]any, http.Header) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/api/v1/me/route-briefs", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Idempotency-Key", key)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Body.Close() }()
		var bodyJSON map[string]any
		if err := json.NewDecoder(r.Body).Decode(&bodyJSON); err != nil {
			t.Fatal(err)
		}
		return r.StatusCode, bodyJSON, r.Header
	}
	const body = `{"topic":"Unverified workers' history","area":"A region, no local IDs","source_links":["https://source.example/lead"]}`
	if code, _, _ := create("", "", body); code != 401 {
		t.Fatalf("anonymous submission %d", code)
	}
	if code, _, _ := create(owner, "", `{"topic":"","area":""}`); code != 422 {
		t.Fatalf("minimal validation %d", code)
	}
	if code, _, _ := create(owner, "key with spaces", body); code != 422 {
		t.Fatalf("public retry key validation %d", code)
	}
	if code, _, _ := create(owner, "", `{"topic":"Lead","area":"Here","status":"accepted"}`); code != 400 {
		t.Fatalf("publication field injection %d", code)
	}
	var before, after string
	query := `SELECT json_build_array((SELECT count(*) FROM tours),(SELECT count(*) FROM jobs),(SELECT count(*) FROM candidates))::text`
	if err := e.store.Pool().QueryRow(context.Background(), query).Scan(&before); err != nil {
		t.Fatal(err)
	}
	code, first, headers := create(owner, "same-suggestion", body)
	if code != 201 || first["status"] != "draft" || headers.Get("Cache-Control") != "no-store" || !strings.Contains(headers.Get("Vary"), "Authorization") {
		t.Fatalf("suggestion %d %+v", code, first)
	}
	code, again, _ := create(owner, "same-suggestion", body)
	if code != 200 || again["id"] != first["id"] {
		t.Fatal("lost-response retry duplicated submission")
	}
	if code, _, _ := create(owner, "same-suggestion", `{"topic":"Changed lead","area":"Here"}`); code != 409 {
		t.Fatalf("changed retry body %d", code)
	}
	if err := e.store.Pool().QueryRow(context.Background(), query).Scan(&after); err != nil || before != after {
		t.Fatal("submission changed tours, jobs or candidates")
	}
	id := first["id"].(string)
	if r := e.get(t, "/api/v1/me/route-briefs/"+id, other); r.status != 404 || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("foreign status %d %s", r.status, r.raw)
	}
	if r := e.get(t, "/api/v1/me/route-briefs", other); len(items(t, r)) != 0 {
		t.Fatal("foreign submission listed")
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs/"+id+"/state", owner, map[string]any{"version": 1, "status": "review", "reason": "Bypass"}); r.status != 403 {
		t.Fatal("visitor obtained curator authority")
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs/"+id+"/state", curator, map[string]any{"version": 1, "status": "declined", "reason": "No source supports this proposed route."}); r.status != 200 {
		t.Fatal(r.raw)
	}
	if r := e.get(t, "/api/v1/me/route-briefs/"+id, owner); r.status != 200 || r.body["status"] != "declined" || r.body["reason"] != "No source supports this proposed route." {
		t.Fatal("owner cannot read decision")
	}
	for range 9 {
		if code, _, _ := create(owner, "", body); code != 201 {
			t.Fatalf("quota setup %d", code)
		}
	}
	if code, _, _ := create(owner, "", body); code != 429 {
		t.Fatalf("public quota %d", code)
	}
	if code, again, _ := create(owner, "same-suggestion", body); code != 200 || again["id"] != id {
		t.Fatal("retry at quota failed")
	}
}

func TestClosedSuggestionIntakeLeavesOwnerStatusReadable(t *testing.T) {
	e := newTestEnv(t)
	owner := e.signIn(t, "closed-suggestion@example.com", "")
	ownerID := e.get(t, "/api/v1/me", owner).body["id"].(string)
	b, _, err := e.store.CreateRouteBrief(context.Background(), ownerID, models.RouteBriefInput{Topic: "Previously submitted", Area: "Here"}, "previous")
	if err != nil {
		t.Fatal(err)
	}
	if r := e.get(t, "/api/v1/route-brief-intake", ""); r.body["enabled"] != false {
		t.Fatal("intake default opened")
	}
	if r := e.do(t, http.MethodPost, "/api/v1/me/route-briefs", owner, map[string]any{"topic": "New", "area": "Here"}); r.status != 403 || errCode(r) != "intake_disabled" {
		t.Fatal("closed server accepted suggestion")
	}
	if r := e.get(t, "/api/v1/me/route-briefs/"+b.ID, owner); r.status != 200 {
		t.Fatal("closing intake hid past status")
	}
}
