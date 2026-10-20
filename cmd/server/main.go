// Commande server : le serveur Coda (API, WebSocket et front compilé).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alexis-marcel/coda/internal/server"
	"github.com/alexis-marcel/gamekit/auth"
	"github.com/alexis-marcel/gamekit/portal"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	// « coda health » : sonde de santé du conteneur (l'image n'a pas de shell).
	if len(os.Args) > 1 && os.Args[1] == "health" {
		port := strings.TrimPrefix(env("ADDR", ":8080"), ":")
		c := &http.Client{Timeout: 3 * time.Second}
		res, err := c.Get("http://127.0.0.1:" + port + "/healthz")
		if err != nil || res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("arrêt", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dataDir := env("DATA_DIR", "data")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return err
	}
	store, err := server.OpenStore(filepath.Join(dataDir, "coda.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	// Comptes : facultatifs, méthode au choix de l'hébergeur (AUTH_MODE, voir gamekit/auth).
	acfg, err := auth.FromEnv(os.Getenv, "coda")
	if err != nil {
		return err
	}
	if acfg.Mode == auth.ModeLocal {
		// Comptes intégrés : dans la base du jeu, pages /compte/ à ses couleurs.
		if acfg.LocalStore, err = auth.NewSQLStore(ctx, store.DB(), auth.SQLite); err != nil {
			return err
		}
		acfg.Theme = auth.Theme{
			Title: "Coda", Background: "#243038", Surface: "#2b3942", Text: "#efe9dc", Muted: "#9fb0b7",
			Line: "#44575f", Accent: "#d1a94f", OnAccent: "#15171c", Danger: "#e8846e",
			Font:    `"Instrument Sans", system-ui, sans-serif`,
			FontURL: "https://fonts.googleapis.com/css2?family=Instrument+Sans:wght@400;600&display=swap",
		}
	}
	actx, cancel := context.WithTimeout(ctx, 15*time.Second)
	authn, err := auth.New(actx, acfg, log)
	cancel()
	if err != nil {
		return err
	}
	log.Info("comptes", "mode", authn.Info().Mode, "invités", authn.Info().GuestPlay)

	var origins []string
	for _, o := range strings.Split(env("ALLOWED_ORIGINS", "localhost:5173"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}

	srv, err := server.New(server.Config{
		Store: store, Auth: authn, Portal: portal.FromEnv(os.Getenv),
		StaticDir: os.Getenv("STATIC_DIR"), Origins: origins, Log: log,
	})
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { srv.Run(ctx); close(done) }()

	httpSrv := &http.Server{Addr: env("ADDR", ":8080"), Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(sctx)
	}()
	log.Info("coda à l'écoute", "addr", httpSrv.Addr)
	if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-done
	return nil
}
