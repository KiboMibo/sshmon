package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/history"
)

func TestWriteVersionPrintsNameAndVersion(t *testing.T) {
	// Given: a version string and an output buffer.
	var out bytes.Buffer

	// When: the version line is written.
	writeVersion(&out, "1.2.3")

	// Then: output names sshmon with the exact version and a trailing newline.
	if got := out.String(); got != "sshmon 1.2.3\n" {
		t.Fatalf("version output = %q", got)
	}
}

func TestOpenHistoryFailsSoftWhenDatabaseCannotOpen(t *testing.T) {
	// Given: history points beneath a regular file, so SQLite cannot create its directory.
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	// When: optional history is opened.
	service := openHistory(config.History{Path: filepath.Join(parent, "history.db")}, &stderr)

	// Then: startup may continue without a service and one user-facing warning is emitted.
	if service != nil {
		t.Fatal("history service must be nil after open failure")
	}
	if got := stderr.String(); strings.Count(got, "история недоступна") != 1 {
		t.Fatalf("warning = %q", got)
	}
}

func TestOpenHistoryDisabledIsSilent(t *testing.T) {
	// Given: history is explicitly disabled.
	disabled := false
	var stderr bytes.Buffer

	// When: optional history is opened.
	service := openHistory(config.History{Enabled: &disabled}, &stderr)

	// Then: no service and no warning are produced.
	if service != nil || stderr.Len() != 0 {
		t.Fatalf("service = %v, stderr = %q", service, stderr.String())
	}
}

func TestEnabledHistoryPersistsCollectorSnapshot(t *testing.T) {
	// Given: enabled history and a collector snapshot for one server.
	service := openHistory(config.History{Path: filepath.Join(t.TempDir(), "history.db")}, &bytes.Buffer{})
	if service == nil {
		t.Fatal("history service was not opened")
	}
	defer func() {
		if err := service.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	collector := collect.New(&config.Config{Servers: []config.Server{{Name: "web", Host: "web.example", User: "deploy", Port: 22}}})
	at := time.Now()
	snapshot := collect.Snapshot{Time: at, Servers: []collect.Metrics{{Name: "web", Online: true, CPUPct: 42}}}

	// When: the runtime history sink receives the snapshot.
	if err := collector.HistorySink(service)(context.Background(), snapshot); err != nil {
		t.Fatalf("HistorySink: %v", err)
	}

	// Then: the point is queryable after the write returns.
	points, err := service.Query(context.Background(), "deploy@web.example:22", history.Range1H)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(points) != 1 || points[0].CPU == nil || *points[0].CPU != 42 {
		t.Fatalf("points = %#v", points)
	}
}
