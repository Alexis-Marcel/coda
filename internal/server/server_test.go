package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexis-marcel/coda/internal/game"
	"github.com/alexis-marcel/gamekit/auth"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type harness struct {
	t     *testing.T
	srv   *Server
	http  *httptest.Server
	store *Store
}

// testAuth : compte de test « Authorization: Bearer user:<id> ».
type testAuth struct{ guests bool }

func (testAuth) User(r *http.Request) *User {
	if b := auth.Bearer(r); strings.HasPrefix(b, "user:") {
		id := strings.TrimPrefix(b, "user:")
		return &User{ID: id, Name: "Compte " + id}
	}
	return nil
}

func (a testAuth) Info() auth.Info { return auth.Info{Mode: "test", GuestPlay: a.guests} }

func newHarness(t *testing.T) *harness {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return startHarness(t, store)
}

func startHarness(t *testing.T, store *Store) *harness {
	t.Helper()
	srv, err := New(Config{Store: store, Auth: testAuth{guests: true}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, srv: srv, http: httptest.NewServer(srv.Handler()), store: store}
	t.Cleanup(h.http.Close)
	return h
}

func (h *harness) post(path, auth string, body any, out any) int {
	h.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, h.http.URL+path, bytes.NewReader(b))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func (h *harness) get(path, auth string, out any) int {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.http.URL+path, nil)
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
}

type msg struct {
	T       string    `json:"t"`
	Message string    `json:"message"`
	State   *gameView `json:"state"`
}

func (h *harness) dial(id, query string) *wsClient {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(h.http.URL, "http") + "/ws/games/" + id + "?" + query
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { conn.CloseNow() })
	return &wsClient{t: h.t, conn: conn}
}

func (c *wsClient) send(cmd any) {
	c.t.Helper()
	if err := wsjson.Write(context.Background(), c.conn, cmd); err != nil {
		c.t.Fatal(err)
	}
}

// until lit jusqu'à un message qui satisfait ok.
func (c *wsClient) until(ok func(msg) bool) msg {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		var m msg
		if err := wsjson.Read(ctx, c.conn, &m); err != nil {
			c.t.Fatalf("lecture : %v", err)
		}
		if ok(m) {
			return m
		}
	}
}

func phase(p string) func(msg) bool {
	return func(m msg) bool { return m.T == "state" && m.State.Phase == p }
}

func isError(m msg) bool { return m.T == "error" }

type created struct {
	ID    string `json:"id"`
	Token string `json:"token"`
	Seat  int    `json:"seat"`
}

func TestFullGame(t *testing.T) {
	h := newHarness(t)

	var c created
	if st := h.post("/api/games", "user:alice", map[string]any{"name": "ignoré", "seats": 3, "turnTime": 30, "public": true}, &c); st != http.StatusCreated {
		t.Fatalf("création %d", st)
	}
	a := h.dial(c.ID, "token="+c.Token)
	a.until(phase("waiting"))

	var open []openGame
	h.get("/api/games/open", "", &open)
	if len(open) != 1 || open[0].Host != "Compte alice" || open[0].Seats != 3 || open[0].TurnMs != 30_000 {
		t.Fatalf("parties ouvertes %+v", open)
	}
	// Le même compte ne s'assied pas deux fois.
	if st := h.post("/api/games/"+c.ID+"/join", "user:alice", map[string]string{}, nil); st != http.StatusConflict {
		t.Fatalf("deux fois assis %d", st)
	}
	var jb, jc created
	if st := h.post("/api/games/"+c.ID+"/join", "", map[string]string{"name": "  Bob   le  malin  "}, &jb); st != http.StatusOK || jb.Seat != 1 {
		t.Fatalf("arrivée %d", st)
	}
	b := h.dial(c.ID, "token="+jb.Token)
	// Bob n'est pas l'hôte : il ne lance pas la partie.
	b.send(command{T: "start"})
	if m := b.until(isError); m.Message != errNotHost.Error() {
		t.Fatalf("départ par un invité : %q", m.Message)
	}
	h.post("/api/games/"+c.ID+"/join", "user:carol", map[string]string{}, &jc)
	if st := h.post("/api/games/"+c.ID+"/join", "", map[string]string{}, nil); st != http.StatusConflict {
		t.Fatalf("quatrième joueur %d", st)
	}
	if st := h.post("/api/games/nope/join", "", map[string]string{}, nil); st != http.StatusNotFound {
		t.Fatalf("partie inconnue %d", st)
	}
	clients := []*wsClient{a, b, h.dial(c.ID, "token="+jc.Token)}

	// Parties en cours : par le jeton de place, ou par le compte ; un faux jeton ne trouve rien.
	var cur []currentGame
	h.post("/api/games/current", "", map[string]any{"tokens": map[string]string{c.ID: jb.Token, "nope": "x"}}, &cur)
	if len(cur) != 1 || cur[0].Seat != 1 || !slices.Equal(cur[0].Opponents, []string{"Compte alice", "Compte carol"}) {
		t.Fatalf("en cours (jeton) %+v", cur)
	}
	h.post("/api/games/current", "user:carol", map[string]any{}, &cur)
	if len(cur) != 1 || cur[0].Seat != 2 || cur[0].Players != 3 {
		t.Fatalf("en cours (compte) %+v", cur)
	}
	h.post("/api/games/current", "", map[string]any{"tokens": map[string]string{c.ID: "faux"}}, &cur)
	if len(cur) != 0 {
		t.Fatalf("en cours (mauvais jeton) %+v", cur)
	}

	// Partie pleine : elle démarre seule.
	s := b.until(phase("playing")).State
	if s.You != 1 || s.Players[1].Name != "Bob le malin" || !s.Players[0].Account || len(s.Players) != 3 || s.Step != game.StepDraw {
		t.Fatalf("vue de Bob %+v", s)
	}
	for i, p := range s.Players {
		for _, tile := range p.Row {
			if (tile.Num != nil) != (i == 1) {
				t.Fatalf("valeur de la tuile %d du joueur %d mal montrée", tile.ID, i)
			}
		}
	}
	spec := h.dial(c.ID, "")
	if s := spec.until(phase("playing")).State; s.You != -1 {
		t.Fatal("spectateur assis")
	}
	spec.send(command{T: "resign"})
	if m := spec.until(isError); m.Message != errNotSeat.Error() {
		t.Fatalf("spectateur : %q", m.Message)
	}

	// Le joueur dont c'est le tour pioche : lui seul voit la valeur piochée.
	turn := s.Turn
	other := (turn + 1) % 3
	color := game.White
	if s.Pile[game.White] == 0 {
		color = game.Black
	}
	clients[turn].send(command{T: "draw", Color: color})
	ts := clients[turn].until(func(m msg) bool { return m.T == "state" && m.State.Drawn != nil }).State
	obs := clients[other].until(func(m msg) bool { return m.T == "state" && m.State.Drawn != nil }).State
	if ts.Drawn.Num == nil || obs.Drawn.Num != nil || obs.Drawn.Color != color || obs.Pile != ts.Pile {
		t.Fatalf("tuile piochée mal cachée : %+v / %+v", ts.Drawn, obs.Drawn)
	}
	clients[other].send(command{T: "stop"})
	if m := clients[other].until(isError); m.Message != game.ErrNotYourTurn.Error() {
		t.Fatalf("hors tour : %q", m.Message)
	}

	// On lit les valeurs dans le moteur pour gagner : le joueur du tour élimine les deux autres.
	g, _ := h.srv.get(c.ID)
	for _, target := range []int{(turn + 1) % 3, (turn + 2) % 3} {
		for {
			g.mu.Lock()
			var tile *game.Tile
			for _, x := range g.rec.Match.Players[target].Row {
				if !x.Revealed {
					tile = &x
					break
				}
			}
			g.mu.Unlock()
			if tile == nil {
				break
			}
			if err := h.srv.apply(g, turn, command{T: "guess", Target: target, Tile: tile.ID, Num: tile.Num}); err != nil {
				t.Fatal(err)
			}
		}
	}
	end := a.until(phase("finished")).State
	if end.Result == nil || end.Result.Winner != turn || end.Result.Reason != game.ByLastStanding {
		t.Fatalf("fin %+v", end.Result)
	}
	for _, p := range end.Players {
		for _, tile := range p.Row {
			if tile.Num == nil {
				t.Fatal("tout est dévoilé à la fin")
			}
		}
	}

	// Revanche : tous la demandent, une nouvelle partie démarre.
	for _, cl := range clients {
		cl.send(command{T: "rematch"})
	}
	next := a.until(func(m msg) bool { return m.T == "state" && m.State.Next != "" }).State.Next
	ns := h.dial(next, "token="+c.Token).until(phase("playing")).State
	if ns.You != 0 || len(ns.Players) != 3 {
		t.Fatalf("revanche %+v", ns)
	}

	// Historique des comptes.
	var me struct {
		Stats stats         `json:"stats"`
		Games []historyItem `json:"games"`
	}
	if st := h.get("/api/me", "user:carol", &me); st != http.StatusOK || me.Stats.Played != 1 || len(me.Games) != 1 ||
		len(me.Games[0].Opponents) != 2 || me.Games[0].Players != 3 {
		t.Fatalf("historique %d %+v", st, me)
	}
	if won := turn == 2; (me.Stats.Won == 1) != won || (me.Games[0].Rank == 1) != won {
		t.Fatalf("victoire de Carol mal comptée : %+v", me)
	}
	var pub stats
	if st := h.get("/api/users/alice", "", &pub); st != http.StatusOK || pub.Played != 1 {
		t.Fatalf("bilan public %d %+v", st, pub)
	}
	if st := h.get("/api/users/inconnu", "", &pub); st != http.StatusOK || pub.Played != 0 {
		t.Fatalf("bilan d'un inconnu %d %+v", st, pub)
	}
	if st := h.get("/api/me", "", nil); st != http.StatusUnauthorized {
		t.Fatalf("historique invité %d", st)
	}
}

func TestStartEarlyCancelAndResume(t *testing.T) {
	h := newHarness(t)
	var c1, c2, j created
	h.post("/api/games", "", map[string]any{"name": "A"}, &c1)
	a := h.dial(c1.ID, "token="+c1.Token)
	a.until(phase("waiting"))
	a.send(command{T: "start"})
	if m := a.until(isError); m.Message != game.ErrFewPlayers.Error() {
		t.Fatalf("départ seul : %q", m.Message)
	}
	a.send(command{T: "cancel"})
	a.until(phase("canceled"))
	if st := h.post("/api/games/"+c1.ID+"/join", "", map[string]string{}, nil); st != http.StatusConflict {
		t.Fatalf("partie annulée rejointe %d", st)
	}

	// À quatre places, l'hôte lance à deux.
	h.post("/api/games", "", map[string]any{"name": "A", "seats": 4, "turnTime": 60}, &c2)
	h.post("/api/games/"+c2.ID+"/join", "", map[string]string{"name": "B"}, &j)
	g, _ := h.srv.get(c2.ID)
	if err := h.srv.apply(g, 0, command{T: "start"}); err != nil {
		t.Fatal(err)
	}
	if m := g.rec.Match; m.Phase != game.Playing || m.Config.Seats != 2 || len(m.Players[0].Row) != 4 {
		t.Fatal("départ anticipé")
	}
	if st := h.post("/api/games/"+c2.ID+"/join", "", map[string]string{}, nil); st != http.StatusConflict {
		t.Fatalf("arrivée après le départ %d", st)
	}

	// Une partie en cours survit à un redémarrage, le tour en pause.
	h.srv.pauseAll()
	h2 := startHarness(t, h.store)
	h2.srv.now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := h2.srv.resume(); err != nil {
		t.Fatal(err)
	}
	s := h2.dial(c2.ID, "token="+j.Token).until(phase("playing")).State
	if s.TurnLeftMs < 55_000 {
		t.Fatalf("tour après redémarrage : %d", s.TurnLeftMs)
	}
}

// Le temps écoulé joue le tour : la minuterie du serveur passe la main.
func TestTurnTimeout(t *testing.T) {
	h := newHarness(t)
	var c, j created
	h.post("/api/games", "", map[string]any{"name": "A"}, &c)
	h.post("/api/games/"+c.ID+"/join", "", map[string]string{"name": "B"}, &j)
	g, _ := h.srv.get(c.ID)
	g.mu.Lock()
	first := g.rec.Match.Turn
	g.rec.Match.TurnStart = time.Now().Add(-time.Minute + 50*time.Millisecond)
	h.srv.arm(g)
	g.mu.Unlock()
	s := h.dial(c.ID, "token="+c.Token).until(func(m msg) bool { return m.T == "state" && m.State.Turn != first }).State
	if k := s.Actions[0].Kind; k != game.ActTimeout {
		t.Fatalf("journal : %v", s.Actions)
	}
}

func TestStaticAndHealth(t *testing.T) {
	dir := t.TempDir()
	_ = writeFile(filepath.Join(dir, "index.html"), "<html>coda</html>")
	store, _ := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	defer store.Close()
	srv, _ := New(Config{Store: store, StaticDir: dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	for path, want := range map[string]int{"/": 200, "/g/abc": 200, "/healthz": 200, "/api/nope": 404, "/api/games/nope": 404} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("%s : %d", path, res.StatusCode)
		}
	}
}

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o644) }

func TestGuestPlayOff(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	srv, err := New(Config{Store: store, Auth: testAuth{guests: false}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, srv: srv, http: httptest.NewServer(srv.Handler()), store: store}
	t.Cleanup(h.http.Close)

	var cfg struct{ Auth auth.Info }
	if h.get("/api/config", "", &cfg); cfg.Auth.GuestPlay {
		t.Fatalf("config %+v", cfg)
	}
	body := map[string]any{}
	_ = json.Unmarshal([]byte(`{"name": "Invité", "seats": 2}`), &body)
	if st := h.post("/api/games", "", body, nil); st != http.StatusUnauthorized {
		t.Fatalf("création en invité : %d", st)
	}
	var c struct{ ID string }
	if st := h.post("/api/games", "user:alice", body, &c); st != http.StatusCreated {
		t.Fatalf("création avec un compte : %d", st)
	}
	if st := h.post("/api/games/"+c.ID+"/join", "", body, nil); st != http.StatusUnauthorized {
		t.Fatalf("arrivée en invité : %d", st)
	}
	if st := h.post("/api/games/"+c.ID+"/join", "user:bob", body, nil); st != http.StatusOK {
		t.Fatalf("arrivée avec un compte : %d", st)
	}
}
