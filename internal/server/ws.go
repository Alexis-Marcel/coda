package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/alexis-marcel/coda/internal/game"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Un client WebSocket suit une partie. À la connexion puis après chaque
// changement, il reçoit {"t":"state"} avec sa vue ; il envoie des commandes
// {"t":"guess","target":1,"tile":7,"num":4}… et reçoit {"t":"error"} si l'une
// est refusée.

type client struct {
	seat int
	send chan any
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	g, err := s.get(r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "partie indisponible")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.cfg.Origins})
	if err != nil {
		return
	}
	conn.SetReadLimit(32 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	u := s.user(r)
	g.mu.Lock()
	c := &client{seat: g.seatOf(r.URL.Query().Get("token"), u), send: make(chan any, 16)}
	g.subs[c] = struct{}{}
	// Arrivée : tout le monde voit le joueur en ligne.
	s.broadcast(g)
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.subs, c)
		s.broadcast(g)
		g.mu.Unlock()
		conn.CloseNow()
	}()

	go func() {
		defer cancel()
		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-c.send:
				wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
				err := wsjson.Write(wctx, conn, msg)
				wcancel()
				if err != nil {
					return
				}
			case <-ping.C:
				pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Ping(pctx)
				pcancel()
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		var cmd command
		if err := wsjson.Read(ctx, conn, &cmd); err != nil {
			var syntax *json.SyntaxError
			if errors.As(err, &syntax) {
				continue
			}
			return
		}
		if err := s.apply(g, c.seat, cmd); err != nil {
			msg := err.Error()
			if !isGameError(err) {
				s.log.Error("commande", "game", g.rec.ID, "t", cmd.T, "err", err)
				msg = "erreur du serveur"
			}
			select {
			case c.send <- map[string]string{"t": "error", "message": msg}:
			default:
			}
		}
	}
}

func isGameError(err error) bool {
	for _, e := range []error{game.ErrFull, game.ErrPhase, game.ErrFewPlayers, game.ErrBadSeat, game.ErrNotYourTurn,
		game.ErrStep, game.ErrEmptyColor, game.ErrBadTarget, game.ErrBadTile, game.ErrBadNum, game.ErrOver,
		errNotSeat, errNotHost} {
		if errors.Is(err, e) {
			return true
		}
	}
	return err.Error() == "commande inconnue"
}

// broadcast envoie à chaque abonné sa vue (verrou de la partie tenu).
func (s *Server) broadcast(g *live) {
	now := s.now()
	for c := range g.subs {
		msg := map[string]any{"t": "state", "state": g.view(c.seat, now)}
		select {
		case c.send <- msg:
		default:
			// File pleine : on jette le plus ancien, le nouvel état suffit.
			select {
			case <-c.send:
			default:
			}
			select {
			case c.send <- msg:
			default:
			}
		}
	}
}
