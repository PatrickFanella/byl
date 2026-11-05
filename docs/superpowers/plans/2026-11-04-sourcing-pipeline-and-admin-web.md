# Sourcing Pipeline and Admin Web Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fill the review queue from open and local archives, draft moments with Claude or any OpenAI-compatible model, rank moments for a tour topic, and ship the admin web app curators use to review and publish.

**Architecture:** Two new Go packages (`internal/sources`, `internal/llm`) behind interfaces, two new job kinds plus `fetch_media`, four admin endpoints, and a SvelteKit static app in `web/` typed from the OpenAPI document and served by nginx in compose.

**Tech Stack:** Go 1.25, `github.com/anthropics/anthropic-sdk-go/v2`, SvelteKit 2 / Svelte 5 / TypeScript / Tailwind 4 / MapLibre GL, `openapi-typescript`, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-04-sourcing-pipeline-and-admin-web-design.md`

## Global Constraints

- Go 1.25 in go.mod, `golang:1.25-alpine` in the Dockerfile, `go-version: "1.25"` in CI. Existing dependency pins stay.
- All archive HTTP goes through `sources.NewHTTPClient(userAgent)`; handlers never call adapters directly.
- Every LLM response is schema-validated after decoding; one retry, then error.
- Nothing is published by any job or adapter.
- The OpenAPI contract test must stay green after every route change.
- Commit after every task.

## Review Focus

1. An adapter returning the same external id twice in one run, or across runs: exactly one candidate exists afterwards. Pinned in Task 4.
2. A draft whose model output names a topic slug not in the offered list: the slug is dropped, the draft is kept, the note says so. Pinned in Task 5.
3. `LLM_PROVIDER` unset: draft and suggest endpoints return 503, the draft job fails once without retry, nothing is enqueued forever. Pinned in Task 6.
4. A `url` adapter page that redirects to another host or returns HTML over 2 MiB: no candidate, no crash, the job records the skip. Pinned in Task 3.
5. Web: a visitor account signing in sees a clear "needs curator role" screen, never a blank page or a loop. Pinned in Task 9.

---

### Task 1: Go 1.25 and the LLM package

**Files:** `backend/go.mod`, `backend/Dockerfile`, `.gitea/workflows/ci.yml`; create `backend/internal/llm/{llm.go,prompt.go,schema.go,anthropic.go,openai.go,none.go}` + `anthropic_test.go`, `openai_test.go`, `schema_test.go`; modify `backend/internal/config/config.go` (`LocalArchiveURLs []string`, `SourcesUserAgent string`), `backend/internal/providers/providers.go` (`Drafter llm.Drafter`).

**Interfaces:** spec §4 verbatim. `llm.NewAnthropic(apiKey, baseURL, model string) Drafter`, `llm.NewOpenAI(baseURL, apiKey, model string, client *http.Client) Drafter`, `llm.None{}` whose methods return `llm.ErrUnavailable`. `llm.MomentDraftSchema` and `llm.RankingSchema` are `map[string]any` JSON schemas with `additionalProperties:false`. `llm.ValidateDraft(MomentDraft, offeredSlugs []string) (MomentDraft, []string dropped)`.

- [ ] Tests: OpenAI client posts `response_format.json_schema.strict=true` with the schema and `Authorization: Bearer`; decodes `choices[0].message.content` JSON; a malformed first response followed by a valid one succeeds with two requests; two malformed fail. Anthropic client (against `httptest` via `option.WithBaseURL`) sends `output_config.format.type == "json_schema"`, the model, and decodes the text block. `ValidateDraft` drops unknown slugs and invalid precision.
- [ ] Run → fail. Implement. Run → PASS. `go build ./...` on 1.25.
- [ ] Commit: `feat(backend): llm drafters for Claude and OpenAI-compatible endpoints; Go 1.25`.

### Task 2: Sources package core and Wikipedia / Commons adapters

**Files:** create `backend/internal/sources/{sources.go,httpclient.go,registry.go,license.go,wikipedia.go,commons.go}` + tests and `testdata/{wikipedia_geosearch.json,wikipedia_pages.json,commons_geosearch.json,commons_imageinfo.json}`.

**Interfaces:** spec §3 `Request`, `Found`, `Adapter`, `Registry{Get(name) (Adapter, bool); List() []Info}`; `Info{Name, Description string; SupportsQuery, SupportsNear bool}`; `NewHTTPClient(ua string) *http.Client` (20 s timeout, UA transport, one retry on 429/5xx); `MapLicense(shortName string) string`.

- [ ] Tests: wikipedia adapter against fixtures yields a place and a moment find per page with the expected external ids, coordinates, and `CC-BY-SA-4.0`; commons yields media finds with mapped licences and an empty licence for an unknown short name; the HTTP client retries once on 429 and sends the UA.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): source adapters for Wikipedia and Wikimedia Commons`.

### Task 3: Internet Archive, Library of Congress, and URL adapters

**Files:** `backend/internal/sources/{archive.go,loc.go,url.go,html.go}` + tests and fixtures `testdata/{ia_search.json,loc_search.json,local_page.html}`.

- [ ] Tests: IA texts become moments and images media with the download URL; LOC photo becomes media with the no-known-restrictions licence only when the rights text says so; URL adapter strips nav and scripts, caps at 20,000 chars, filters by `Query`, skips a page over 2 MiB and a redirect to another host (pins Review Focus 4), and uses `url:<sha256>` ids.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): Internet Archive, Library of Congress, and local page adapters`.

### Task 4: Import job and media fetch job

**Files:** `backend/internal/jobs/{import_job.go,fetch_media_job.go}` + tests; `backend/internal/models/review.go` (payload structs `ImportSourcePayload`, `DraftCandidatePayload`, `FetchMediaPayload`, job kind constants); `backend/internal/repository/candidates.go` (`CandidateExists(source, externalID) (bool, error)`, `CandidateFilter.Origin`, `CandidateFilter.Source`); `backend/internal/repository/places.go` (`NearestPublishedPlace(cityID string, pt geo.Point, maxM float64) (*models.PlaceSummary, error)`); `backend/internal/jobs/worker.go` (registry + dispatch).

- [ ] Tests: import with a fake adapter creates one candidate per find, assigns `target_place_id` to the nearest place within 150 m, skips duplicates on a second run (pins Review Focus 1), records "imported N, skipped M"; fetch_media downloads from an `httptest` server into the FS store and fills `storage_key`/`public_url`.
- [ ] Run → fail. Implement. Run → PASS.
- [ ] Commit: `feat(backend): import and media fetch jobs`.

### Task 5: Draft job and ranking

**Files:** `backend/internal/jobs/draft_job.go` + tests; `backend/internal/repository/topics.go` (`TopicsBySlugs(cityID, slugs)`).

- [ ] Tests (fake drafter): pending moment candidate gets title/body/dates/topic_ids/sources merged and `origin: draft`; unknown slug dropped with note (pins Review Focus 2); non-pending or non-moment candidate fails without retry (`attempts == max_attempts` semantics: the job sets its own final failure via a `jobs.ErrNoRetry` sentinel honoured by `finish`).
- [ ] Run → fail. Implement (`ErrNoRetry` handling in `worker.finish`). Run → PASS.
- [ ] Commit: `feat(backend): draft moments from candidates with an LLM`.

### Task 6: Admin endpoints

**Files:** `backend/internal/http/admin_sources.go` + tests, `router.go`, `api/openapi.yaml`, `admin_review.go` (media approval enqueues `fetch_media`), `admin_review.go` (candidate list filters), `admin_tours.go` (suggest).

- [ ] Tests: `GET /admin/sources` lists five adapters; `POST /admin/imports` validates source and city, resolves a place, enqueues `import_source`; `POST /admin/candidates/{id}/draft` → 202, 409 for approved, 503 with `llm.None` (pins Review Focus 3); `POST /admin/tours/{id}/stops/{position}/suggest` → 503 without LLM, 200 ordered list with a fake drafter, 404 for a bad position; `GET /admin/stats` counts; media candidate approval creates a draft asset and a `fetch_media` job.
- [ ] Run → fail. Implement. Run → PASS; OpenAPI contract test green.
- [ ] Commit: `feat(backend): sources, imports, drafting, suggestion, and stats endpoints`.

### Task 7: Wiring, config, compose, docs for the backend half

**Files:** `backend/cmd/api/main.go`, `backend/internal/providers/providers.go`, `backend/.env.example`, `backend/docker-compose.yml`, `backend/docker-compose.nuc.yml`, `README.md`, `docs/deployment/nuc-almaz.md`.

- [ ] Wire registry and drafter into `Deps` and the worker; env vars from spec §7; docs describe how to pick a provider and point `LLM_BASE_URL` at Ollama or OpenRouter.
- [ ] `go vet ./... && go test ./... && go build ./...` green; `docker compose config` green.
- [ ] Commit: `feat(backend): wire sources and drafting; config and docs`.

### Task 8: Web app scaffold, API client, auth

**Files:** `web/` via `npx sv create` (SvelteKit, TypeScript, Vitest, Playwright, Tailwind), `web/package.json` scripts (`gen:api` runs `openapi-typescript ../backend/api/openapi.yaml -o src/lib/api/types.ts`), `web/src/lib/api/client.ts` + `client.test.ts`, `web/src/lib/auth.ts`, `web/src/routes/+layout.svelte`, `web/src/routes/signin/+page.svelte`, `web/vite.config.ts` (proxy `/api`, `/media` to `localhost:8080`), `web/svelte.config.js` (adapter-static, fallback `index.html`).

- [ ] Tests (Vitest): client adds bearer, maps `{error:{code,fields}}` to `ApiError` with fields, unwraps `items`; `auth.ts` persists and clears the token.
- [ ] Implement; `npm run check && npm run test:unit && npm run build` green.
- [ ] Commit: `feat(web): SvelteKit admin scaffold with typed API client and sign-in`.

### Task 9: Review queue, imports, dashboard

**Files:** `web/src/routes/+page.svelte` (dashboard), `queue/+page.svelte`, `queue/[id]/+page.svelte`, `imports/+page.svelte`, `web/src/lib/components/{DataTable,Field,StatusPill,Dialog,PayloadForm}.svelte`, `web/src/lib/payload.ts` + test, `web/src/routes/+layout.ts` (role gate).

- [ ] Tests: `payload.ts` maps a moment candidate payload to form fields and back without losing unknown keys; role gate renders the "needs curator role" screen for a visitor token (pins Review Focus 5, Vitest component test).
- [ ] Implement pages; Playwright `e2e/queue.spec.ts`: sign in as the bootstrap admin, create a candidate through the API, open it, approve with an edited title, see it under places/moments. Skips when `E2E_API_URL` is unset.
- [ ] Commit: `feat(web): dashboard, review queue, and imports`.

### Task 10: Content editors and tours

**Files:** `web/src/routes/{places,places/[id],moments/[id],media,tours,tours/[id],jobs}/+page.svelte`, `web/src/lib/components/{Map,StopsEditor,MediaPicker}.svelte`.

- [ ] Implement: MapLibre map with OpenFreeMap style; stops editor with reorder, add place search, featured moment select with "Suggest"; publish buttons surface the 409 blockers verbatim; media upload with licence field; jobs retry.
- [ ] Playwright `e2e/tour.spec.ts`: create a tour, set two stops, run routes (wait for worker), publish; expect it in the public tours list.
- [ ] Commit: `feat(web): places, moments, media, tours, and jobs editors`.

### Task 11: Web deployment and docs

**Files:** `web/Dockerfile` (node build stage → `nginx:alpine`), `web/nginx.conf`, compose `web` services in both compose files, `.gitea/workflows/ci.yml` (node job: `npm ci`, `check`, `test:unit`, `build`), `README.md`, `docs/deployment/nuc-almaz.md`, `Makefile` (`web-dev`, `web-build`).

- [ ] Verify: `docker compose config`, CI file parses, `npm run build` output served by nginx config in a local container responds on `/` and proxies `/api/v1/cities`.
- [ ] Commit: `feat(web): nginx image, compose service, CI, and docs`.
- [ ] Push.
