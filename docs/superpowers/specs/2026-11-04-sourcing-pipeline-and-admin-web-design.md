# Invisible Museum: Sourcing Pipeline and Admin Web

Date: 2026-11-04
Status: approved in conversation (the user pre-approved continuing without stops); implementation in progress
Cycle: 2 of 5

## 1. Goal

Fill the review queue automatically and give curators a place to work it.

- **Sourcing:** pull candidate places, moments, and media near a city or a
  place from open archives and from configured local archive pages, through
  one adapter interface.
- **Drafting:** turn a candidate's raw source text into a moment draft with
  an LLM, through either the Claude API or any OpenAI-compatible endpoint,
  with the citation attached and nothing published without review.
- **Topic fit:** rank a place's moments against a tour's topic so a curator
  can pick the featured moment for a stop in one click.
- **Admin web:** the first slice of the web app: sign in, work the queue,
  trigger imports, edit places, moments, media, and tours, and publish.

Decisions carried from the brainstorm: open-archive adapters plus local
archives behind the same interface; humans approve before publish; the web
client is native to the web (a TypeScript app, no cross-platform toolkit).

## 2. Scope

In:

- Go 1.25 across go.mod, Dockerfile, and CI (the official Anthropic Go SDK
  requires it; everything already pinned keeps working).
- `internal/llm`: `Drafter` interface, Anthropic and OpenAI-compatible
  implementations, JSON-schema constrained output on both.
- `internal/sources`: adapter interface and five adapters: `wikipedia`,
  `wikimedia_commons`, `internet_archive`, `loc`, and `url` (a configured
  list of local archive pages, used for the Downers Grove Historical Society
  and the park district site).
- Jobs: `import_source` and `draft_candidate`.
- Endpoints: list sources, trigger imports, request a draft, suggest a
  featured moment, plus `origin` and `source` filters on the candidate list.
- `web/`: SvelteKit admin app with sign-in, dashboard, review queue, imports,
  places, moments, media, tours, and jobs pages, built against the OpenAPI
  document.
- Compose `web` service and dev proxy; docs.

Out:

- Visitor web features (browse tours, build routes): cycle 4.
- Flickr Commons (needs an API key), Wikidata SPARQL (Wikipedia page props
  give enough for now).
- Automatic publishing of anything.
- Image resizing and transcoding.

## 3. Sourcing

### Adapter interface (`internal/sources`)

```go
type Request struct {
    CityID   string
    Center   geo.Point   // search centre
    RadiusM  float64     // default 2000, max 20000
    Query    string      // free text, used by text-search adapters
    Limit    int         // default 25, max 100
}

type Found struct {
    Kind        models.CandidateKind // place, moment, media
    ExternalID  string               // stable per adapter
    Title       string
    Text        string               // raw text for drafting (moments) or caption (media)
    URL         string               // human-visible page
    Lat, Lng    *float64
    License     string               // identifier when known, else ""
    LicenseURL  string
    Credit      string
    MediaURL    string               // direct file URL for media
    MimeType    string
    Raw         json.RawMessage      // adapter response for audit
}

type Adapter interface {
    Name() string
    Discover(ctx context.Context, req Request) ([]Found, error)
}
```

`Registry` maps names to adapters and is built from config. All HTTP goes
through one client with a descriptive User-Agent (`InvisibleMuseum/<version>
(+repo URL)`), a 20 s timeout, and one retry on 429 or 5xx after the
`Retry-After` delay or 2 s.

### Adapters

- **wikipedia**: `list=geosearch` within the radius, then `prop=extracts|
  coordinates|pageprops|pageimages` for the hits. Each page becomes a `place`
  find (title, coordinates, first paragraph as text) **and** a `moment` find
  with the full plain-text extract as `Text`, `URL` the article, `License`
  `CC-BY-SA-4.0`, `Credit` "Wikipedia contributors". External ID `wikipedia:
  <pageid>:place` and `wikipedia:<pageid>:moment`.
- **wikimedia_commons**: `list=geosearch` with `gsnamespace=6`, then
  `prop=imageinfo&iiprop=url|extmetadata|mime|size`. Each file becomes a
  `media` find with `MediaURL`, `MimeType`, width and height, caption from
  `ImageDescription` (HTML stripped), `Credit` from `Artist`, and `License`
  mapped from `LicenseShortName` (`CC BY-SA 4.0` to `CC-BY-SA-4.0`, `CC0` to
  `CC0-1.0`, `Public domain` to `PD`, otherwise the short name verbatim).
  Files whose licence cannot be mapped keep `License` empty so publishing
  stays blocked until a curator fills it. External ID `commons:<pageid>`.
- **internet_archive**: `advancedsearch.php` with `q=<query> AND mediatype:
  (texts OR image OR movies)`, `fl=identifier,title,description,date,
  licenseurl,creator,mediatype`. Texts become `moment` finds whose `Text` is
  the description (plus title and date) and `URL` the item page; images and
  movies become `media` finds with `MediaURL` built from the item's download
  URL and `License` from `licenseurl` when present. External ID
  `ia:<identifier>`.
- **loc**: `https://www.loc.gov/search/?q=<query>&fo=json`, items with
  `original_format` of photo, print, or map become `media` finds (image URL
  from `image_url`, `License` "PD-US-LOC-no-known-restrictions" only when the
  item's rights text says no known restrictions, else empty), others become
  `moment` finds with the description. External ID `loc:<id>`. The endpoint
  is slow or blocked from some networks; the adapter honours the context
  deadline and returns what it has.
- **url**: fetches each configured page (`LOCAL_ARCHIVE_URLS`, comma
  separated), strips scripts, styles, and navigation, converts the main body
  to plain text, and emits one `moment` find per page with `Text` as the
  page text (capped at 20,000 characters), `URL` the page, `Credit` the site
  host, `License` empty. External ID `url:<sha256 of URL>`. `Query`, when
  set, filters pages whose text contains it.

### Import job

`import_source {source, city_id, place_id?, lat?, lng?, radius_m?, query?}`:

1. Resolve the search centre: the place's coordinates when `place_id` is set,
   else `lat`/`lng`, else the city centre.
2. Run the adapter.
3. For each find, skip when a candidate with the same `(source_name,
   external_id)` exists. Otherwise insert a candidate with `origin: import`,
   `source_name`, `external_id`, `kind`, `target_place_id` and `payload`:
   - `place`: `CreatePlaceRequest` fields plus `source_url`, `raw`.
   - `moment`: `{ place_id?, title, raw_text, source_url, source_title,
     source_publisher, source_license, raw }`. `place_id` is the request's
     `place_id`, else the nearest published place within 150 m of the find's
     coordinates, else null (the curator picks one before approval, or
     approves the matching place candidate first).
   - `media`: `MediaAsset`-shaped fields plus `media_url` and `target_moment_id`
     null. On approval the media job downloads the file into blob storage;
     approval of a media candidate therefore enqueues `fetch_media
     {candidate_id, asset_id}` instead of creating the asset synchronously.
4. Record counts in the job's `last_error` field as an info line (the jobs
   table has no result column; the text is "imported N, skipped M").

### Draft job

`draft_candidate {candidate_id}` on a pending `moment` candidate: builds a
`DraftRequest` from the payload (raw text, source title, URL, publisher,
licence, the target place's name and summary, and the city's topics), calls
the `Drafter`, and merges the result into the payload: `title`, `body`,
`date_start`, `date_end`, `date_precision`, `date_display`, `topic_ids`
(resolved from the returned topic slugs), `sources` (exactly one: the
candidate's source), and `draft_notes` (the model's uncertainty note). Sets
`origin: draft`. A candidate that is not pending or not a moment fails the
job without retry.

## 4. LLM drafting (`internal/llm`)

```go
type DraftRequest struct {
    PlaceName, PlaceSummary string
    SourceTitle, SourceURL, SourcePublisher, SourceLicense string
    RawText string            // capped at 12,000 characters by the caller
    Topics []TopicOption      // slug + name + description
}
type MomentDraft struct {
    Title, Body string
    DateStart, DateEnd string // YYYY-MM-DD or ""
    DatePrecision string      // day, month, year, circa
    DateDisplay string
    TopicSlugs []string
    Notes string              // what the source does not support
}
type RankRequest struct {
    TopicName, TopicDescription, TourTitle, TourSummary string
    PlaceName string
    Moments []MomentOption   // id, title, excerpt, date_display
}
type Ranking struct {
    Ordered []RankedMoment   // id, reason
}
type Drafter interface {
    DraftMoment(ctx context.Context, req DraftRequest) (MomentDraft, error)
    RankMoments(ctx context.Context, req RankRequest) (Ranking, error)
}
```

Both implementations send the same system prompt and user content and
constrain output to the same JSON schema:

- **anthropic**: official Go SDK (`github.com/anthropics/anthropic-sdk-go/v2`),
  model from `LLM_MODEL` (default `claude-opus-5-5`), thinking left at the
  model default, `output_config.format` set to the JSON schema, `max_tokens`
  4096, streaming not needed at this size. `LLM_BASE_URL` overrides the
  endpoint when set.
- **openai**: `POST {LLM_BASE_URL}/v1/chat/completions` with
  `response_format: {type: json_schema, json_schema: {strict: true, ...}}`,
  bearer `LLM_API_KEY`, model from `LLM_MODEL`. Works with OpenAI, Ollama,
  vLLM, OpenRouter, and similar.
- **none** (when `LLM_PROVIDER` is empty): drafting and ranking endpoints
  return 503 `unavailable`, and `draft_candidate` jobs fail without retry.

The system prompt fixes the rules: write 120 to 220 words of plain,
second-person-free prose for a visitor standing at the place; use only facts
in the provided source; put anything the source does not support in `notes`
instead of the body; give dates with the precision the source supports;
choose topic slugs only from the offered list. Responses are validated
against the schema after decoding; a response that fails validation is one
retry, then an error.

## 5. Endpoints

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/admin/sources` | adapters with `name`, `description`, `supports_query`, `supports_near` |
| POST | `/admin/imports` | `{ source, city, place?, near?, radius_m?, query? }` → 202 `{ job }` |
| POST | `/admin/candidates/{id}/draft` | → 202 `{ job }`; 409 when not a pending moment candidate; 503 when no LLM |
| POST | `/admin/tours/{id}/stops/{position}/suggest` | → 200 `{ ordered: [{ moment, reason }] }`; 503 when no LLM; synchronous with a 60 s timeout |
| GET | `/admin/candidates` | gains `?origin=` and `?source=` |
| GET | `/admin/stats` | counts for the dashboard: pending candidates by kind, jobs by status, published places, moments, tours |

Approving a `media` candidate enqueues `fetch_media` and returns the
candidate with `approved_entity_id` set to the asset created in `draft`
status with an empty `storage_key`; the job fills `storage_key` and
`public_url` and leaves the asset for the curator to attach to a moment.

## 6. Admin web (`web/`)

- **Stack:** SvelteKit 2, Svelte 5, TypeScript, Tailwind CSS 4,
  `adapter-static`, `openapi-typescript` generating `src/lib/api/types.ts`
  from `backend/api/openapi.yaml` at build, a thin typed fetch wrapper that
  adds the bearer token and maps the error envelope. No component library;
  a small set of own components (table, form field, status pill, dialog).
- **Auth:** sign-in page with dev email login (when the API advertises it
  through a 200 on `/auth/dev`, otherwise hidden) and Google sign-in via the
  Google Identity Services script when `PUBLIC_GOOGLE_CLIENT_ID` is set.
  Token stored in `localStorage`, attached as `Authorization`. Routes under
  `/admin` redirect to sign-in without a token and show "needs curator role"
  for visitors.
- **Pages:** `/` dashboard (stats, recent failed jobs); `/queue` list with
  filters and `/queue/[id]` detail showing payload fields as a form, the raw
  source text, "Draft with AI", approve with edits, reject with note;
  `/imports` form (source, city, place or point, radius, query) and recent
  import jobs; `/places`, `/places/[id]` with moments, primary moment select,
  publish; `/moments/[id]` editor with sources, topics, media attach,
  publish; `/media` grid with upload and licence editing; `/tours`,
  `/tours/[id]` with stops editor (reorder, add place, featured moment select
  with "Suggest"), routes status, publish; `/jobs` list with retry.
- **Map:** MapLibre GL with the OpenFreeMap vector style on the place and
  tour pages, showing the place markers and the route line.
- **Testing:** Vitest for the fetch wrapper and payload-form mapping;
  Playwright against the real API on the test database for sign-in, approve
  a candidate, publish a tour (run in CI only when `TEST_DATABASE_URL` is
  set).
- **Deployment:** `npm run build` emits `web/build`; compose `web` service
  is `nginx:alpine` serving it with `/api/` and `/media/` proxied to the API.
  Dev: `npm run dev` proxies the same paths to `localhost:8080`.

## 7. Configuration additions

```
LLM_PROVIDER=anthropic|openai|            (empty disables drafting)
LLM_API_KEY=
LLM_BASE_URL=                             (optional override)
LLM_MODEL=claude-opus-5-5 | gpt-4o | ...
LOCAL_ARCHIVE_URLS=https://www.dghistory.org/..., https://www.dgparks.org/...
SOURCES_USER_AGENT=InvisibleMuseum/0.2 (+https://github.com/PatrickFanella/invisible-museum)
PUBLIC_GOOGLE_CLIENT_ID=                  (web)
PUBLIC_API_BASE_URL=                      (web, default same origin)
```

## 8. Testing

- Adapters against `httptest` servers replaying recorded JSON fixtures in
  `internal/sources/testdata/` captured from the live APIs on 2026-11-04.
- LLM clients against `httptest`: request shape (schema present, model,
  auth header), response decoding, schema validation retry.
- Jobs on the test database: import creates candidates and skips
  duplicates, nearest-place matching, draft merges fields and sets origin,
  media approval enqueues and fetches into the FS blob store from an
  `httptest` file server.
- Handlers: sources list, import trigger validation, draft 409/503, suggest
  503 and 200 with a fake drafter, stats.
- Web: Vitest units; Playwright smoke when the API is available.

## 9. Risks

- Open-archive rate limits: adapters are called from jobs, one at a time per
  worker, and never from request handlers.
- LLM cost: drafts run only when a curator asks (or when an import is
  triggered with `draft: true`, which is off by default).
- The `url` adapter's text extraction is heuristic; the draft prompt tells
  the model to ignore navigation residue, and the curator sees the raw text.
