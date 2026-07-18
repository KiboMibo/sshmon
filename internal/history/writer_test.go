package history

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestWritePersistsOnlineAndOfflineSamples(t *testing.T) {
	// Given a history service and two samples with different availability states.
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	service := NewService(store, Options{Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = service.Close() })
	cpu := 42.5
	mem := 61.0

	// When online and offline samples are written through the bounded writer.
	if err := service.Write(context.Background(), Sample{
		ServerKey: "root@web:22", At: now.Add(-2 * time.Minute), Online: true,
		CPU: &cpu, Memory: &mem, Issues: []string{"CPU high"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Write(context.Background(), Sample{
		ServerKey: "root@web:22", At: now.Add(-time.Minute), Online: false,
		Issues: []string{"offline"},
	}); err != nil {
		t.Fatal(err)
	}

	// Then both rows exist and the unavailable sample keeps numeric metrics NULL.
	var rows, nullCPU int
	if err := store.db.QueryRow(`SELECT COUNT(*), SUM(cpu_pct IS NULL) FROM metric_samples`).Scan(&rows, &nullCPU); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || nullCPU != 1 {
		t.Fatalf("rows=%d null_cpu=%d, want 2 and 1", rows, nullCPU)
	}
}

func TestWriteReturnsErrClosedAfterServiceClose(t *testing.T) {
	// Given a closed history service.
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, Options{})
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}

	// When another sample is submitted.
	err = service.Write(context.Background(), Sample{ServerKey: "root@web:22", At: time.Now()})

	// Then callers receive the typed closed-service error.
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Write error = %v, want ErrClosed", err)
	}
}

func TestWriteForcesOfflineMetricsToNull(t *testing.T) {
	// Given an offline sample that still carries stale numeric values.
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, Options{})
	t.Cleanup(func() { _ = service.Close() })
	stale := 99.0

	// When the sample crosses the history boundary.
	if err := service.Write(context.Background(), Sample{
		ServerKey: "root@web:22", At: time.Now(), Online: false,
		CPU: &stale, Memory: &stale, Disk: &stale, NetRX: &stale, NetTX: &stale, Load1: &stale,
	}); err != nil {
		t.Fatal(err)
	}

	// Then every numeric metric is persisted as NULL to create a graph gap.
	var nullMetrics int
	if err := store.db.QueryRow(`SELECT
		(cpu_pct IS NULL) + (mem_pct IS NULL) + (disk_pct IS NULL) +
		(net_rx_bps IS NULL) + (net_tx_bps IS NULL) + (load1 IS NULL)
		FROM metric_samples`).Scan(&nullMetrics); err != nil {
		t.Fatal(err)
	}
	if nullMetrics != 6 {
		t.Fatalf("NULL metric count = %d, want 6", nullMetrics)
	}
}
