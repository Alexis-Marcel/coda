// Package server sert Coda : API REST, WebSocket par partie, front compilé.
// Un seul processus : les parties vivent en mémoire et sont enregistrées dans
// SQLite à chaque coup.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alexis-marcel/coda/internal/game"
	"github.com/alexis-marcel/gamekit/auth"
	"github.com/alexis-marcel/gamekit/portal"
)

type Config struct {
	Store     *Store
	Auth      auth.Authenticator // nil : pas de comptes (auth.None)
	Portal    *portal.Info       // nil : le jeu vit seul, sans barre de portail
	StaticDir string             // front compilé ; vide : API seule
	Origins   []string           // origines acceptées pour le WebSocket (en plus du même hôte)
	Log       *slog.Logger
}

type Server struct {
	cfg   Config
	store *Store
	auth  auth.Authenticator
	log   *slog.Logger
	now   func() time.Time

	mu    sync.Mutex
	games map[string]*live
}

func New(cfg Config) (*Server, error) {
	s := &Server{cfg: cfg, store: cfg.Store, auth: cfg.Auth, log: cfg.Log, now: time.Now, games: map[string]*live{}}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.auth == nil {
		s.auth = auth.None{}
	}
	if err := s.resume(); err != nil {
		return nil, err
	}
	return s, nil
}

// Run fait le ménage en mémoire jusqu'à l'arrêt, puis met les pendules en pause.
func (s *Server) Run(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.pauseAll()
			return
		case <-t.C:
			s.sweep()
		}
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.store.Ping(r.Context()); err != nil {
			http.Error(w, "db", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("POST /api/games", s.handleCreate)
	mux.HandleFunc("GET /api/games/open", s.handleOpen)
	mux.HandleFunc("POST /api/games/current", s.handleCurrent)
	mux.HandleFunc("GET /api/games/{id}", s.handleGame)
	mux.HandleFunc("POST /api/games/{id}/join", s.handleJoin)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("GET /api/users/{id}", s.handleUserStats)
	mux.HandleFunc("GET /ws/games/{id}", s.handleWS)
	if m, ok := s.auth.(auth.Mounter); ok {
		m.Mount(mux) // pages de comptes du mode local, sous /compte/
	}
	if s.cfg.StaticDir != "" {
		mux.Handle("/", spa(s.cfg.StaticDir))
	}
	return limitBody(mux)
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		next.ServeHTTP(w, r)
	})
}

// spa sert les fichiers du front, et index.html pour toute autre adresse.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	// user : le compte de la requête, utile au front quand c'est le serveur qui
	// le connaît (mode proxy) ; en mode oidc, le front a déjà le sien.
	writeJSON(w, http.StatusOK, map[string]any{"auth": s.auth.Info(), "user": s.user(r), "portal": s.cfg.Portal})
}

// Temps par tour proposés, en secondes (0 : illimité).
var turnTimes = map[int]bool{30: true, 60: true, 120: true, 0: true}

// guestAllowed refuse une requête sans compte quand l'hébergeur exige un
// compte pour jouer (GUEST_PLAY=false).
func (s *Server) guestAllowed(w http.ResponseWriter, r *http.Request) bool {
	if s.auth.Info().GuestPlay || s.user(r) != nil {
		return true
	}
	writeError(w, http.StatusUnauthorized, "connecte-toi pour jouer")
	return false
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !s.guestAllowed(w, r) {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Seats    int    `json:"seats"`
		TurnTime *int   `json:"turnTime"`
		Public   bool   `json:"public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "requête illisible")
		return
	}
	cfg := game.DefaultConfig
	if req.Seats >= 2 && req.Seats <= game.MaxSeats {
		cfg.Seats = req.Seats
	}
	if req.TurnTime != nil && turnTimes[*req.TurnTime] {
		cfg.TurnTime = time.Duration(*req.TurnTime) * time.Second
	}
	rec, tok, err := s.create(cfg, req.Public, req.Name, s.user(r))
	if err != nil {
		s.log.Error("création", "err", err)
		writeError(w, http.StatusInternalServerError, "création impossible")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": rec.ID, "token": tok, "seat": 0})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if !s.guestAllowed(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	tok, seat, err := s.join(r.PathValue("id"), req.Name, s.user(r))
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, game.ErrFull), errors.Is(err, errClosed), errors.Is(err, errSameUser):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		s.log.Error("arrivée", "err", err)
		writeError(w, http.StatusInternalServerError, "impossible de rejoindre")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"token": tok, "seat": seat})
	}
}

// handleGame dit si une partie existe, et ce qu'en voit un inconnu.
func (s *Server) handleGame(w http.ResponseWriter, r *http.Request) {
	g, err := s.get(r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "partie indisponible")
		return
	}
	g.mu.Lock()
	v := g.view(-1, s.now())
	g.mu.Unlock()
	writeJSON(w, http.StatusOK, v)
}

// handleCurrent : les parties en cours du navigateur (ses jetons de place, au
// plus 200) et du compte connecté.
func (s *Server) handleCurrent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tokens map[string]string `json:"tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Tokens) > 200 {
		writeError(w, http.StatusBadRequest, "requête invalide")
		return
	}
	writeJSON(w, http.StatusOK, s.currentGames(req.Tokens, s.user(r)))
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.openGames())
}

type historyItem struct {
	ID        string      `json:"id"`
	Opponents []string    `json:"opponents"`
	Rank      int         `json:"rank"`
	Players   int         `json:"players"`
	Reason    game.Reason `json:"reason"`
	At        time.Time   `json:"at"`
}

// handleUserStats : le bilan public d'un compte (parties finies), par son
// identifiant de compte. Pour un portail qui affiche les parties de chacun.
func (s *Server) handleUserStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.stats(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bilan indisponible")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := s.user(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "connexion nécessaire")
		return
	}
	recs, err := s.store.history(u.ID, 30)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "historique indisponible")
		return
	}
	st, err := s.store.stats(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "historique indisponible")
		return
	}
	items := []historyItem{}
	for _, rec := range recs {
		m := rec.Match
		it := historyItem{ID: rec.ID, Opponents: []string{}, Players: len(m.Players), Reason: m.Result.Reason, At: m.TurnStart}
		for _, p := range m.Players {
			if p.UserID == u.ID {
				it.Rank = p.Rank
			} else {
				it.Opponents = append(it.Opponents, p.Name)
			}
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": u.Name, "stats": st, "games": items})
}
