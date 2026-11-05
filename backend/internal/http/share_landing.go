package httpapi

import (
	"html/template"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
)

var slugPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var landingTemplate = template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Byl tour</title>
<meta http-equiv="refresh" content="0; url={{.AppURL}}">
<style>
:root{color-scheme:light dark;--ground:#f1edf0;--text:#37232e;--muted:#715462;--action:#81344c}
@media(prefers-color-scheme:dark){:root{--ground:#2d1f2b;--text:#f5edf1;--muted:#d1bdca;--action:#e2b3c3}}
body{font-family:system-ui,sans-serif;background:var(--ground);color:var(--text);margin:0;padding:48px 24px;line-height:1.625}
main{max-width:66ch;margin:auto}h1{font:400 40px Georgia,serif}a{color:var(--action);text-underline-offset:3px;display:inline-block;min-height:48px}
p{color:var(--muted)}a:focus-visible{outline:3px solid var(--action);outline-offset:3px}
</style>
</head><body><main>
<h1>Byl</h1>
<p>Someone shared a route with you. Opening it in the app…</p>
<p><a href="{{.AppURL}}">Open in Byl</a></p>
<p>This link opens an installed Byl app. Byl is in development.</p>
</main></body></html>
`))

// shareLanding answers GET /t/{slug}: the link visitors share for a route. It redirects into
// the app through its URL scheme and explains itself to anyone without the app.
func (d Deps) shareLanding(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if !slugPattern.MatchString(slug) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// The fixed app scheme and validated slug are safe; template otherwise rejects custom schemes.
	_ = landingTemplate.Execute(w, map[string]template.URL{"AppURL": template.URL("invisiblemuseum://tour/" + slug)})
}
