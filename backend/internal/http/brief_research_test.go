package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestBriefResearchAPIPrivacyAndLifecycle(t *testing.T) {
	e := newTestEnv(t)
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	owner := e.signIn(t, "owner@example.com", "")
	other := e.signIn(t, "other@example.com", "")
	ownerID := e.get(t, "/api/v1/me", owner).body["id"].(string)
	b, _, err := e.store.CreateRouteBrief(context.Background(), ownerID, models.RouteBriefInput{Topic: "Workers", Area: "Chicagoland", Notes: "Sensitive submitted notes"}, "")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/admin/route-briefs/" + b.ID + "/research"
	if r := e.do(t, http.MethodPost, path, owner, map[string]any{"version": 1}); r.status != 403 {
		t.Fatalf("visitor start: %d", r.status)
	}
	r := e.do(t, http.MethodPost, path, curator, map[string]any{"version": 1})
	if r.status != 202 || r.body["status"] != "active" {
		t.Fatalf("start: %d %s", r.status, r.raw)
	}
	id := r.body["id"].(string)
	jobID := r.body["job_id"].(string)
	if retry := e.do(t, http.MethodPost, path, curator, map[string]any{"version": 1}); retry.status != 202 || retry.body["id"] != id {
		t.Fatalf("retried start: %d %s", retry.status, retry.raw)
	}
	if r := e.do(t, http.MethodPost, path, curator, map[string]any{"version": 0}); r.status != 422 {
		t.Fatalf("version: %d", r.status)
	}
	for _, tc := range []struct {
		name, token, prefix string
		status              int
	}{
		{"anonymous", "", "me", 401}, {"owner", owner, "me", 200}, {"other owner", other, "me", 404},
		{"curator", curator, "admin", 200}, {"visitor admin", owner, "admin", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := e.get(t, "/api/v1/"+tc.prefix+"/route-briefs/"+b.ID+"/research", tc.token)
			if r.status != tc.status {
				t.Fatalf("%d %s", r.status, r.raw)
			}
			if strings.Contains(string(r.raw), "Sensitive submitted notes") || strings.Contains(string(r.raw), `"output"`) {
				t.Fatal("research progress exposed private source content")
			}
			if r.status == 200 && r.header.Get("Cache-Control") != "no-store" {
				t.Fatal("private progress cached")
			}
		})
	}
	if r := e.do(t, http.MethodPost, path+"/cancel", owner, map[string]any{"version": 2, "run_id": id}); r.status != 403 {
		t.Fatalf("visitor cancel: %d", r.status)
	}
	if r := e.do(t, http.MethodPost, path+"/cancel", curator, map[string]any{"version": 2}); r.status != 422 {
		t.Fatalf("missing run: %d", r.status)
	}
	if r := e.do(t, http.MethodPost, path+"/cancel", curator, map[string]any{"version": 2, "run_id": id}); r.status != 202 || r.body["status"] != "cancelled" {
		t.Fatalf("cancel: %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, path+"/resume", curator, map[string]any{"version": 2, "run_id": id}); r.status != 409 {
		t.Fatalf("stale resume: %d", r.status)
	}
	r = e.do(t, http.MethodPost, path+"/resume", curator, map[string]any{"version": 3, "run_id": id})
	if r.status != 202 || r.body["job_id"] == jobID {
		t.Fatalf("resume: %d %s", r.status, r.raw)
	}
	if r := e.do(t, http.MethodPost, "/api/v1/admin/jobs/"+r.body["job_id"].(string)+"/retry", curator, nil); r.status != 409 {
		t.Fatalf("generic retry must retain research budgets: %d %s", r.status, r.raw)
	}
	e.drain(t)
	r = e.get(t, path, curator)
	if r.status != 200 || r.body["status"] != "needs_research" || r.body["reason"] != "processor_unavailable" || r.body["requests_used"] != float64(0) {
		t.Fatalf("unconfigured processor: %d %s", r.status, r.raw)
	}
	if r := e.get(t, "/api/v1/admin/route-briefs/"+b.ID, curator); r.body["status"] != "needs_research" {
		t.Fatalf("stranded brief: %s", r.raw)
	}
}
