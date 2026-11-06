// Package httpapi serves the Invisible Museum HTTP API.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"invisible-museum/backend/internal/auth"
	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/seedmedia"
	"invisible-museum/backend/internal/sources"
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
	Sources        *sources.Registry
	Drafter        llm.Drafter
	Log            *slog.Logger
	Ready          func(ctx context.Context) error
}

// drafterAvailable reports whether an LLM provider is configured.
func (d Deps) drafterAvailable() bool {
	if d.Drafter == nil {
		return false
	}
	_, none := d.Drafter.(llm.None)
	return !none
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
	r.Get("/t/{slug}", d.shareLanding)
	r.Handle("/media/files/library/*", http.StripPrefix("/media/files/library/", noDirListing(http.FS(seedmedia.Files))))
	if d.MediaFSDir != "" {
		r.Handle("/media/files/*", http.StripPrefix("/media/files/", noDirListing(http.Dir(d.MediaFSDir))))
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
		r.Get("/auth/dev", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]bool{"enabled": d.DevAuth})
		})
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

		r.Route("/admin", func(r chi.Router) {
			r.Use(d.requireAuth, requireRole(models.AccountRoleCurator))
			r.Get("/cities", d.adminListCities)
			r.Post("/cities", d.adminCreateCity)
			r.Patch("/cities/{id}", d.adminUpdateCity)
			r.Get("/topics", d.adminListTopics)
			r.Post("/topics", d.adminCreateTopic)
			r.Patch("/topics/{id}", d.adminUpdateTopic)
			r.Get("/places", d.adminListPlaces)
			r.Post("/places", d.adminCreatePlace)
			r.Get("/places/{id}", d.adminGetPlace)
			r.Patch("/places/{id}", d.adminUpdatePlace)
			r.Put("/places/{id}/media", d.adminSetPlaceMedia)
			r.Delete("/places/{id}", d.adminSetPlaceStatus(models.EntityStatusArchived))
			r.Post("/places/{id}/publish", d.adminSetPlaceStatus(models.EntityStatusPublished))
			r.Post("/places/{id}/unpublish", d.adminSetPlaceStatus(models.EntityStatusDraft))
			r.Get("/moments", d.adminListMoments)
			r.Post("/moments", d.adminCreateMoment)
			r.Get("/moments/{id}", d.adminGetMoment)
			r.Patch("/moments/{id}", d.adminUpdateMoment)
			r.Delete("/moments/{id}", d.adminSetMomentStatus(models.EntityStatusArchived))
			r.Post("/moments/{id}/publish", d.adminPublishMoment)
			r.Post("/moments/{id}/unpublish", d.adminSetMomentStatus(models.EntityStatusDraft))
			r.Put("/moments/{id}/media", d.adminSetMomentMedia)
			r.Put("/moments/{id}/sources", d.adminSetMomentSources)
			r.Put("/moments/{id}/topics", d.adminSetMomentTopics)
			r.Get("/media", d.adminListMedia)
			r.Post("/media", d.adminUploadMedia)
			r.Post("/media/links", d.adminCreateMediaLink)
			r.Patch("/media/{id}", d.adminUpdateMedia)
			r.Get("/tours", d.adminListTours)
			r.Post("/tours", d.adminCreateTour)
			r.Get("/tours/{id}", d.adminGetTour)
			r.Patch("/tours/{id}", d.adminUpdateTour)
			r.Delete("/tours/{id}", d.adminDeleteTour)
			r.Put("/tours/{id}/stops", d.adminSetTourStops)
			r.Post("/tours/{id}/publish", d.adminPublishTour)
			r.Post("/tours/{id}/unpublish", d.adminUnpublishTour)
			r.Post("/tours/{id}/routes/recompute", d.adminRecomputeRoutes)
			r.Get("/candidates", d.adminListCandidates)
			r.Post("/candidates", d.adminCreateCandidate)
			r.Get("/candidates/{id}", d.adminGetCandidate)
			r.Post("/candidates/{id}/approve", d.adminApproveCandidate)
			r.Post("/candidates/{id}/reject", d.adminRejectCandidate)
			r.Get("/jobs", d.adminListJobs)
			r.Get("/jobs/{id}", d.adminGetJob)
			r.Post("/jobs/{id}/retry", d.adminRetryJob)
			r.Get("/sources", d.adminListSources)
			r.Post("/imports", d.adminCreateImport)
			r.Post("/candidates/{id}/draft", d.adminDraftCandidate)
			r.Post("/tours/{id}/stops/{position}/suggest", d.adminSuggestFeatured)
			r.Get("/stats", d.adminStats)
			r.Group(func(r chi.Router) {
				r.Use(requireRole(models.AccountRoleAdmin))
				r.Get("/accounts", d.adminListAccounts)
				r.Patch("/accounts/{id}/role", d.adminSetRole)
			})
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

// noDirListing serves files but answers 404 for directories.
func noDirListing(root http.FileSystem) http.Handler {
	fs := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasSuffix(p, "/") {
			http.NotFound(w, r)
			return
		}
		f, err := root.Open(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		info, err := f.Stat()
		f.Close()
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(strings.TrimPrefix(p, "/"), "documents/") {
			w.Header().Set("Content-Disposition", "attachment")
		}
		fs.ServeHTTP(w, r)
	})
}
