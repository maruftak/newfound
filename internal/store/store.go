// Package store persists timestamped attack-surface snapshots in SQLite
// (pure-Go modernc driver, so the binary cross-compiles without cgo).
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/maruftak/reconsentry/internal/model"
)

// Store is a snapshot database handle.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	scope TEXT NOT NULL,
	started_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS assets (
	run_id INTEGER NOT NULL,
	scope  TEXT NOT NULL,
	target TEXT NOT NULL,
	host   TEXT NOT NULL,
	ip     TEXT,
	alive  INTEGER NOT NULL DEFAULT 0,
	status INTEGER NOT NULL DEFAULT 0,
	tech   TEXT,
	title  TEXT,
	server TEXT
);
CREATE INDEX IF NOT EXISTS idx_assets_run ON assets(run_id);
CREATE INDEX IF NOT EXISTS idx_runs_scope ON runs(scope, id);
`

// Open opens (creating if needed) the snapshot database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// LatestAssets returns the assets from the most recent run for scope. It
// returns a nil slice (not an error) when no prior run exists.
func (s *Store) LatestAssets(scope string) ([]model.Asset, error) {
	var runID int64
	err := s.db.QueryRow(`SELECT id FROM runs WHERE scope = ? ORDER BY id DESC LIMIT 1`, scope).Scan(&runID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest run: %w", err)
	}
	return s.assetsForRun(runID)
}

func (s *Store) assetsForRun(runID int64) ([]model.Asset, error) {
	rows, err := s.db.Query(
		`SELECT target, host, ip, alive, status, tech, title, server FROM assets WHERE run_id = ?`, runID)
	if err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	defer rows.Close()

	var out []model.Asset
	for rows.Next() {
		var (
			a                       model.Asset
			ip, tech, title, server sql.NullString
			alive                   int
		)
		if err := rows.Scan(&a.Target, &a.Host, &ip, &alive, &a.Status, &tech, &title, &server); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		a.IP = ip.String
		a.Alive = alive != 0
		a.Title = title.String
		a.Server = server.String
		if tech.String != "" {
			a.Tech = strings.Split(tech.String, ", ")
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveRun persists a run and its assets atomically, returning the new run id.
func (s *Store) SaveRun(scope string, at time.Time, assets []model.Asset) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO runs(scope, started_at) VALUES(?, ?)`, scope, at)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("run id: %w", err)
	}

	stmt, err := tx.Prepare(
		`INSERT INTO assets(run_id, scope, target, host, ip, alive, status, tech, title, server)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, a := range assets {
		alive := 0
		if a.Alive {
			alive = 1
		}
		if _, err := stmt.Exec(runID, scope, a.Target, a.Host, a.IP, alive, a.Status, a.TechString(), a.Title, a.Server); err != nil {
			return 0, fmt.Errorf("insert asset %s: %w", a.Host, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return runID, nil
}
