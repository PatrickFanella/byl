# Web Visitor App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A public visitor site in the existing SvelteKit app: browse tours, walk or ride one with arrival detection and audio, build and share routes; the admin app moves under `/admin`.

**Architecture:** Two route groups in `web/src/routes`: `(visitor)` at `/` and `(admin)/admin`. Pure logic (arrival, progress, estimates, narration, directions, route builder) lives in `web/src/lib/tour/*.ts` and `web/src/lib/routes/builder.ts` with Vitest; pages are thin Svelte 5 components over those modules. Static SPA, same nginx image.

**Tech Stack:** SvelteKit 3, Svelte 5 runes, TypeScript, Tailwind 4, MapLibre + OpenFreeMap, Vitest, Playwright, generated OpenAPI types (`npm run gen:api`).

**Spec:** `docs/superpowers/specs/2026-11-05-web-visitor-app-design.md`

## Global Constraints

- Imports use `#lib/<path>.ts` (SvelteKit 3 has no `$lib`); install with `npx -y npm@11 install --legacy-peer-deps`.
- `ssr = false`, `prerender = false` for both groups; `adapter-static` with `fallback: 'index.html'`.
- No `{@html}`; API errors render `ApiError.message`; the session token never appears in a URL.
- Arrival rule: fix `accuracy <= 65` m and `distance <= radius + accuracy`; fires once per stop.
- Share link: `{location.origin}/t/{slug}`; app scheme `invisiblemuseum://tour/{slug}`.
- Directions: Google `https://www.google.com/maps/dir/?api=1&destination={lat},{lng}&travelmode={walking|bicycling}`; Apple `https://maps.apple.com/?daddr={lat},{lng}&dirflg={w|b}` when the user agent contains `iPhone`, `iPad` or `Macintosh`.
- Before commit: `npm run check`, `npm run lint`, `npx vitest run src/lib` clean; Playwright specs pass against a live API with `DEV_AUTH_ENABLED=true` and `ADMIN_BOOTSTRAP_EMAILS=admin@example.com` (`E2E_API_URL`, `PLAYWRIGHT_CHROMIUM_PATH=/opt/pw-browsers/chromium`).
- Commits use `-c user.name="Claude" -c user.email="noreply@anthropic.com"` and the session trailers.

## Review Focus

1. Geolocation denied or unavailable (desktop): the walk page must still work through "I'm here" and say why there is no distance. Task 5.
2. Reload mid-walk without network: the walk page must come back from the cached tour and route rather than an error page. Task 5.
3. Two tabs or devices on one tour: merging server progress must never drop a visited stop. Task 4.
4. A shared link opened by a signed-out stranger: `/t/{slug}` must land on the unlisted tour with "Open in the app", never on sign-in. Task 6.
5. The admin move: every admin nav link and in-page link must resolve under `/admin` (no SPA not-found page). Task 1.

---

### Task 1: Route groups — admin under `/admin`, visitor shell at `/`

**Files:**
- Move: `web/src/routes/{+page.svelte,+page.ts,queue,imports,jobs,places,moments,media,tours}` → `web/src/routes/(admin)/admin/`; `web/src/routes/+layout.svelte` → `(admin)/admin/+layout.svelte`; `web/src/routes/+layout.ts` stays at the root (session restore for both groups).
- Move: `web/src/routes/signin` → `web/src/routes/(visitor)/signin`.
- Create: `web/src/routes/(visitor)/+layout.svelte` (dark shell: top nav Home · Tours · Routes · Saved · Profile, Admin link when `canCurate`), `web/src/routes/(visitor)/+page.svelte` (placeholder heading "Your city as a museum" until Task 2).
- Modify: every admin `href="/x"` / `goto('/x')` → `/admin/x`; admin sign-out → `goto('/signin')`; the admin layout redirects to `/signin?next=/admin/...`; `web/src/routes/layout.css` adds the dark tokens (`--color-gallery: #0a0a0b`, accent `#3b82f6`).
- Modify: `web/e2e/queue.e2e.ts`, `web/e2e/tour.e2e.ts` (admin paths get `/admin`).
- Test: `web/e2e/admin-links.e2e.ts`.

**Interfaces:**
- Produces: the visitor layout slot; `canCurate` from `#lib/auth.ts` (existing).

- [ ] **Step 1: Write the failing e2e test** (Review Focus 5)

```ts
test('every admin nav link resolves under /admin', async ({ page }) => {
	await signIn(page, 'admin@example.com');
	await page.goto('/admin');
	const hrefs = await page.locator('header nav a').evaluateAll((as) => as.map((a) => a.getAttribute('href')));
	expect(hrefs.length).toBeGreaterThanOrEqual(7);
	for (const href of hrefs) {
		expect(href).toMatch(/^\/admin/);
		await page.goto(href!);
		await expect(page.getByRole('heading').first()).toBeVisible();
		await expect(page.getByText(/not found/i)).toHaveCount(0);
	}
});
```

- [ ] **Step 2: Run it** — Expected: FAIL (links are `/queue`, `/imports`, …).
- [ ] **Step 3: Move the files and rewrite the links** (`git mv`; `grep -rn "href=\"/\|goto(" src/routes/(admin)` must show only `/admin/...` or `/signin`).
- [ ] **Step 4: Run all e2e, check, lint** — Expected: 5 specs pass; 0 errors.
- [ ] **Step 5: Commit** `refactor(web): admin under /admin, visitor shell at /`.

### Task 2: Browse — home, tours, tour detail, place, moment

**Files:**
- Create: `web/src/lib/visitor.ts` (typed calls: `listTours(city, topic?, mode?)`, `getTour(ref)`, `getRoute(tour, mode)`, `listPlacesNear(city, lat, lng, radiusM)`, `getPlace(ref)`, `getMoment(id)`, `me()`, `mySaved()`, `saveTour(id)`, `unsaveTour(id)`, `myProgress()`, `putProgress(tour, body)`, `myTours()`, `createTour(body)`, `updateTour(id, body)`, `deleteTour(id)`; DTO aliases from `Schemas`), `web/src/lib/tour/estimate.ts`, `web/src/lib/components/visitor/{LoadState,TourCard,StopRow,MomentCard,MediaGallery,CityPicker,SignInPrompt}.svelte`, `web/src/lib/city.svelte.ts` (`selectedCity` rune store remembered in `localStorage` key `invisible-museum.city`).
- Create pages: `(visitor)/+page.svelte`, `(visitor)/tours/+page.svelte`, `(visitor)/tours/[ref]/+page.svelte`, `(visitor)/places/[ref]/+page.svelte`, `(visitor)/moments/[id]/+page.svelte`.
- Test: `web/src/lib/tour/estimate.test.ts`, `web/e2e/visitor.e2e.ts`.

**Interfaces:**
- Produces: `estimateText(route: RouteSummary): string | null` ("24 min walk", "7 min ride", null unless `status === 'ready'`; minutes = `estimated_minutes ?? round(duration_s/60)`, min 1), `distanceText(m: number): string` ("850 m", "1.8 km"), `modeVerb(mode)`.
- Produces: `selectedCity` with `city.select(c)`, `city.current`.

- [ ] **Step 1: Write failing unit tests** in `estimate.test.ts`: ready walk 1404 s / 24 min → "24 min walk"; bike 405 s estimated 7 → "7 min ride"; ready without `estimated_minutes` 650 s → "11 min walk"; pending → null; `distanceText(1824.68)` → "1.8 km"; `distanceText(850)` → "850 m".
- [ ] **Step 2: Run** `npx vitest run src/lib/tour` — Expected: FAIL (module missing).
- [ ] **Step 3: Implement `estimate.ts`**; **Step 4: Run** — PASS.
- [ ] **Step 5: Write the failing e2e** in `visitor.e2e.ts`: visit `/`, expect heading "Downers Grove as a museum", click the "Downtown Memory Loop" card, expect `/tours/downtown-memory-loop`, heading, 4 stop rows, text matching `/min walk/` once routes are ready (poll with `expect.toPass` and reload, as `tour.e2e.ts` does), click stop 2 → `/places/...?featured=` shows "Featured on this tour" then the moment link → `/moments/...` shows "Sources".
- [ ] **Step 6: Implement pages and components** with loading/empty/error states (`LoadState` takes `state: 'loading' | 'error' | 'empty' | 'ready'`, `message`, `onretry`). Tour detail: `Map` with stop markers and the walk route line when ready; buttons "Start walking"/"Start riding" link to `/tours/[ref]/walk?mode=`; Save calls `saveTour`/`unsaveTour` when signed in, else shows `SignInPrompt`; "Open in the app" links `invisiblemuseum://tour/{slug}`.
- [ ] **Step 7: Run e2e, check, lint** — PASS. **Step 8: Commit** `feat(web): visitor browse pages`.

### Task 3: Arrival, directions, narration modules

**Files:**
- Create: `web/src/lib/tour/arrival.ts`, `web/src/lib/tour/directions.ts`, `web/src/lib/tour/narration.ts`.
- Test: `web/src/lib/tour/arrival.test.ts`, `directions.test.ts`, `narration.test.ts`.

**Interfaces:**
- Produces: `hasArrived(fix: {lat, lng, accuracy}, stop: {lat, lng}, radiusM): boolean`; `class ArrivalMonitor { constructor(stop, radiusM); evaluate(fix): boolean; update(fix): boolean /* true the first time */; retarget(stop, radiusM); hasArrived; distance(fix): number }`; `distanceM(a, b)` haversine.
- Produces: `mapsURL(lat, lng, mode: 'walk'|'bike', userAgent: string): string`.
- Produces: `plan(narrationURL: string | null, body: string): {kind: 'stream', url} | {kind: 'speech', text} | {kind: 'none'}`; `plainText(markdown): string`.

- [ ] **Step 1: Failing tests.** Arrival: 20 m away, accuracy 10, radius 40 → true; accuracy 200 → false; 120 m away → false; 50 m away accuracy 15 → true, 60 m → false; accuracy -1 → false; 65 ok, 66 not; `update` fires once until `retarget`. Directions: walk + Chrome UA → Google walking URL; bike + iPhone UA → Apple `dirflg=b`. Narration: URL → stream; Markdown body → speech with `plainText`; blank → none; `plainText("## The Depot\n\nIt **opened** in [1864](https://x).")` → "The Depot It opened in 1864.".
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS. **Step 5: Commit** `feat(web): arrival, directions, and narration logic`.

### Task 4: Local progress with server merge

**Files:**
- Create: `web/src/lib/tour/progress.ts`.
- Test: `web/src/lib/tour/progress.test.ts`.

**Interfaces:**
- Produces: `class ProgressStore { constructor(tourID, mode, stopCount, opts: { storage: Storage; api: { get(): Promise<TourProgress[]>; put(body: {mode, visited_positions, completed}): Promise<unknown> }; signedIn: () => boolean }); visited: Set<number>; markVisited(position): void; mergeRemote(): Promise<void>; needsSync: boolean; syncIfNeeded(): Promise<void>; flush(): Promise<void>; isComplete: boolean; reset(): void }`; storage key `invisible-museum.progress.{tourID}.{mode}` holding `{visited: number[], updatedAt, syncedAt, completedAt}`.

- [ ] **Step 1: Failing tests** with an in-memory `Storage` and a fake api recording calls: markVisited persists and PUTs `{mode:'walk', visited_positions:[1], completed:false}` when signed in; no PUT when signed out; `mergeRemote` with remote `[1,2]` and local `[3]` → visited `[1,2,3]` and a PUT of `[1,2,3]` (Review Focus 3); remote equal to local → no PUT, `needsSync` false; failed PUT → `needsSync` stays true and `syncIfNeeded` re-sends once the fake succeeds; two quick `markVisited` → PUT bodies `[[1],[1,2]]` in order; `stopCount` reached → `completed: true`; a second store on the same storage restores `visited`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS. **Step 5: Commit** `feat(web): tour progress store with server merge`.

### Task 5: Walk page

**Files:**
- Modify: `web/src/lib/components/Map.svelte` (props: `stops: {lat,lng,label,position,visited,next}[]`, `legs: {coords: [number,number][], state: 'past'|'active'|'future'}[]`, `user?: {lat,lng}`, `fit: 'all'|'ahead'`).
- Create: `web/src/lib/tour/walk.svelte.ts` (`class ActiveWalk` built from Tasks 3–4: `load()` fetches tour + route with `localStorage` cache keys `invisible-museum.tour.{ref}` / `invisible-museum.route.{id}.{mode}` and falls back to them on failure (`fromCache`), `handleFix(fix)`, `markArrived()`, `nextStop`, `arrivedStop`, `pastLegs/activeLeg/futureLegs`, `distanceToNextM`, `progressText`, `audioEnabled`, `playNarration(stop)` via `<audio>` or `speechSynthesis`), `web/src/lib/tour/walk.test.ts`, `web/src/routes/(visitor)/tours/[ref]/walk/+page.svelte`, `web/src/lib/components/visitor/ArrivalPanel.svelte`.
- Test: `web/e2e/walk.e2e.ts`.

**Interfaces:**
- Consumes: Task 2 `visitor.ts`, Task 3, Task 4.

- [ ] **Step 1: Failing unit tests** in `walk.test.ts` with a fake fetch: `load()` caches; a second `load()` whose fetch rejects resolves from cache with `fromCache === true` (Review Focus 2); `handleFix` at stop 1 → `nextStop.position === 2`, `activeLeg.to_position === 2`, `futureLegs` `[3,4]`; `markArrived()` works without a fix.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement `walk.svelte.ts`.** **Step 4: Run** — PASS.
- [ ] **Step 5: Failing e2e** `walk.e2e.ts`: `context.grantPermissions(['geolocation'])`, `setGeolocation` at the station (41.7960, -88.0093, accuracy 10); open `/tours/downtown-memory-loop/walk?mode=walk`; expect "Next · stop 1"; the arrival panel "You have arrived" appears with "The Burlington Arrives"; progress "1 of 4 stops"; Continue; "Next · stop 2". Second test with geolocation denied (`context.setGeolocation` unset and permission not granted): page shows "Location is off" and "I'm here" advances to stop 2 (Review Focus 1).
- [ ] **Step 6: Implement the page and panel** (audio toggle persisted in `localStorage` `invisible-museum.audio`; Directions button uses `mapsURL`; End asks to keep or reset progress).
- [ ] **Step 7: Run e2e, check, lint** — PASS. **Step 8: Commit** `feat(web): walk or ride a tour in the browser`.

### Task 6: Routes, share links, saved, profile

**Files:**
- Create: `web/src/lib/routes/builder.ts`, `web/src/lib/routes/builder.test.ts`, pages `(visitor)/routes/+page.svelte`, `(visitor)/routes/new/+page.svelte`, `(visitor)/routes/[id]/edit/+page.svelte`, `(visitor)/saved/+page.svelte`, `(visitor)/profile/+page.svelte`, `(visitor)/t/[slug]/+page.ts` (redirect 307 to `/tours/{slug}`), `web/src/lib/components/visitor/RouteBuilderForm.svelte`, `ShareLink.svelte` (copy + `navigator.share` when present).
- Test: `web/e2e/routes.e2e.ts`.

**Interfaces:**
- Produces: `class RouteBuilder { constructor(city: City, editing?: TourDetail); title; summary; walk; bike; visibility; stops: PlaceSummary[]; add(p); remove(id); move(from, to); contains(p); foreignStops; stopsHint: string | null; stopsError: string | null; canSubmit; fieldErrors; error; submit(api): Promise<TourDetail | null> }`; `shareURL(origin, slug)`.

- [ ] **Step 1: Failing unit tests**: disabled under 2 stops with hint "Add at least 2 stops (1 so far)"; duplicates ignored; cap 30 with hint "30 stops is the maximum"; cross-city stop → `canSubmit` false, hint "Every stop must be in Downers Grove"; `submit` with a fake api posting `{city, title, modes:['walk'], visibility:'private', stops:[{place_id}]}`; a 422 `ApiError` with `fields.stops` → `stopsError` and `error` set, returns null; editing prefills and PATCHes.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [ ] **Step 5: Failing e2e** `routes.e2e.ts`: signed-in visitor (dev sign-in, any email) opens `/routes/new`, names "Two stops", adds two places from the search list, Create → share link shown containing `/t/`; open that link in a fresh context (signed out) → lands on `/tours/{slug}` showing "Two stops" and "Open in the app" (Review Focus 4). Saved: Save on the seed tour then `/saved` lists it.
- [ ] **Step 6: Implement pages.** **Step 7: Run e2e, check, lint** — PASS. **Step 8: Commit** `feat(web): build and share routes, saved, profile`.

### Task 7: Docs and handoff

**Files:**
- Modify: `README.md` (web section: visitor at `/`, admin at `/admin`), `web/README.md`, `docs/deployment/nuc-almaz.md` (admin URL), `web/nginx.conf` only if a `/t/` rule is needed (it is not: the SPA fallback serves it).

- [ ] **Step 1: Update docs.** **Step 2: Run** `npm run check && npm run lint && npx vitest run src/lib` and the full e2e — PASS. **Step 3: Commit** `docs(web): visitor app and admin prefix`.
