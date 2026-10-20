package server

import (
	"slices"
	"time"

	"github.com/alexis-marcel/coda/internal/game"
)

// La vue d'une partie dépend de qui regarde : chacun voit la valeur de ses
// tuiles et de celles qui sont retournées ; des autres, seulement la couleur.
// La tuile piochée n'est connue que de celui qui l'a piochée. Tout est dévoilé
// à la fin. Le journal ne contient rien de secret : il est envoyé tel quel.
// La vue est encodée hors du verrou de la partie : elle ne partage rien avec elle.

type tileView struct {
	ID       int        `json:"id"`
	Color    game.Color `json:"color"`
	Num      *int       `json:"num,omitempty"` // absent : valeur cachée pour ce joueur
	Revealed bool       `json:"revealed"`
}

type playerView struct {
	Name    string     `json:"name"`
	Account bool       `json:"account"`
	Online  bool       `json:"online"`
	Out     bool       `json:"out"`
	Rank    int        `json:"rank"`
	Row     []tileView `json:"row"`
}

type gameView struct {
	ID         string        `json:"id"`
	Phase      string        `json:"phase"`
	You        int           `json:"you"`
	Seats      int           `json:"seats"`
	Turn       int           `json:"turn"`
	Step       game.Step     `json:"step"`
	TurnMs     int64         `json:"turnMs"`
	TurnLeftMs int64         `json:"turnLeftMs"`
	Drawn      *tileView     `json:"drawn,omitempty"`
	Pile       [2]int        `json:"pile"`
	Players    []playerView  `json:"players"`
	Actions    []game.Action `json:"actions"`
	Result     *game.Result  `json:"result,omitempty"`
	Rematch    []bool        `json:"rematch"`
	Next       string        `json:"next,omitempty"`
	Public     bool          `json:"public"`
}

func tileFor(t game.Tile, visible bool) tileView {
	v := tileView{ID: t.ID, Color: t.Color, Revealed: t.Revealed}
	if visible || t.Revealed {
		n := t.Num
		v.Num = &n
	}
	return v
}

func (g *live) view(seat int, now time.Time) gameView {
	r, m := g.rec, g.rec.Match
	phase := string(m.Phase)
	if r.Canceled {
		phase = "canceled"
	}
	done := m.Phase == game.Finished
	v := gameView{
		ID: r.ID, Phase: phase, You: seat, Seats: m.Config.Seats, TurnMs: m.Config.TurnTime.Milliseconds(),
		Pile: m.PileCount(), Actions: slices.Clone(m.Actions), Rematch: slices.Clone(r.Rematch), Next: r.Next, Public: r.Public,
		Players: []playerView{},
	}
	if m.Result != nil {
		res := *m.Result
		v.Result = &res
	}
	if v.Actions == nil {
		v.Actions = []game.Action{}
	}
	if m.Phase == game.Playing {
		v.Turn, v.Step, v.TurnLeftMs = m.Turn, m.Step, m.Left(now).Milliseconds()
		if m.Drawn != nil {
			d := tileFor(*m.Drawn, seat == m.Turn)
			v.Drawn = &d
		}
	}
	for i, p := range m.Players {
		pv := playerView{Name: p.Name, Account: p.UserID != "", Online: g.online(i), Out: p.Out, Rank: p.Rank, Row: []tileView{}}
		for _, t := range p.Row {
			pv.Row = append(pv.Row, tileFor(t, done || i == seat))
		}
		v.Players = append(v.Players, pv)
	}
	return v
}
