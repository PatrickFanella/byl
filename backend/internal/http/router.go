// Package httpapi serves the Invisible Museum HTTP API.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"invisible-museum/backend/internal/auth"
	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

// SessionManager issues and revokes sessions in addition to resolving them.
type SessionManager interface {
	SessionResolver
	Issue(ctx context.Context, accountID string) (string, time.Time, error)
	Revoke(ctx context.Context, token string) error
}

// Deps holds everything the handlers need.
type Deps struct {
	Store          *repository.Store
	Sessions       SessionResolver
	SessionManager SessionManager
	Apple          auth.Verifier
	Google         auth.Verifier
	DevAuth        bool
	IsBootstrap    func(email string) bool
	Blob           blob.Store
	MediaFSDir     string
	Log            *slog.Logger
	Ready          func(ctx context.Context) error
}

func (d Deps) logger() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

// NewRouter wires every route.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.StripSlashes)

	r.Get("/healthz", d.healthz)
	if d.MediaFSDir != "" {
		r.Handle("/media/files/*", http.StripPrefix("/media/files/", http.FileServer(http.Dir(d.MediaFSDir))))
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(d.optionalAuth)
		r.Get("/cities", d.listCities)
		r.Get("/cities/{city}", d.getCity)
		r.Get("/cities/{city}/places", d.listPlaces)
		r.Get("/cities/{city}/tours", d.listTours)
		r.Get("/places/{place}", d.getPlace)
		r.Get("/places/{place}/moments", d.listPlaceMoments)
		r.Get("/moments/{id}", d.getMoment)
		r.Get("/topics/{id}", d.getTopic)
		r.Get("/tours/{tour}", d.getTour)
		r.Get("/tours/{tour}/route", d.getTourRoute)
		r.Get("/media/{id}", d.getMedia)
		r.Post("/auth/dev", d.devSignIn)
		r.Post("/auth/apple", d.appleSignIn)
		r.Post("/auth/google", d.googleSignIn)

		r.Group(func(r chi.Router) {
			r.Use(d.requireAuth)
			r.Get("/me", d.me)
			r.Delete("/auth/session", d.logout)
			r.Get("/me/tours", d.listMyTours)
			r.Post("/me/tours", d.createMyTour)
			r.Get("/me/tours/{id}", d.getMyTour)
			r.Patch("/me/tours/{id}", d.updateMyTour)
			r.Delete("/me/tours/{id}", d.deleteMyTour)
			r.Get("/me/saved", d.listSaved)
			r.Put("/me/saved/{tour}", d.saveTour)
			r.Delete("/me/saved/{tour}", d.unsaveTour)
			r.Get("/me/progress", d.listProgress)
			r.Get("/me/progress/{tour}", d.getProgress)
			r.Put("/me/progress/{tour}", d.putProgress)
		})
	})
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

// fail maps repository errors to HTTP responses.
func (d Deps) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, repository.ErrInvalidReference):
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
	default:
		d.logger().Error("request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "something went wrong")
	}
}

// canSeeTour applies visibility rules for a tour the store returned.
func canSeeTour(t *models.TourSummary, a *models.Account) bool {
	isOwner := a != nil && t.OwnerAccountID != nil && *t.OwnerAccountID == a.ID
	isCurator := a != nil && a.Role.AtLeast(models.AccountRoleCurator)
	if isOwner {
		return true
	}
	if t.Visibility == models.TourVisibilityPrivate {
		return false
	}
	if t.Status != models.EntityStatusPublished {
		return isCurator
	}
	return true
}
