package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"invisible-museum/backend/internal/auth"
	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/jobs"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/sources"
	"invisible-museum/backend/internal/testdb"
)

type testEnv struct {
	srv    *httptest.Server
	store  *repository.Store
	worker *jobs.Worker
	blob   blob.Store
	fsDir  string
}

func newTestEnv(t *testing.T) *testEnv { return newTestEnvWith(t) }

func newTestEnvWith(t *testing.T, opts ...func(*Deps)) *testEnv {
	t.Helper()
	store := repository.New(testdb.Open(t))
	dir := t.TempDir()
	fs, err := blob.NewFS(dir, "http://media.test/files")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(store, time.Hour)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := Deps{
		Store:          store,
		Sessions:       sessions,
		SessionManager: sessions,
		DevAuth:        true,
		IsBootstrap:    func(email string) bool { return email == "admin@example.com" },
		Blob:           fs,
		MediaFSDir:     dir,
		Sources:        sources.NewRegistry(sources.NewHTTPClient("InvisibleMuseum/test"), nil),
		Log:            quiet,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	srv := httptest.NewServer(NewRouter(deps))
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, store: store, worker: jobs.New(store, routing.NewStraightLine(), narration.None{}, fs, 1, quiet), blob: fs, fsDir: dir}
}

// drain runs every queued job.
func (e *testEnv) drain(t *testing.T) {
	t.Helper()
	for {
		processed, err := e.worker.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			return
		}
	}
}

type resp struct {
	status int
	body   map[string]any
	raw    []byte
	header http.Header
}

func (e *testEnv) do(t *testing.T, method, path, token string, payload any) resp {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw, header: res.Header}
	if len(raw) > 0 && strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(raw, &out.body)
	}
	return out
}

func (e *testEnv) get(t *testing.T, path, token string) resp {
	return e.do(t, http.MethodGet, path, token, nil)
}

// signIn creates a dev session; the bootstrap admin email gets the admin role.
func (e *testEnv) signIn(t *testing.T, email string, role models.AccountRole) string {
	t.Helper()
	r := e.do(t, http.MethodPost, "/api/v1/auth/dev", "", map[string]string{"email": email, "display_name": "Test"})
	if r.status != 200 {
		t.Fatalf("dev sign-in %s: %d %s", email, r.status, r.raw)
	}
	token := r.body["token"].(string)
	acct := r.body["account"].(map[string]any)
	if role != "" && role != models.AccountRole(acct["role"].(string)) {
		if err := e.store.SetRole(context.Background(), acct["id"].(string), role); err != nil {
			t.Fatal(err)
		}
	}
	return token
}

func items(t *testing.T, r resp) []any {
	t.Helper()
	list, ok := r.body["items"].([]any)
	if !ok {
		t.Fatalf("no items in %s", r.raw)
	}
	return list
}

func errCode(r resp) string {
	e, _ := r.body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func errMessage(r resp) string {
	e, _ := r.body["error"].(map[string]any)
	msg, _ := e["message"].(string)
	return msg
}
