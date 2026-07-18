package history

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/config"
)

func TestMaintainDeletesExpiredRawAndRollupRows(t *testing.T) {
	// Given recent and expired rows in both history tables.
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	service := newTestService(t, now)
	for _, at := range []time.Time{now.Add(-25 * time.Hour), now.Add(-time.Hour)} {
		if _, err := service.store.db.Exec(`INSERT INTO metric_samples
			(server_key, sampled_at_ms, online, issues_json) VALUES (?, ?, 0, '[]')`,
			"root@web:22", at.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	for _, at := range []time.Time{now.Add(-721 * time.Hour), now.Add(-48 * time.Hour)} {
		if _, err := service.store.db.Exec(`INSERT INTO metric_rollups_minute
			(server_key, bucket_at_ms, online_count, sample_count) VALUES (?, ?, 0, 1)`,
			"root@web:22", at.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}

	// When retention maintenance runs with default windows.
	if err := service.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}

	// Then expired rows are gone and completed raw minutes were retained as rollups.
	var raw, rollups int
	if err := service.store.db.QueryRow(`SELECT COUNT(*) FROM metric_samples`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := service.store.db.QueryRow(`SELECT COUNT(*) FROM metric_rollups_minute`).Scan(&rollups); err != nil {
		t.Fatal(err)
	}
	if raw != 1 || rollups != 3 {
		t.Fatalf("raw=%d rollups=%d, want 1 and 3", raw, rollups)
	}
}

func TestOpenServiceIsFailSoftWhenDisabledOrStorageFails(t *testing.T) {
	// Given disabled history and an invalid database parent path.
	disabled := false
	service, err := OpenService(config.History{Enabled: &disabled})
	if err != nil || service != nil {
		t.Fatalf("disabled OpenService = (%v, %v), want (nil, nil)", service, err)
	}
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When opening history below that file.
	service, err = OpenService(config.History{Path: filepath.Join(parentFile, "history.db")})

	// Then initialization reports the error and leaves callers a nil service to ignore.
	if err == nil || service != nil {
		t.Fatalf("failed OpenService = (%v, %v), want (nil, error)", service, err)
	}
}
