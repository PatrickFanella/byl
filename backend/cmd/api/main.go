// Command api serves the Invisible Museum HTTP API and runs the background job worker.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"invisible-museum/backend/internal/auth"
	"invisible-museum/backend/internal/config"
	"invisible-museum/backend/internal/db"
	httpapi "invisible-museum/backend/internal/http"
	"invisible-museum/backend/internal/jobs"
	"invisible-museum/backend/internal/providers"
	"invisible-museum/backend/internal/repository"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := db.Open(connectCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	prov, err := providers.Build(cfg)
	if err != nil {
		log.Error("build providers", "error", err)
		os.Exit(1)
	}

	store := repository.New(pool)
	sessions := auth.NewSessions(store, time.Duration(cfg.SessionTTLHours)*time.Hour)

	deps := httpapi.Deps{
		Store:          store,
		Sessions:       sessions,
		SessionManager: sessions,
		DevAuth:        cfg.DevAuthEnabled,
		IsBootstrap:    cfg.IsBootstrapAdmin,
		Blob:           prov.Blob,
		MediaFSDir:     prov.MediaFSDir,
		Log:            log,
		Ready:          pool.Ping,
	}
	if len(cfg.AppleClientIDs) > 0 {
		deps.Apple = auth.NewApple(cfg.AppleClientIDs)
	}
	if len(cfg.GoogleClientIDs) > 0 {
		deps.Google = auth.NewGoogle(cfg.GoogleClientIDs)
	}
	if cfg.DevAuthEnabled {
		log.Warn("DEV_AUTH_ENABLED is set: anyone can sign in by email; never run this way in production")
	}

	worker := jobs.New(store, prov.Router, prov.Narrator, prov.Blob, cfg.JobWorkers, log)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(ctx)
	}()

	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           httpapi.NewRouter(deps),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Info("starting api", "addr", cfg.APIAddr, "routing", cfg.RoutingProvider, "narration", cfg.NarrationProvider, "media", cfg.MediaStore, "workers", cfg.JobWorkers)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "error", err)
	}
	stop()
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		log.Warn("worker did not stop in time")
	}
}
