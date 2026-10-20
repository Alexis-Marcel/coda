package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	mrand "math/rand/v2"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alexis-marcel/coda/internal/game"
)

// live est une partie chargée en mémoire : un seul verrou par partie, des
// abonnés WebSocket, et une minuterie pour la fin du tour.
type live struct {
	mu    sync.Mutex
	rec   *record
	subs  map[*client]struct{}
	timer *time.Timer
}

var (
	errNotFound = errors.New("partie introuvable")
	errNotSeat  = errors.New("tu ne joues pas dans cette partie")
	errSameUser = errors.New("tu es déjà assis dans cette partie")
	errNotHost  = errors.New("seul l'hôte peut lancer ou annuler la partie")
	errClosed   = errors.New("la partie a déjà commencé")
)

const idAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func newID() string {
	var sb strings.Builder
	for range 8 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(idAlphabet))))
		sb.WriteByte(idAlphabet[n.Int64()])
	}
	return sb.String()
}

func newToken() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// newRand : un mélange tiré au hasard du système, pour la donne.
func newRand() *mrand.Rand {
	var seed [32]byte
	_, _ = rand.Read(seed[:])
	return mrand.New(mrand.NewChaCha8(seed))
}

func cleanName(name string, u *User) string {
	if u != nil && u.Name != "" {
		name = u.Name
	}
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > 24 {
		name = string([]rune(name)[:24])
	}
	if name == "" {
		name = "Invité"
	}
	return name
}

func userID(u *User) string {
	if u == nil {
		return ""
	}
	return u.ID
}

// get renvoie une partie, chargée depuis la base si besoin.
func (s *Server) get(id string) (*live, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.games[id]; ok {
		return g, nil
	}
	rec, err := s.store.load(id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, errNotFound
	}
	return s.adopt(rec), nil
}

// adopt met une partie en mémoire (verrou du serveur tenu).
func (s *Server) adopt(rec *record) *live {
	g := &live{rec: rec, subs: map[*client]struct{}{}}
	s.games[rec.ID] = g
	return g
}

func (s *Server) create(cfg game.Config, public bool, name string, u *User) (*record, string, error) {
	tok, hash := newToken()
	rec := &record{ID: newID(), Match: game.NewMatch(cfg), Public: public, Created: s.now()}
	if _, err := rec.Match.Sit(cleanName(name, u), userID(u)); err != nil {
		return nil, "", err
	}
	rec.Tokens = []string{hash}
	rec.Rematch = []bool{false}
	if err := s.store.save(rec); err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	s.adopt(rec)
	s.mu.Unlock()
	return rec, tok, nil
}

// join assied un joueur ; quand toutes les places sont prises, la partie démarre.
func (s *Server) join(id, name string, u *User) (string, int, error) {
	g, err := s.get(id)
	if err != nil {
		return "", 0, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	m := g.rec.Match
	if g.rec.Canceled || m.Phase != game.Waiting {
		return "", 0, errClosed
	}
	if u != nil {
		for _, p := range m.Players {
			if p.UserID == u.ID {
				return "", 0, errSameUser
			}
		}
	}
	seat, err := m.Sit(cleanName(name, u), userID(u))
	if err != nil {
		return "", 0, err
	}
	tok, hash := newToken()
	g.rec.Tokens = append(g.rec.Tokens, hash)
	g.rec.Rematch = append(g.rec.Rematch, false)
	if m.Full() {
		_ = m.Start(newRand(), mrand.IntN(len(m.Players)), s.now())
	}
	s.commit(g)
	return tok, seat, nil
}

// seatOf retrouve la place d'un client : par son jeton de place, sinon par son compte.
func (g *live) seatOf(token string, u *User) int {
	if token != "" {
		h := hashToken(token)
		for i, t := range g.rec.Tokens {
			if t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(h)) == 1 {
				return i
			}
		}
	}
	if u != nil {
		for i, p := range g.rec.Match.Players {
			if p.UserID == u.ID {
				return i
			}
		}
	}
	return -1
}

// commit enregistre la partie, réarme sa minuterie et prévient les abonnés
// (verrou de la partie tenu).
func (s *Server) commit(g *live) {
	if err := s.store.save(g.rec); err != nil {
		s.log.Error("enregistrement", "game", g.rec.ID, "err", err)
	}
	s.arm(g)
	s.broadcast(g)
}

func (s *Server) arm(g *live) {
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	d := g.rec.Match.Deadline()
	if d.IsZero() || g.rec.Canceled {
		return
	}
	g.timer = time.AfterFunc(time.Until(d)+20*time.Millisecond, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.rec.Match.Tick(s.now()) {
			s.commit(g)
		} else {
			s.arm(g)
		}
	})
}

type command struct {
	T      string     `json:"t"`
	Color  game.Color `json:"color"`
	Target int        `json:"target"`
	Tile   int        `json:"tile"`
	Num    int        `json:"num"`
}

// apply joue une commande d'un joueur.
func (s *Server) apply(g *live, seat int, c command) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if seat < 0 {
		return errNotSeat
	}
	m, now := g.rec.Match, s.now()
	if m.Tick(now) {
		// Le tour vient de se jouer tout seul : la commande arrive trop tard.
		s.commit(g)
		return game.ErrNotYourTurn
	}
	var err error
	switch c.T {
	case "start":
		if seat != 0 {
			return errNotHost
		}
		if g.rec.Canceled {
			return game.ErrPhase
		}
		err = m.Start(newRand(), mrand.IntN(max(len(m.Players), 1)), now)
	case "draw":
		err = m.Draw(seat, c.Color, now)
	case "guess":
		_, err = m.Guess(seat, c.Target, c.Tile, c.Num, now)
	case "stop":
		err = m.Stop(seat, now)
	case "reveal":
		err = m.Reveal(seat, c.Tile, now)
	case "resign":
		err = m.Resign(seat, now)
	case "cancel":
		if seat != 0 {
			return errNotHost
		}
		if m.Phase != game.Waiting || g.rec.Canceled {
			return game.ErrPhase
		}
		g.rec.Canceled = true
	case "rematch":
		err = s.rematch(g, seat)
	default:
		return errors.New("commande inconnue")
	}
	if err != nil {
		return err
	}
	s.commit(g)
	return nil
}

// rematch : quand tous la demandent, une nouvelle partie démarre avec les
// mêmes joueurs (mêmes jetons), le joueur suivant commençant.
func (s *Server) rematch(g *live, seat int) error {
	rec := g.rec
	if rec.Match.Phase != game.Finished || rec.Next != "" {
		return game.ErrPhase
	}
	rec.Rematch[seat] = true
	for _, r := range rec.Rematch {
		if !r {
			return nil
		}
	}
	now := s.now()
	next := &record{ID: newID(), Match: game.NewMatch(rec.Match.Config), Tokens: rec.Tokens,
		Rematch: make([]bool, len(rec.Tokens)), Created: now}
	for _, p := range rec.Match.Players {
		if _, err := next.Match.Sit(p.Name, p.UserID); err != nil {
			return err
		}
	}
	if err := next.Match.Start(newRand(), rec.Match.First+1, now); err != nil {
		return err
	}
	if err := s.store.save(next); err != nil {
		return err
	}
	s.mu.Lock()
	ng := s.adopt(next)
	s.mu.Unlock()
	ng.mu.Lock()
	s.arm(ng)
	ng.mu.Unlock()
	rec.Next = next.ID
	return nil
}

// openGames liste les parties publiques en attente dont l'hôte est là.
func (s *Server) openGames() []openGame {
	s.mu.Lock()
	games := make([]*live, 0, len(s.games))
	for _, g := range s.games {
		games = append(games, g)
	}
	s.mu.Unlock()
	out := []openGame{}
	for _, g := range games {
		g.mu.Lock()
		r := g.rec
		if r.Public && !r.Canceled && r.Match.Phase == game.Waiting && g.online(0) {
			out = append(out, openGame{ID: r.ID, Host: r.Match.Players[0].Name, Players: len(r.Match.Players),
				Seats: r.Match.Config.Seats, TurnMs: r.Match.Config.TurnTime.Milliseconds(), Created: r.Created})
		}
		g.mu.Unlock()
	}
	return out
}

type openGame struct {
	ID      string    `json:"id"`
	Host    string    `json:"host"`
	Players int       `json:"players"`
	Seats   int       `json:"seats"`
	TurnMs  int64     `json:"turnMs"`
	Created time.Time `json:"created"`
}

func (g *live) online(seat int) bool {
	for c := range g.subs {
		if c.seat == seat {
			return true
		}
	}
	return false
}

// pauseAll marque l'heure d'arrêt de toutes les parties en cours.
func (s *Server) pauseAll() {
	s.mu.Lock()
	games := make([]*live, 0, len(s.games))
	for _, g := range s.games {
		games = append(games, g)
	}
	s.mu.Unlock()
	now := s.now()
	for _, g := range games {
		g.mu.Lock()
		if g.timer != nil {
			g.timer.Stop()
		}
		if !g.rec.Canceled && g.rec.Match.Phase == game.Playing {
			g.rec.PausedAt = now
			if err := s.store.save(g.rec); err != nil {
				s.log.Error("pause", "game", g.rec.ID, "err", err)
			}
		}
		g.mu.Unlock()
	}
}

// resume recharge les parties en cours au démarrage. Le temps passé serveur
// arrêté n'est décompté à personne.
func (s *Server) resume() error {
	recs, err := s.store.unfinished()
	if err != nil {
		return err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range recs {
		if rec.Canceled {
			continue
		}
		if !rec.PausedAt.IsZero() {
			rec.Match.TurnStart = rec.Match.TurnStart.Add(now.Sub(rec.PausedAt))
			rec.PausedAt = time.Time{}
		}
		g := s.adopt(rec)
		g.mu.Lock()
		s.arm(g)
		g.mu.Unlock()
	}
	s.log.Info("parties reprises", "count", len(recs))
	return nil
}

// sweep retire de la mémoire les parties finies (ou en attente depuis longtemps)
// que plus personne ne regarde.
func (s *Server) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, g := range s.games {
		// Ordre des verrous : partie puis serveur (revanche). Ici l'inverse, donc
		// on passe une partie occupée.
		if !g.mu.TryLock() {
			continue
		}
		r := g.rec
		idle := len(g.subs) == 0
		done := r.Canceled || r.Match.Phase == game.Finished
		stale := r.Match.Phase == game.Waiting && now.Sub(r.Created) > 24*time.Hour
		if idle && (done || stale) {
			if g.timer != nil {
				g.timer.Stop()
			}
			delete(s.games, id)
		}
		g.mu.Unlock()
	}
}
