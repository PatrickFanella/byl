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
<title>Invisible Museum tour</title>
<meta http-equiv="refresh" content="0; url={{.AppURL}}">
<style>body{font-family:-apple-system,system-ui,sans-serif;background:#09090b;color:#fff;margin:0;padding:48px 24px;text-align:center}a{color:#3b82f6}p{color:#a1a1aa}</style>
</head><body>
<h1>Invisible Museum</h1>
<p>Someone shared a route with you. Opening it in the app…</p>
<p><a href="{{.AppURL}}">Open in Invisible Museum</a></p>
<p>Don't have the app yet? Install Invisible Museum, then open this link again.</p>
</body></html>
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
