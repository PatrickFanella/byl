# Invisible Museum web

SvelteKit single-page app: the visitor site at `/` (`src/routes/(visitor)`) and the curator admin at `/admin` (`src/routes/(admin)`). Designs: `../docs/superpowers/specs/2026-11-05-web-visitor-app-design.md` and `../docs/superpowers/specs/2026-11-04-sourcing-pipeline-and-admin-web-design.md`.

Tested logic lives in `src/lib/tour` (arrival, progress, estimates, narration, directions, the walk store) and `src/lib/routes` (route builder); run it with `npm run test:unit -- --run`. Playwright specs in `e2e/` need a live API.

```bash
npm install          # Node 22; use npx npm@11 if npm 10 fails on this lockfile
npm run dev          # proxies /api to http://localhost:8080 (VITE_API_PROXY overrides)
npm run check
npm run test:unit -- --run
npm run build        # static output in build/
E2E_API_URL=http://localhost:8080 npm run test:e2e
```
