package game

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func newGame(t *testing.T, seats int, seed uint64) *Match {
	t.Helper()
	m := NewMatch(Config{Seats: seats, TurnTime: time.Minute})
	for i := range seats {
		if _, err := m.Sit(string(rune('A'+i)), ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Start(rand.New(rand.NewPCG(seed, 1)), 0, t0); err != nil {
		t.Fatal(err)
	}
	return m
}

// hiddenTile renvoie la première tuile cachée d'un joueur.
func hiddenTile(m *Match, seat int) Tile {
	for _, t := range m.Players[seat].Row {
		if !t.Revealed {
			return t
		}
	}
	panic("aucune tuile cachée")
}

func sorted(row []Tile) bool {
	return slices.IsSortedFunc(row, compare)
}

func TestSetIsComplete(t *testing.T) {
	tiles := newSet(rand.New(rand.NewPCG(1, 2)))
	seen, ids := map[[2]int]bool{}, map[int]bool{}
	for _, x := range tiles {
		seen[[2]int{int(x.Color), x.Num}] = true
		ids[x.ID] = true
	}
	if len(tiles) != 24 || len(seen) != 24 || len(ids) != 24 {
		t.Fatalf("jeu incomplet : %d tuiles, %d valeurs, %d repères", len(tiles), len(seen), len(ids))
	}
}

func TestOrder(t *testing.T) {
	if !Less(Tile{Color: Black, Num: 5}, Tile{Color: White, Num: 5}) || !Less(Tile{Color: White, Num: 4}, Tile{Color: Black, Num: 5}) {
		t.Fatal("ordre faux")
	}
}

func TestDeal(t *testing.T) {
	for seats, hand := range map[int]int{2: 4, 3: 4, 4: 3} {
		m := newGame(t, seats, 7)
		total := len(m.Pile)
		for _, p := range m.Players {
			if len(p.Row) != hand || !sorted(p.Row) {
				t.Fatalf("%d joueurs : rangée %v", seats, p.Row)
			}
			total += len(p.Row)
		}
		if total != 24 {
			t.Fatalf("%d joueurs : %d tuiles en jeu", seats, total)
		}
		if m.Step != StepDraw || m.Turn != 0 {
			t.Fatalf("premier tour : %v %d", m.Step, m.Turn)
		}
	}
}

func TestSitAndStart(t *testing.T) {
	m := NewMatch(Config{Seats: 3})
	if _, err := m.Sit("A", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(rand.New(rand.NewPCG(1, 1)), 0, t0); !errors.Is(err, ErrFewPlayers) {
		t.Fatalf("un seul joueur : %v", err)
	}
	m.Sit("B", "")
	if m.Full() {
		t.Fatal("complète à deux sur trois")
	}
	if err := m.Start(rand.New(rand.NewPCG(1, 1)), 5, t0); err != nil {
		t.Fatal(err)
	}
	if m.Config.Seats != 2 || m.Turn != 1 {
		t.Fatalf("départ à deux : %d places, tour %d", m.Config.Seats, m.Turn)
	}
	if _, err := m.Sit("C", ""); !errors.Is(err, ErrPhase) {
		t.Fatalf("arrivée après le départ : %v", err)
	}
}

func TestCorrectGuessThenStop(t *testing.T) {
	m := newGame(t, 2, 3)
	if err := m.Draw(0, White, t0); err != nil {
		t.Fatal(err)
	}
	drawn := *m.Drawn
	if drawn.Color != White {
		t.Fatal("couleur piochée")
	}
	target := hiddenTile(m, 1)
	ok, err := m.Guess(0, 1, target.ID, target.Num, t0)
	if err != nil || !ok {
		t.Fatalf("annonce juste : %v %v", ok, err)
	}
	if m.Step != StepChoose || m.Turn != 0 {
		t.Fatal("après une annonce juste, on choisit")
	}
	if err := m.Stop(0, t0); err != nil {
		t.Fatal(err)
	}
	row := m.Players[0].Row
	i := slices.IndexFunc(row, func(x Tile) bool { return x.ID == drawn.ID })
	if i < 0 || row[i].Revealed || len(row) != 5 || !sorted(row) {
		t.Fatalf("tuile piochée mal rangée : %v", row)
	}
	if m.Turn != 1 || m.Step != StepDraw {
		t.Fatal("la main passe")
	}
}

func TestWrongGuess(t *testing.T) {
	m := newGame(t, 2, 4)
	m.Draw(0, Black, t0)
	drawn := *m.Drawn
	target := hiddenTile(m, 1)
	ok, err := m.Guess(0, 1, target.ID, (target.Num+1)%12, t0)
	if err != nil || ok {
		t.Fatalf("annonce fausse : %v %v", ok, err)
	}
	row := m.Players[0].Row
	i := slices.IndexFunc(row, func(x Tile) bool { return x.ID == drawn.ID })
	if i < 0 || !row[i].Revealed {
		t.Fatal("la tuile piochée doit être retournée")
	}
	a := m.Actions[len(m.Actions)-1]
	if a.Kind != ActGuess || a.Penalty == nil || *a.Penalty != drawn.ID {
		t.Fatalf("journal : %+v", a)
	}
	if m.Turn != 1 {
		t.Fatal("la main passe")
	}
}

func TestErrors(t *testing.T) {
	m := newGame(t, 3, 5)
	if err := m.Draw(1, Black, t0); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("hors tour : %v", err)
	}
	if _, err := m.Guess(0, 1, hiddenTile(m, 1).ID, 0, t0); !errors.Is(err, ErrStep) {
		t.Fatalf("annonce avant de piocher : %v", err)
	}
	if err := m.Stop(0, t0); !errors.Is(err, ErrStep) {
		t.Fatalf("arrêt sans annonce : %v", err)
	}
	m.Draw(0, Black, t0)
	if _, err := m.Guess(0, 0, m.Players[0].Row[0].ID, 0, t0); !errors.Is(err, ErrBadTarget) {
		t.Fatalf("annonce sur soi : %v", err)
	}
	if _, err := m.Guess(0, 1, hiddenTile(m, 2).ID, 0, t0); !errors.Is(err, ErrBadTile) {
		t.Fatalf("tuile d'un autre joueur : %v", err)
	}
	if _, err := m.Guess(0, 1, hiddenTile(m, 1).ID, 12, t0); !errors.Is(err, ErrBadNum) {
		t.Fatalf("valeur 12 : %v", err)
	}
	// Une couleur épuisée ne se pioche plus.
	m.Pile = slices.DeleteFunc(m.Pile, func(x Tile) bool { return x.Color == White })
	m.Step = StepDraw
	m.Drawn = nil
	if err := m.Draw(0, White, t0); !errors.Is(err, ErrEmptyColor) {
		t.Fatalf("couleur épuisée : %v", err)
	}
}

// Avec la pioche vide, une annonce fausse oblige à retourner une de ses tuiles.
func TestEmptyPile(t *testing.T) {
	m := newGame(t, 2, 6)
	m.Pile = nil
	m.startTurn(0, t0)
	if m.Step != StepGuess {
		t.Fatal("pioche vide : on annonce tout de suite")
	}
	target := hiddenTile(m, 1)
	m.Guess(0, 1, target.ID, (target.Num+1)%12, t0)
	if m.Step != StepReveal || m.Turn != 0 {
		t.Fatal("il faut retourner une tuile")
	}
	mine := hiddenTile(m, 0)
	if err := m.Reveal(0, mine.ID, t0); err != nil {
		t.Fatal(err)
	}
	if !m.Players[0].Row[0].Revealed || m.Turn != 1 {
		t.Fatal("tuile retournée, main passée")
	}
}

// On élimine tout le monde à coups d'annonces justes : le dernier gagne.
func TestEliminationAndWin(t *testing.T) {
	m := newGame(t, 3, 8)
	m.Draw(0, Black, t0)
	for _, target := range []int{1, 2} {
		for m.Active(target) {
			x := hiddenTile(m, target)
			if ok, err := m.Guess(0, target, x.ID, x.Num, t0); !ok || err != nil {
				t.Fatal(ok, err)
			}
		}
	}
	if m.Phase != Finished || m.Result.Winner != 0 || m.Result.Reason != ByLastStanding {
		t.Fatalf("fin : %v %+v", m.Phase, m.Result)
	}
	if m.Players[1].Rank != 3 || m.Players[2].Rank != 2 || m.Players[0].Rank != 1 {
		t.Fatal("classement")
	}
	if _, err := m.Guess(0, 1, 0, 0, t0); !errors.Is(err, ErrOver) {
		t.Fatalf("après la fin : %v", err)
	}
}

// Un joueur qui retourne sa dernière tuile sur sa propre erreur est éliminé.
func TestSelfElimination(t *testing.T) {
	m := newGame(t, 3, 9)
	for i := range m.Players[0].Row {
		m.Players[0].Row[i].Revealed = true
	}
	m.Players[0].Row[0].Revealed = false
	m.Pile = nil
	m.startTurn(0, t0)
	x := hiddenTile(m, 1)
	m.Guess(0, 1, x.ID, (x.Num+1)%12, t0)
	m.Reveal(0, m.Players[0].Row[0].ID, t0)
	if !m.Players[0].Out || m.Turn != 1 || m.Phase != Playing {
		t.Fatal("éliminé par sa propre erreur, la main passe")
	}
	// Le tour suivant saute le joueur éliminé.
	x = hiddenTile(m, 2)
	m.Guess(1, 2, x.ID, (x.Num+1)%12, t0)
	m.Reveal(1, hiddenTile(m, 1).ID, t0)
	if m.Turn != 2 {
		t.Fatalf("tour : %d", m.Turn)
	}
	x = hiddenTile(m, 1)
	m.Guess(2, 1, x.ID, (x.Num+1)%12, t0)
	m.Reveal(2, hiddenTile(m, 2).ID, t0)
	if m.Turn != 1 {
		t.Fatalf("le joueur éliminé est sauté : tour %d", m.Turn)
	}
}

func TestTimeout(t *testing.T) {
	m := newGame(t, 2, 10)
	if m.Tick(t0.Add(30 * time.Second)) {
		t.Fatal("trop tôt")
	}
	before := len(m.Players[0].Row)
	if !m.Tick(t0.Add(time.Minute)) {
		t.Fatal("temps écoulé")
	}
	row := m.Players[0].Row
	if len(row) != before+1 || hidden(row) != before || m.Turn != 1 {
		t.Fatalf("temps écoulé : pioche puis tuile retournée (%v)", row)
	}
	// Après une annonce juste, le temps écoulé arrête le tour sans pénalité.
	now := t0.Add(time.Minute)
	m.Draw(1, Black, now)
	x := hiddenTile(m, 0)
	m.Guess(1, 0, x.ID, x.Num, now)
	m.Tick(now.Add(time.Minute))
	if m.Turn != 0 || hidden(m.Players[1].Row) != 5 {
		t.Fatal("arrêt automatique")
	}
	// Pioche vide et aucune annonce : une tuile retournée.
	now = now.Add(time.Minute)
	m.Pile = nil
	m.startTurn(0, now)
	h := hidden(m.Players[0].Row)
	m.Tick(now.Add(time.Minute))
	if hidden(m.Players[0].Row) != h-1 || m.Turn != 1 {
		t.Fatal("pioche vide : une tuile retournée")
	}
}

func TestResign(t *testing.T) {
	m := newGame(t, 3, 11)
	m.Draw(0, Black, t0)
	pile := len(m.Pile)
	if err := m.Resign(0, t0); err != nil {
		t.Fatal(err)
	}
	if len(m.Pile) != pile+1 || m.Turn != 1 || !m.Players[0].Out {
		t.Fatal("abandon pendant son tour")
	}
	if err := m.Resign(2, t0); err != nil {
		t.Fatal(err)
	}
	if m.Phase != Finished || m.Result.Winner != 1 || m.Result.Reason != ByResign {
		t.Fatalf("fin par abandon : %+v", m.Result)
	}
}

// Des parties entières jouées au hasard finissent toujours, rangées triées et
// 24 tuiles en jeu.
func TestRandomGames(t *testing.T) {
	for seed := range uint64(300) {
		seats := 2 + int(seed%3)
		m := newGame(t, seats, seed)
		rng := rand.New(rand.NewPCG(seed, 99))
		now := t0
		for steps := 0; m.Phase == Playing; steps++ {
			if steps > 2000 {
				t.Fatalf("graine %d : la partie ne finit pas", seed)
			}
			now = now.Add(time.Second)
			s := m.Turn
			switch m.Step {
			case StepDraw:
				c := Color(rng.IntN(2))
				if m.Draw(s, c, now) != nil {
					m.Draw(s, 1-c, now)
				}
			case StepGuess, StepChoose:
				if m.Step == StepChoose && rng.IntN(2) == 0 {
					m.Stop(s, now)
					continue
				}
				var targets []int
				for i := range m.Players {
					if i != s && m.Active(i) {
						targets = append(targets, i)
					}
				}
				tg := targets[rng.IntN(len(targets))]
				x := hiddenTile(m, tg)
				num := rng.IntN(12)
				if rng.IntN(3) == 0 {
					num = x.Num
				}
				if _, err := m.Guess(s, tg, x.ID, num, now); err != nil {
					t.Fatalf("graine %d : %v", seed, err)
				}
			case StepReveal:
				m.Reveal(s, hiddenTile(m, s).ID, now)
			}
			total := len(m.Pile)
			if m.Drawn != nil {
				total++
			}
			for _, p := range m.Players {
				if !sorted(p.Row) {
					t.Fatalf("graine %d : rangée en désordre", seed)
				}
				total += len(p.Row)
			}
			if total != 24 {
				t.Fatalf("graine %d : %d tuiles", seed, total)
			}
		}
		ranks := map[int]bool{}
		for _, p := range m.Players {
			ranks[p.Rank] = true
		}
		if len(ranks) != seats || !ranks[1] || !ranks[seats] {
			t.Fatalf("graine %d : classement %v", seed, ranks)
		}
	}
}
