# Web visitor app — design

Cycle 4 of the Invisible Museum rebuild. Earlier cycles: the backend content
model and API (`2026-11-04-tours-content-model-and-api-design.md`), the
sourcing pipeline and admin web (`2026-11-04-sourcing-pipeline-and-admin-web-design.md`),
and the iOS app (`2026-11-04-ios-tours-app-design.md`). Cycle 5 is Android.

## 1. Goal

Anyone with a browser can do what the iOS app does: browse a city's tours by
topic, open a tour, walk or ride it with a route line and arrival detection,
read and hear each stop's story, save tours and keep progress when signed in,
and build and share their own routes. Shared route links open in the browser
first, with the native apps one tap away.

The decision brief that shaped this cycle was taken with the human partner's
standing approval to proceed without stopping; every choice below that would
normally be a question is recorded as a decision with its reasoning.

## 2. Decisions

- **One SvelteKit app, two route groups.** The visitor site lives in `web/`
  beside the admin app: `src/routes/(visitor)/` serves `/` and the public
  pages; the admin app moves under `src/routes/(admin)/admin/`. They share
  the API client, session store, generated OpenAPI types, design tokens, and
  the nginx image. A second SvelteKit project would duplicate all of that and
  need a second image. Cost: every admin link and the admin e2e tests gain an
  `/admin` prefix.
- **Static SPA, no SSR.** Both groups stay `ssr = false` behind
  `adapter-static` with the `index.html` fallback, as the admin app is today.
  Share links are meant to be opened, not crawled; SSR for SEO is a later
  decision when a public hostname exists.
- **Visitor pages are public; account pages gate themselves.** The admin
  layout's redirect-to-sign-in and curator-role gate move into the `(admin)`
  group. The visitor layout never redirects; Saved, My routes and the route
  builder show an inline sign-in prompt when signed out.
- **Sign-in is the existing page**, moved to `/signin` inside the visitor
  group (Google when `VITE_GOOGLE_CLIENT_ID` is set, dev email when the API
  offers it). Sign in with Apple on the web needs a Services ID and a return
  URL and is out of scope.
- **Walking in the browser uses the Geolocation API** with `watchPosition`,
  the same arrival rule as iOS (fix accuracy ≤ 65 m, distance ≤ radius +
  accuracy, once per stop), an "I'm here" button, and a directions handoff
  to Google Maps (`https://www.google.com/maps/dir/?api=1&destination=lat,lng&travelmode=walking|bicycling`)
  or Apple Maps on Apple devices (`https://maps.apple.com/?daddr=lat,lng&dirflg=w|b`).
- **Audio guide in the browser:** the moment's `narration.public_url` plays
  through an `<audio>` element; otherwise the body is read with the Web
  Speech API (`speechSynthesis`), Markdown stripped the same way as iOS. No
  background playback promises: the tab must stay open.
- **Progress** is kept in `localStorage` per tour and mode and merged with
  `GET /me/progress` when signed in, with `PUT /me/progress/{tour}` sent in
  order and retried when the server is behind, mirroring `ActiveTourModel`.
- **Share links** are `{web origin}/t/{slug}` from the web app (copy button
  plus the Web Share API when available). `/t/[slug]` in the visitor group
  redirects to `/tours/[slug]`, which shows "Open in the app" linking to
  `invisiblemuseum://tour/{slug}`. The API's own `/t/{slug}` landing page
  stays for links the iOS app shares.
- **Map** reuses `MapLibre` with the OpenFreeMap style already used by the
  admin tour editor, extended with user position, numbered stop markers, and
  past/active/future leg styling.
- **Design**: dark gallery look matching iOS (`stone-950` background, white
  text, blue accent) through Tailwind tokens in `layout.css`; the admin keeps
  its light theme. No component library.

## 3. Pages

| Route | Shows | Data |
| --- | --- | --- |
| `/` | city picker (remembered in `localStorage`), tours by topic, places near the visitor when geolocation is allowed | `GET /cities`, `/cities/{c}/tours`, `/cities/{c}/places?near=` |
| `/tours` | all tours with topic and mode filters | `/cities/{c}/tours?topic=&mode=` |
| `/tours/[ref]` | hero, summary, per-mode estimate ("24 min walk · 1.8 km"), Save, Start walking / riding, stops with featured moments, map with route line, "Open in the app" | `/tours/{ref}`, `/tours/{ref}/route?mode=` |
| `/tours/[ref]/walk?mode=` | walking mode: full-height map, next stop card with distance, I'm here, Directions, audio toggle, progress "2 of 4", arrival panel with the featured moment and the place's other moments, End | route, place detail on arrival, progress |
| `/places/[ref]?featured=` | place with the featured moment first, the rest listed | `/places/{ref}` |
| `/moments/[id]` | title, date, media gallery with credits, Markdown body, topics, sources, Listen button | `/moments/{id}` |
| `/routes` | my routes: share, edit, delete; New route | `/me/tours` |
| `/routes/new`, `/routes/[id]/edit` | builder: name, summary, modes, visibility, ordered stops with up/down, place search sorted by distance, Create/Save, share after success | `/cities/{c}/places`, `POST`/`PATCH /me/tours` |
| `/saved` | saved tours with "n of m stops" | `/me/saved`, `/me/progress` |
| `/profile` | account, sign out, audio guide preference, location permission state | `/me` |
| `/signin` | Google and dev sign-in (existing page) | `/auth/*` |
| `/t/[slug]` | redirect to `/tours/[slug]` | — |
| `/admin/**` | the existing admin app, unchanged apart from the prefix | — |

Every page has loading, empty, and error states; errors show the API message.

## 4. Architecture

```
web/src/
  lib/
    api/            client.ts, index.ts, types.ts (generated)   [existing]
    auth.ts         session store                                 [existing]
    admin.ts        admin + public typed calls                    [existing; public calls move to visitor.ts]
    visitor.ts      typed public and /me calls, DTO aliases
    tour/
      arrival.ts    hasArrived(fix, stop, radiusM), ArrivalMonitor (pure, tested)
      progress.ts   LocalProgress store: load/save per tour+mode, merge(remote), needsSync, serialised sync
      estimate.ts   estimateText(route), distanceText(m), modeVerb
      narration.ts  plan(narrationURL, body) → stream | speech | none; plainText(markdown)
      directions.ts mapsURL(lat, lng, mode, userAgent)
    routes/
      builder.ts    RouteBuilder state: stops add/remove/move, canSubmit, stopsHint, foreignStops, submit → fieldErrors
    components/
      visitor/      Hero, TourCard, StopRow, MomentCard, MediaGallery, LoadState (loading/empty/error), SignInPrompt, CityPicker
      Map.svelte    extended: user marker, numbered stops, leg styles         [existing]
  routes/
    (visitor)/      +layout.svelte (dark shell, top nav), pages above
    (admin)/admin/  everything that is under routes/ today, links prefixed
```

State lives in small Svelte 5 rune modules (`$state` in `.svelte.ts` files)
or plain classes with tests: the walk page owns an `ActiveWalk` object
(route, next stop, visited, arrival monitor, progress store) built from the
tested modules, so the page body stays declarative.

## 5. Error handling and offline

- API errors render the envelope's message in the page's error state; 401
  clears the session (existing client behaviour) and the visitor layout
  shows the sign-in prompt where an account is needed.
- Geolocation denied: the walk page explains and relies on "I'm here".
- Route pending or failed: the walk page shows stops without a line and a
  notice; Directions still works.
- The walk page caches the tour and route in `localStorage` when they load
  and reads them back when the network fails, so a walk in progress survives
  a reload without signal.

## 6. Testing

- Vitest: `arrival.ts` (accuracy ceiling, radius + accuracy margin, once per
  stop), `progress.ts` (merge, needsSync, serialised PUT order with a fake
  fetch), `estimate.ts`, `narration.ts` (plan and plainText), `directions.ts`,
  `builder.ts` (2–30 stops, duplicates, cross-city, 422 field mapping).
- Playwright against the live API (`E2E_API_URL`): visitor browses to the
  seed tour and starts walking; with `context.setGeolocation` at stop 1 the
  arrival panel appears and progress reads "1 of 4"; a signed-in visitor
  builds a two-stop route and the share link opens it; admin smoke under
  `/admin` (existing specs with the prefix).
- `npm run check` and `npm run lint` stay clean.

## 7. Out of scope

- SSR, prerendering, Open Graph cards.
- Sign in with Apple on the web.
- Background audio, offline map tiles, service worker.
- A public gallery of user routes, likes, comments.
- Android (cycle 5).
