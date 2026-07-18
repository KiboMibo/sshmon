package history

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesSQLiteSchemaAndWriterPragmas(t *testing.T) {
	// Given: a history database path whose parent does not exist.
	path := filepath.Join(t.TempDir(), "nested", "history.db")

	// When: the history store is opened.
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Then: both schema tables and writer-oriented SQLite settings exist.
	for _, table := range []string{"metric_samples", "metric_rollups_minute"} {
		var name string
		if err := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
	}
	var journalMode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode=%q", journalMode)
	}
	var busyTimeout int
	if err := store.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout=%d", busyTimeout)
	}
	if got := store.db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections=%d", got)
	}
}

func TestStoreCloseReleasesDatabase(t *testing.T) {
	// Given: an open history store.
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// When: the store is closed.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Then: subsequent database work fails.
	if err := store.db.Ping(); err == nil {
		t.Fatal("Ping succeeded after Close")
	}
}
