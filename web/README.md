# Invisible Museum admin web

SvelteKit single-page app for curators. See the repository README for the API and `../docs/superpowers/specs/2026-11-04-sourcing-pipeline-and-admin-web-design.md` for the design.

```bash
npm install          # Node 22; use npx npm@11 if npm 10 fails on this lockfile
npm run dev          # proxies /api to http://localhost:8080 (VITE_API_PROXY overrides)
npm run check
npm run test:unit -- --run
npm run build        # static output in build/
E2E_API_URL=http://localhost:8080 npm run test:e2e
```
