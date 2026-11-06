package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestRouteBriefHTTPRetriesAndLimits(t *testing.T) {
	e := newTestEnv(t)
	token := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	create := func(key, body string) (int, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/api/v1/admin/route-briefs", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Error(err)
			}
		}()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, data
	}
	body := `{"topic":"Labor","area":"Chicago"}`
	status, first := create("retry-me", body)
	if status != 201 {
		t.Fatalf("create: %d %+v", status, first)
	}
	status, again := create("retry-me", body)
	if status != 200 || again["id"] != first["id"] {
		t.Fatalf("retry: %d %+v", status, again)
	}
	if status, _ := create("retry-me", `{"topic":"Different","area":"Chicago"}`); status != 409 {
		t.Fatalf("key conflict: %d", status)
	}
	if status, _ := create(strings.Repeat("x", 129), body); status != 422 {
		t.Fatalf("key size: %d", status)
	}
	for range 9 {
		if status, _ := create("", body); status != 201 {
			t.Fatalf("quota setup: %d", status)
		}
	}
	if status, _ := create("", body); status != 429 {
		t.Fatalf("quota: %d", status)
	}
	if status, _ := create("retry-me", body); status != 200 {
		t.Fatalf("retry at quota: %d", status)
	}
}

func TestRouteBriefPrivacyAndDisabledIntake(t *testing.T) {
	e := newTestEnv(t)
	owner := e.signIn(t, "owner@example.com", "")
	other := e.signIn(t, "other@example.com", "")
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	ownerID := e.get(t, "/api/v1/me", owner).body["id"].(string)
	b, _, err := e.store.CreateRouteBrief(context.Background(), ownerID, models.RouteBriefInput{Topic: "Railroad history", Area: "Downers Grove", Notes: "Private unverified lead"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, path string
		status            int
	}{
		{"anonymous", "", "/me/route-briefs/" + b.ID, 401},
		{"owner", owner, "/me/route-briefs/" + b.ID, 200},
		{"other owner", other, "/me/route-briefs/" + b.ID, 404},
		{"curator", curator, "/admin/route-briefs/" + b.ID, 200},
		{"visitor admin", owner, "/admin/route-briefs/" + b.ID, 403},
		{"anonymous admin", "", "/admin/route-briefs", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := e.get(t, "/api/v1"+tc.path, tc.token)
			if r.status != tc.status {
				t.Fatalf("%d %s", r.status, r.raw)
			}
			if tc.status == 200 && r.header.Get("Cache-Control") != "no-store" {
				t.Fatal("private response must not be cached")
			}
		})
	}
	if r := e.get(t, "/api/v1/me/route-briefs", other); len(items(t, r)) != 0 {
		t.Fatalf("another owner's list: %s", r.raw)
	}
	if r := e.get(t, "/api/v1/me/route-briefs", owner); len(items(t, r)) != 1 {
		t.Fatalf("owner list: %s", r.raw)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		path := "/api/v1/me/route-briefs"
		if method == http.MethodPut {
			path += "/" + b.ID
		}
		r := e.do(t, method, path, owner, map[string]any{"topic": "Never public", "area": "Chicago"})
		if r.status != 403 || errCode(r) != "intake_disabled" {
			t.Fatalf("gate: %d %s", r.status, r.raw)
		}
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs", owner, map[string]any{"topic": "No", "area": "Chicago"}); r.status != 403 {
		t.Fatalf("visitor create: %d", r.status)
	}
	if r := e.do(t, http.MethodPut, "/api/v1/admin/route-briefs/"+b.ID, other, map[string]any{"version": 1}); r.status != 403 {
		t.Fatalf("visitor update: %d", r.status)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/route-briefs/"+b.ID+"/state", owner, map[string]any{"version": 1, "status": "review", "reason": "No"}); r.status != 403 {
		t.Fatalf("visitor state: %d", r.status)
	}
}

func TestRouteBriefLifecycleAndValidation(t *testing.T) {
	e := newTestEnv(t)
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	base := "/api/v1/admin/route-briefs"
	in := map[string]any{"topic": "Labor history", "area": "Chicagoland", "place_leads": []string{"Old factory, address unknown"}}
	r := e.do(t, http.MethodPost, base, curator, in)
	if r.status != 201 || r.body["status"] != "draft" || r.body["version"] != float64(1) {
		t.Fatalf("minimal: %d %s", r.status, r.raw)
	}
	id := r.body["id"].(string)
	path := base + "/" + id
	if strings.Contains(string(r.raw), "source_evidence") || strings.Contains(string(r.raw), "public_text") {
		t.Fatal("submitted prose must not become evidence or public text")
	}
	if r := e.do(t, http.MethodPost, path+"/state", curator, map[string]any{"version": 1, "status": "accepted", "reason": "Skip review"}); r.status != 409 {
		t.Fatalf("skipped review: %d %s", r.status, r.raw)
	}
	for i, status := range []string{"processing", "needs_research", "review", "accepted", "draft", "declined"} {
		if status == "needs_research" {
			if r := e.do(t, http.MethodPut, path, curator, map[string]any{"version": i + 1, "submitted": in}); r.status != 409 {
				t.Fatalf("edit during processing: %d", r.status)
			}
		}
		r = e.do(t, http.MethodPost, path+"/state", curator, map[string]any{"version": i + 1, "status": status, "reason": "Curator decision"})
		if r.status != 200 || r.body["status"] != status || r.body["reason"] != "Curator decision" {
			t.Fatalf("state %s: %d %s", status, r.status, r.raw)
		}
	}
	if r := e.do(t, http.MethodPut, path, curator, map[string]any{"version": 1, "submitted": in}); r.status != 409 {
		t.Fatalf("stale edit: %d %s", r.status, r.raw)
	}
	in["notes"] = "A curator's new lead"
	r = e.do(t, http.MethodPut, path, curator, map[string]any{"version": 7, "submitted": in})
	if r.status != 200 || r.body["status"] != "draft" || r.body["version"] != float64(8) {
		t.Fatalf("new review: %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, path+"/state", curator, map[string]any{"version": 7, "status": "review", "reason": "Stale decision"}); r.status != 409 {
		t.Fatalf("stale state: %d", r.status)
	}
	for _, tc := range []struct {
		name, field string
		input       map[string]any
	}{
		{"topic", "topic", map[string]any{"area": "Chicago"}},
		{"area", "area", map[string]any{"topic": "Labor"}},
		{"size", "notes", map[string]any{"topic": "Labor", "area": "Chicago", "notes": strings.Repeat("x", 10001)}},
		{"link", "source_links", map[string]any{"topic": "Labor", "area": "Chicago", "source_links": []string{"javascript:alert(1)"}}},
		{"credentials", "source_links", map[string]any{"topic": "Labor", "area": "Chicago", "source_links": []string{"https://user:secret@example.org/"}}},
		{"mode", "modes", map[string]any{"topic": "Labor", "area": "Chicago", "modes": []string{"fly"}}},
		{"duration", "duration_minutes", map[string]any{"topic": "Labor", "area": "Chicago", "duration_minutes": 0}},
		{"shape", "trip_shape", map[string]any{"topic": "Labor", "area": "Chicago", "trip_shape": "random"}},
		{"too many leads", "place_leads", map[string]any{"topic": "Labor", "area": "Chicago", "place_leads": strings.Fields(strings.Repeat("place ", 21))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := e.do(t, http.MethodPost, base, curator, tc.input)
			if r.status != 422 {
				t.Fatalf("%d %s", r.status, r.raw)
			}
			fields := r.body["error"].(map[string]any)["fields"].(map[string]any)
			if fields[tc.field] == nil {
				t.Fatalf("missing field error: %s", r.raw)
			}
		})
	}
	if r := e.do(t, http.MethodPost, path+"/state", curator, map[string]any{"version": 8, "status": "review"}); r.status != 422 {
		t.Fatalf("missing reason: %d", r.status)
	}
	if r := e.do(t, http.MethodPost, base, curator, map[string]any{"topic": "Labor", "area": "Chicago", "public_text": "untrusted"}); r.status != 400 {
		t.Fatalf("unknown field: %d", r.status)
	}
	// Acceptance is a private disposition and leaves the existing public catalogue unchanged.
	if r := e.get(t, "/api/v1/cities/downers-grove/tours", ""); len(items(t, r)) != 1 {
		t.Fatalf("intake changed public tours: %s", r.raw)
	}
}
