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

	"invisible-museum/backend/internal/models"
)

type httpEvidenceFetcher struct{}

func (httpEvidenceFetcher) RetrieveEvidence(context.Context, string, string) (models.EvidenceResult, error) {
	hash := sha256.Sum256([]byte("Private retained source text."))
	return models.EvidenceResult{ContentSHA256: hex.EncodeToString(hash[:]), Outcome: "inspected_page", Title: "Depot record", Publisher: "archive.example", Passages: []models.EvidencePassage{{Reference: "paragraph:1", Text: "Private retained source text."}}}, nil
}

func (e *testEnv) queueEvidence(t *testing.T, path, token, key string, body any) resp {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", key)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	raw, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return resp{status: response.StatusCode, body: data, raw: raw, header: response.Header}
}

func TestSourceEvidenceCuratorContractAndPrivatePassages(t *testing.T) {
	e := newTestEnv(t)
	e.worker.Evidence = httpEvidenceFetcher{}
	curator := e.signIn(t, "curator@example.com", models.AccountRoleCurator)
	owner := e.signIn(t, "owner@example.com", "")
	ownerID := e.get(t, "/api/v1/me", owner).body["id"].(string)
	b, _, err := e.store.CreateRouteBrief(context.Background(), ownerID, models.RouteBriefInput{Topic: "Workers", Area: "Downers Grove"}, "")
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/route-briefs/" + b.ID + "/evidence"
	body := map[string]any{"version": 1, "url": "https://archive.example/history#page=4"}
	for _, tc := range []struct {
		name, token string
		status      int
	}{{"anonymous", "", 401}, {"owner", owner, 403}} {
		t.Run(tc.name, func(t *testing.T) {
			if r := e.do(t, http.MethodPost, base, tc.token, body); r.status != tc.status {
				t.Fatalf("create: %d", r.status)
			}
			if r := e.get(t, base, tc.token); r.status != tc.status {
				t.Fatalf("read: %d", r.status)
			}
		})
	}
	r := e.queueEvidence(t, base, curator, "one-request", body)
	if r.status != 202 || r.body["result"] != nil || r.body["reference"] != "page=4" {
		t.Fatalf("queue: %d %s", r.status, r.raw)
	}
	id := r.body["id"].(string)
	jobID := r.body["job_id"]
	retry := e.queueEvidence(t, base, curator, "one-request", body)
	if retry.status != 202 || retry.body["id"] != id || retry.body["job_id"] != jobID {
		t.Fatalf("retry duplicated work: %s", retry.raw)
	}
	e.drain(t)
	r = e.get(t, base+"/"+id, curator)
	if r.status != 200 || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("get: %d %s", r.status, r.raw)
	}
	result := r.body["result"].(map[string]any)
	if result["outcome"] != "inspected_page" || result["untrusted"] != true || result["media_reuse"] != "unresolved" || !strings.Contains(string(r.raw), "Private retained source text.") {
		t.Fatalf("provenance: %s", r.raw)
	}
	if r := e.get(t, base+"/"+id, owner); r.status != 403 {
		t.Fatalf("owner read private research: %d", r.status)
	}
	if r := e.get(t, "/api/v1/admin/route-briefs/00000000-0000-4000-8000-000000000000/evidence/"+id, curator); r.status != 404 {
		t.Fatalf("foreign brief evidence: %d", r.status)
	}
	if r := e.get(t, base, curator); r.status != 200 || len(items(t, r)) != 1 {
		t.Fatalf("list: %d %s", r.status, r.raw)
	}
	body["url"] = "https://archive.example/different"
	if r := e.queueEvidence(t, base, curator, "one-request", body); r.status != 409 {
		t.Fatalf("key conflict: %d", r.status)
	}
	body["version"] = 2
	if r := e.queueEvidence(t, base, curator, "new-request", body); r.status != 409 {
		t.Fatalf("stale version: %d", r.status)
	}
	for _, tc := range []struct {
		field string
		body  map[string]any
	}{
		{"url", map[string]any{"version": 1, "url": "http://127.0.0.1/private"}},
		{"url", map[string]any{"version": 1, "url": "https://user:secret@example.org/"}},
		{"url", map[string]any{"version": 1, "url": "http://example.org:2375/"}},
		{"source_kind", map[string]any{"version": 1, "url": "https://example.org/", "source_kind": "search_snippet"}},
		{"version", map[string]any{"url": "https://example.org/"}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			r := e.queueEvidence(t, base, curator, "", tc.body)
			if r.status != 422 {
				t.Fatalf("%d %s", r.status, r.raw)
			}
			if r.body["error"].(map[string]any)["fields"].(map[string]any)[tc.field] == nil {
				t.Fatalf("missing field error: %s", r.raw)
			}
		})
	}
}
