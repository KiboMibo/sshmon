package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initialize() error {
	statements := [...]string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		`CREATE TABLE IF NOT EXISTS metric_samples (
			server_key TEXT NOT NULL,
			sampled_at_ms INTEGER NOT NULL,
			online INTEGER NOT NULL,
			cpu_pct REAL,
			mem_pct REAL,
			disk_pct REAL,
			net_rx_bps REAL,
			net_tx_bps REAL,
			load1 REAL,
			issues_json TEXT NOT NULL,
			PRIMARY KEY(server_key, sampled_at_ms)
		)`,
		`CREATE TABLE IF NOT EXISTS metric_rollups_minute (
			server_key TEXT NOT NULL,
			bucket_at_ms INTEGER NOT NULL,
			online_count INTEGER NOT NULL,
			sample_count INTEGER NOT NULL,
			cpu_min REAL,
			cpu_max REAL,
			cpu_avg REAL,
			mem_min REAL,
			mem_max REAL,
			mem_avg REAL,
			disk_min REAL,
			disk_max REAL,
			disk_avg REAL,
			net_rx_avg REAL,
			net_tx_avg REAL,
			load1_avg REAL,
			PRIMARY KEY(server_key, bucket_at_ms)
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize history database: %w", err)
		}
	}
	return nil
}
