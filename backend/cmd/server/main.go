package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mhandewith/StoryForge/backend/internal/api"
	"github.com/mhandewith/StoryForge/backend/internal/identity"
	"github.com/mhandewith/StoryForge/backend/migrations"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func handler(db *pgxpool.Pool, webDir string, options ...*api.API) http.Handler {
	mux := http.NewServeMux()
	a := &api.API{DB: db}
	if len(options) > 0 {
		a = options[0]
	}
	a.Register(mux)
	protected := a.Protect(mux)
	mux.Handle("GET /assets/", http.FileServer(http.Dir(webDir)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		api.JSON(w, 200, map[string]string{"status": "ok", "service": "StoryForge"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; media-src 'self' blob:; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "microphone=(self), camera=()")
		protected.ServeHTTP(w, r)
	})
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("DATABASE_URL") == "" && os.Getenv("PGHOST") == "" {
		return errors.New("configure DATABASE_URL or PGHOST/PGUSER/PGPASSWORD/PGDATABASE before starting StoryForge")
	}
	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("invalid PostgreSQL connection configuration")
	}
	defer db.Close()
	// Unraid may start the app before PostgreSQL finishes initializing.
	startup, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		pingCtx, pingCancel := context.WithTimeout(startup, 2*time.Second)
		err = db.Ping(pingCtx)
		pingCancel()
		if err == nil {
			break
		}
		slog.Info("Waiting for PostgreSQL")
		select {
		case <-startup.Done():
			return errors.New("PostgreSQL did not become available within 60 seconds; check connection settings and database logs")
		case <-time.After(time.Second):
		}
	}
	if err = migrations.Apply(startup, db); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}
	auth, err := identity.FromEnv()
	if err != nil {
		return err
	}
	recordings := os.Getenv("STORYFORGE_RECORDINGS_DIR")
	if recordings != "" {
		if err := api.CheckStorage(recordings); err != nil {
			return fmt.Errorf("recording storage: %w", err)
		}
	}
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "../frontend/dist"
	}
	if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
		return errors.New("frontend build missing; run npm run build in frontend or configure WEB_DIR")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	eleven, err := api.NewElevenClient()
	if err != nil {
		return err
	}
	service := &api.API{DB: db, Auth: auth, RecordingsDir: recordings, Eleven: eleven}
	stopVoiceWorker := service.StartVoiceWorker(ctx)
	defer stopVoiceWorker()
	server := &http.Server{
		Addr: ":" + port, Handler: handler(db, webDir, service),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	result := make(chan error, 1)
	go func() {
		slog.Info("StoryForge backend starting", "address", server.Addr)
		result <- server.ListenAndServe()
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		slog.Info("StoryForge backend shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func main() {
	if err := run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("StoryForge backend stopped", "error", err)
		os.Exit(1)
	}
}
