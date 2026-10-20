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

	// Comptes : facultatifs. Sans OIDC_ISSUER, on joue en invité seulement.
	issuer, client := os.Getenv("OIDC_ISSUER"), env("OIDC_CLIENT_ID", "coda")
	var verifier *server.Verifier
	if issuer != "" {
		vctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		verifier, err = server.NewVerifier(vctx, issuer, env("OIDC_AUDIENCE", client))
		cancel()
		if err != nil {
			return err
		}
	} else {
		log.Warn("OIDC_ISSUER absent : comptes désactivés")
		client = ""
	}

	var origins []string
	for _, o := range strings.Split(env("ALLOWED_ORIGINS", "localhost:5173"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}

	srv, err := server.New(server.Config{
		Store: store, Verifier: verifier, OIDCIssuer: issuer, OIDCClient: client,
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
