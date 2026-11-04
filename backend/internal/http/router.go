// Package httpapi serves the Invisible Museum HTTP API.
package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Deps holds everything the handlers need. Later tasks add fields.
type Deps struct {
	Sessions SessionResolver
	Ready    func(ctx context.Context) error
}

// NewRouter wires every route.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.StripSlashes)

	r.Get("/healthz", d.healthz)
	return r
}

func (d Deps) healthz(w http.ResponseWriter, r *http.Request) {
	if d.Ready != nil {
		if err := d.Ready(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "database is not reachable")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
