package game

import (
	"errors"
	"math/rand/v2"
	"slices"
	"time"
)

// Une partie de 2 à 4 joueurs. Chacun a une rangée de tuiles cachées, rangées
// par valeur ; seule leur couleur se voit. À son tour, on pioche une tuile (de
// la couleur de son choix), puis on désigne une tuile cachée d'un adversaire en
// annonçant sa valeur. Juste : elle est retournée, et l'on peut continuer ou
// s'arrêter (la tuile piochée rejoint alors sa rangée, cachée). Fausse : la
// tuile piochée rejoint sa rangée, retournée. Qui n'a plus de tuile cachée est
// éliminé ; le dernier en lice gagne.

type Phase string

const (
	Waiting  Phase = "waiting"  // en attente des joueurs
	Playing  Phase = "playing"  // en cours
	Finished Phase = "finished" // terminée
)

// Step est ce qu'attend le joueur dont c'est le tour.
type Step string

const (
	StepDraw   Step = "draw"   // piocher une tuile
	StepGuess  Step = "guess"  // faire une première annonce
	StepChoose Step = "choose" // annonce juste : en faire une autre ou s'arrêter
	StepReveal Step = "reveal" // annonce fausse sans tuile piochée : retourner une de ses tuiles
)

type Config struct {
	Seats    int           `json:"seats"`    // nombre de joueurs attendus (2 à 4)
	TurnTime time.Duration `json:"turnTime"` // temps par tour, 0 : illimité
}

var DefaultConfig = Config{Seats: 2, TurnTime: time.Minute}

const MaxSeats = 4

type Player struct {
	Name   string `json:"name"`
	UserID string `json:"userId,omitempty"`
	Row    []Tile `json:"row"`
	Out    bool   `json:"out,omitempty"`
	// Rank : place finale, 1 pour le gagnant ; 0 tant que le joueur est en lice.
	Rank int `json:"rank,omitempty"`
}

type ActionKind string

const (
	ActDraw    ActionKind = "draw"    // a pioché une tuile de couleur Color
	ActGuess   ActionKind = "guess"   // a annoncé Num sur la tuile Tile de Target
	ActStop    ActionKind = "stop"    // s'est arrêté après une annonce juste
	ActReveal  ActionKind = "reveal"  // a retourné sa tuile Tile (annonce fausse, pioche vide)
	ActTimeout ActionKind = "timeout" // temps écoulé : le serveur a joué à sa place
	ActOut     ActionKind = "out"     // n'a plus de tuile cachée
	ActResign  ActionKind = "resign"  // a abandonné
)

type Action struct {
	Seat    int        `json:"seat"`
	Kind    ActionKind `json:"kind"`
	Color   Color      `json:"color"`
	Target  int        `json:"target"`
	Tile    int        `json:"tile"`
	Num     int        `json:"num"`
	Correct bool       `json:"correct,omitempty"`
	// Penalty : tuile piochée, retournée par cette annonce fausse.
	Penalty *int      `json:"penalty,omitempty"`
	At      time.Time `json:"at"`
}

type Reason string

const (
	ByLastStanding Reason = "last"   // les autres sont éliminés
	ByResign       Reason = "resign" // les autres ont abandonné
)

type Result struct {
	Winner int    `json:"winner"`
	Reason Reason `json:"reason"`
}

type Match struct {
	Config  Config    `json:"config"`
	Phase   Phase     `json:"phase"`
	Players []*Player `json:"players"`
	// Pile : la pioche, déjà mélangée ; on y prend la première tuile de la couleur demandée.
	Pile      []Tile    `json:"pile"`
	First     int       `json:"first"`
	Turn      int       `json:"turn"`
	Step      Step      `json:"step"`
	Drawn     *Tile     `json:"drawn,omitempty"`
	Actions   []Action  `json:"actions"`
	Result    *Result   `json:"result,omitempty"`
	Started   time.Time `json:"started,omitzero"`
	TurnStart time.Time `json:"turnStart,omitzero"`
}

var (
	ErrFull        = errors.New("la partie est complète")
	ErrPhase       = errors.New("impossible à ce moment de la partie")
	ErrFewPlayers  = errors.New("il faut au moins deux joueurs")
	ErrBadSeat     = errors.New("place inconnue")
	ErrNotYourTurn = errors.New("ce n'est pas ton tour")
	ErrStep        = errors.New("pas maintenant")
	ErrEmptyColor  = errors.New("plus de tuile de cette couleur dans la pioche")
	ErrBadTarget   = errors.New("ce joueur n'est pas un adversaire en lice")
	ErrBadTile     = errors.New("cette tuile n'est pas une tuile cachée de ce joueur")
	ErrBadNum      = errors.New("une tuile vaut de 0 à 11")
	ErrOver        = errors.New("la partie est terminée")
)

func NewMatch(cfg Config) *Match {
	cfg.Seats = min(max(cfg.Seats, 2), MaxSeats)
	return &Match{Config: cfg, Phase: Waiting, Players: []*Player{}}
}

// Sit installe un joueur à la première place libre et renvoie cette place.
func (m *Match) Sit(name, userID string) (int, error) {
	if m.Phase != Waiting {
		return 0, ErrPhase
	}
	if len(m.Players) >= m.Config.Seats {
		return 0, ErrFull
	}
	m.Players = append(m.Players, &Player{Name: name, UserID: userID})
	return len(m.Players) - 1, nil
}

// Full dit si toutes les places sont prises.
func (m *Match) Full() bool { return len(m.Players) >= m.Config.Seats }

// Start distribue les tuiles (4 chacun, 3 à quatre joueurs) et donne la main à first.
func (m *Match) Start(rng *rand.Rand, first int, now time.Time) error {
	if m.Phase != Waiting {
		return ErrPhase
	}
	if len(m.Players) < 2 {
		return ErrFewPlayers
	}
	m.Config.Seats = len(m.Players)
	hand := 4
	if len(m.Players) == 4 {
		hand = 3
	}
	pile := newSet(rng)
	for _, p := range m.Players {
		p.Row = nil
		for _, t := range pile[:hand] {
			p.Row = insert(p.Row, t)
		}
		pile = pile[hand:]
	}
	m.Pile = pile
	m.Phase = Playing
	m.Started = now
	m.First = first % len(m.Players)
	m.startTurn(m.First, now)
	return nil
}

func (m *Match) startTurn(seat int, now time.Time) {
	m.Turn, m.TurnStart, m.Drawn = seat, now, nil
	m.Step = StepGuess
	if len(m.Pile) > 0 {
		m.Step = StepDraw
	}
}

// Active dit si un joueur est encore en lice.
func (m *Match) Active(seat int) bool {
	return seat >= 0 && seat < len(m.Players) && !m.Players[seat].Out
}

func (m *Match) activeCount() int {
	n := 0
	for i := range m.Players {
		if m.Active(i) {
			n++
		}
	}
	return n
}

// Deadline est l'instant où le tour en cours se joue tout seul, zéro sans limite.
func (m *Match) Deadline() time.Time {
	if m.Phase != Playing || m.Config.TurnTime <= 0 {
		return time.Time{}
	}
	return m.TurnStart.Add(m.Config.TurnTime)
}

// Left donne le temps qu'il reste au tour en cours (0 sans limite).
func (m *Match) Left(now time.Time) time.Duration {
	d := m.Deadline()
	if d.IsZero() {
		return 0
	}
	return max(d.Sub(now), 0)
}

// Tick joue à la place d'un joueur dont le temps est écoulé : il pioche s'il
// ne l'a pas fait, et une annonce manquante compte comme fausse ; après une
// annonce juste, il s'arrête. Renvoie vrai si la partie a changé.
func (m *Match) Tick(now time.Time) bool {
	d := m.Deadline()
	if d.IsZero() || now.Before(d) {
		return false
	}
	seat := m.Turn
	m.log(Action{Seat: seat, Kind: ActTimeout}, now)
	if m.Step == StepDraw {
		c := Black
		if m.pileCount(White) > m.pileCount(Black) {
			c = White
		}
		_ = m.draw(seat, c, now)
	}
	switch m.Step {
	case StepGuess:
		m.miss(seat, now)
	case StepChoose:
		_ = m.Stop(seat, now)
	}
	if m.Phase == Playing && m.Turn == seat && m.Step == StepReveal {
		for _, t := range m.Players[seat].Row {
			if !t.Revealed {
				_ = m.Reveal(seat, t.ID, now)
				break
			}
		}
	}
	return true
}

func (m *Match) log(a Action, now time.Time) {
	a.At = now
	m.Actions = append(m.Actions, a)
}

func (m *Match) pileCount(c Color) int {
	n := 0
	for _, t := range m.Pile {
		if t.Color == c {
			n++
		}
	}
	return n
}

// PileCount donne le nombre de tuiles noires et blanches dans la pioche.
func (m *Match) PileCount() [2]int { return [2]int{m.pileCount(Black), m.pileCount(White)} }

func (m *Match) check(seat int, steps ...Step) error {
	if m.Phase != Playing {
		if m.Phase == Finished {
			return ErrOver
		}
		return ErrPhase
	}
	if seat < 0 || seat >= len(m.Players) {
		return ErrBadSeat
	}
	if seat != m.Turn {
		return ErrNotYourTurn
	}
	if !slices.Contains(steps, m.Step) {
		return ErrStep
	}
	return nil
}

// Draw pioche une tuile de la couleur demandée.
func (m *Match) Draw(seat int, c Color, now time.Time) error {
	if err := m.check(seat, StepDraw); err != nil {
		return err
	}
	return m.draw(seat, c, now)
}

func (m *Match) draw(seat int, c Color, now time.Time) error {
	i := slices.IndexFunc(m.Pile, func(t Tile) bool { return t.Color == c })
	if i < 0 {
		return ErrEmptyColor
	}
	t := m.Pile[i]
	m.Pile = slices.Delete(m.Pile, i, i+1)
	m.Drawn = &t
	m.Step = StepGuess
	m.log(Action{Seat: seat, Kind: ActDraw, Color: c}, now)
	return nil
}

// Guess annonce la valeur num pour la tuile tile du joueur target. Renvoie si
// l'annonce était juste.
func (m *Match) Guess(seat, target, tile, num int, now time.Time) (bool, error) {
	if err := m.check(seat, StepGuess, StepChoose); err != nil {
		return false, err
	}
	if target == seat || !m.Active(target) {
		return false, ErrBadTarget
	}
	row := m.Players[target].Row
	i := slices.IndexFunc(row, func(t Tile) bool { return t.ID == tile && !t.Revealed })
	if i < 0 {
		return false, ErrBadTile
	}
	if num < 0 || num > MaxNum {
		return false, ErrBadNum
	}
	correct := row[i].Num == num
	m.log(Action{Seat: seat, Kind: ActGuess, Target: target, Tile: tile, Num: num, Correct: correct}, now)
	if !correct {
		m.miss(seat, now)
		return false, nil
	}
	row[i].Revealed = true
	if hidden(row) == 0 {
		m.eliminate(target, now)
		if m.Phase == Finished {
			return true, nil
		}
	}
	m.Step = StepChoose
	return true, nil
}

// miss applique une annonce fausse : la tuile piochée rejoint la rangée,
// retournée ; sans tuile piochée, le joueur doit retourner une des siennes.
func (m *Match) miss(seat int, now time.Time) {
	p := m.Players[seat]
	if m.Drawn == nil {
		m.Step = StepReveal
		return
	}
	t := *m.Drawn
	t.Revealed = true
	p.Row = insert(p.Row, t)
	m.Drawn = nil
	if a := m.lastGuess(seat); a != nil {
		id := t.ID
		a.Penalty = &id
	}
	m.endTurn(now)
}

func (m *Match) lastGuess(seat int) *Action {
	for i := len(m.Actions) - 1; i >= 0; i-- {
		a := &m.Actions[i]
		if a.Seat != seat {
			return nil
		}
		if a.Kind == ActGuess {
			return a
		}
	}
	return nil
}

// Stop termine le tour après une annonce juste : la tuile piochée rejoint la
// rangée, cachée.
func (m *Match) Stop(seat int, now time.Time) error {
	if err := m.check(seat, StepChoose); err != nil {
		return err
	}
	if m.Drawn != nil {
		m.Players[seat].Row = insert(m.Players[seat].Row, *m.Drawn)
		m.Drawn = nil
	}
	m.log(Action{Seat: seat, Kind: ActStop}, now)
	m.endTurn(now)
	return nil
}

// Reveal retourne une de ses tuiles cachées, après une annonce fausse quand la
// pioche est vide.
func (m *Match) Reveal(seat, tile int, now time.Time) error {
	if err := m.check(seat, StepReveal); err != nil {
		return err
	}
	row := m.Players[seat].Row
	i := slices.IndexFunc(row, func(t Tile) bool { return t.ID == tile && !t.Revealed })
	if i < 0 {
		return ErrBadTile
	}
	row[i].Revealed = true
	m.log(Action{Seat: seat, Kind: ActReveal, Tile: tile}, now)
	m.endTurn(now)
	return nil
}

// endTurn passe la main au joueur suivant encore en lice, après avoir éliminé
// le joueur qui vient de retourner sa dernière tuile.
func (m *Match) endTurn(now time.Time) {
	if seat := m.Turn; hidden(m.Players[seat].Row) == 0 && !m.Players[seat].Out {
		m.eliminate(seat, now)
	}
	if m.Phase != Playing {
		return
	}
	next := m.Turn
	for {
		next = (next + 1) % len(m.Players)
		if m.Active(next) {
			break
		}
	}
	m.startTurn(next, now)
}

// eliminate sort un joueur ; s'il n'en reste qu'un, il gagne.
func (m *Match) eliminate(seat int, now time.Time) {
	p := m.Players[seat]
	p.Rank = m.activeCount()
	p.Out = true
	m.log(Action{Seat: seat, Kind: ActOut}, now)
	if m.activeCount() == 1 {
		m.finishLast(ByLastStanding, now)
	}
}

func (m *Match) finishLast(reason Reason, now time.Time) {
	for i, p := range m.Players {
		if !p.Out {
			p.Rank = 1
			m.Result = &Result{Winner: i, Reason: reason}
		}
	}
	if m.Drawn != nil {
		m.Pile = append(m.Pile, *m.Drawn)
		m.Drawn = nil
	}
	m.Phase = Finished
	m.TurnStart = now
}

// Resign fait abandonner un joueur, à tout moment de la partie : il est
// éliminé, et si c'était son tour, la main passe.
func (m *Match) Resign(seat int, now time.Time) error {
	if m.Phase != Playing {
		return ErrPhase
	}
	if !m.Active(seat) {
		return ErrBadSeat
	}
	p := m.Players[seat]
	m.log(Action{Seat: seat, Kind: ActResign}, now)
	p.Rank = m.activeCount()
	p.Out = true
	if m.activeCount() == 1 {
		m.finishLast(ByResign, now)
		return nil
	}
	if m.Turn == seat {
		if m.Drawn != nil {
			// La tuile piochée retourne dans la pioche.
			m.Pile = append(m.Pile, *m.Drawn)
			m.Drawn = nil
		}
		m.endTurn(now)
	}
	return nil
}
