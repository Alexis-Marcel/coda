package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alexis-marcel/coda/internal/game"
	_ "modernc.org/sqlite"
)

// Une partie est enregistrée en entier (JSON) à chaque coup : c'est petit, et
// une partie en cours survit à un redémarrage.

type record struct {
	ID       string      `json:"id"`
	Match    *game.Match `json:"match"`
	Tokens   []string    `json:"tokens"` // SHA-256 des jetons de place
	Public   bool        `json:"public"`
	Rematch  []bool      `json:"rematch"`
	Next     string      `json:"next,omitempty"`
	Canceled bool        `json:"canceled,omitempty"`
	Created  time.Time   `json:"created"`
	// PausedAt est posé à l'arrêt du serveur : au redémarrage, le tour en
	// cours reprend là où il était.
	PausedAt time.Time `json:"pausedAt,omitzero"`
}

type Store struct{ db *sql.DB }

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS games (
	id         TEXT PRIMARY KEY,
	data       TEXT NOT NULL,
	phase      TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS games_phase ON games(phase);
CREATE TABLE IF NOT EXISTS seats (
	game_id TEXT NOT NULL REFERENCES games(id),
	seat    INTEGER NOT NULL,
	user_id TEXT NOT NULL,
	PRIMARY KEY (game_id, seat)
);
CREATE INDEX IF NOT EXISTS seats_user ON seats(user_id);`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("schéma : %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) save(rec *record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	phase := string(rec.Match.Phase)
	if rec.Canceled {
		phase = "canceled"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO games (id, data, phase, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, phase = excluded.phase, updated_at = excluded.updated_at`,
		rec.ID, data, phase, rec.Created.UnixMilli(), time.Now().UnixMilli())
	if err != nil {
		return err
	}
	for i, p := range rec.Match.Players {
		if p.UserID == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO seats (game_id, seat, user_id) VALUES (?, ?, ?)`, rec.ID, i, p.UserID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanRecords(rows *sql.Rows) ([]*record, error) {
	defer rows.Close()
	var out []*record
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		rec := &record{}
		if err := json.Unmarshal(data, rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) load(id string) (*record, error) {
	rows, err := s.db.Query(`SELECT data FROM games WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	recs, err := scanRecords(rows)
	if err != nil || len(recs) == 0 {
		return nil, err
	}
	return recs[0], nil
}

// unfinished renvoie les parties à reprendre au démarrage.
func (s *Store) unfinished() ([]*record, error) {
	rows, err := s.db.Query(`SELECT data FROM games WHERE phase IN ('waiting', 'playing')`)
	if err != nil {
		return nil, err
	}
	return scanRecords(rows)
}

// history renvoie les dernières parties terminées d'un compte.
func (s *Store) history(userID string, limit int) ([]*record, error) {
	rows, err := s.db.Query(`SELECT g.data FROM games g JOIN seats s ON s.game_id = g.id
		WHERE g.phase = 'finished' AND s.user_id = ? ORDER BY g.updated_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanRecords(rows)
}

type stats struct {
	Played int `json:"played"`
	Won    int `json:"won"`
}

func (s *Store) stats(userID string) (stats, error) {
	var st stats
	err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(json_extract(g.data, '$.match.result.winner') = s.seat), 0)
		FROM games g JOIN seats s ON s.game_id = g.id WHERE g.phase = 'finished' AND s.user_id = ?`, userID).Scan(&st.Played, &st.Won)
	return st, err
}
