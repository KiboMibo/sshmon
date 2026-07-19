package collect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/history"
)

type recordingHistoryWriter struct {
	samples []history.Sample
	err     error
}

func (w *recordingHistoryWriter) Write(_ context.Context, sample history.Sample) error {
	w.samples = append(w.samples, sample)
	return w.err
}

func TestHistorySamplesMapOnlineAndOfflineMetrics(t *testing.T) {
	// Given: configured server identities and a mixed online/offline snapshot.
	at := time.Unix(1_700_000_000, 0)
	collector := &Collector{cfg: &config.Config{Servers: []config.Server{
		{Name: "web", Host: "WEB.EXAMPLE", User: "Deploy", Port: 0},
		{Name: "db", Host: "2001:db8::2", User: "root", Port: 2222},
	}}}
	snapshot := Snapshot{
		Time: at,
		Servers: []Metrics{
			{Name: "web", Online: true, CPUPct: 12, MemPct: 34, Load1: 0.5,
				Disks: []DiskUsage{{UsedPct: 45}}, Net: []NetRate{{RxBps: 10, TxBps: 20}}},
			{Name: "db", Online: false, CPUPct: 99, MemPct: 98, Load1: 10},
		},
		Issues: []Issue{
			{Server: "web", Severity: "warn", Msg: "CPU high"},
			{Server: "db", Severity: "crit", Msg: "offline"},
		},
	}

	// When: the snapshot is converted for history storage.
	samples := collector.historySamples(snapshot)

	// Then: identities, time, metrics, gaps, and per-server issues are preserved.
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(samples))
	}
	web := samples[0]
	if web.ServerKey != "Deploy@web.example:22" || !web.At.Equal(at) || !web.Online {
		t.Fatalf("online sample identity = %#v", web)
	}
	if web.CPU == nil || *web.CPU != 12 || web.Memory == nil || *web.Memory != 34 ||
		web.Disk == nil || *web.Disk != 45 || web.NetRX == nil || *web.NetRX != 10 ||
		web.NetTX == nil || *web.NetTX != 20 || web.Load1 == nil || *web.Load1 != 0.5 {
		t.Fatalf("online sample metrics = %#v", web)
	}
	if len(web.Issues) != 1 || web.Issues[0] != "[warn] CPU high" {
		t.Fatalf("online issues = %#v", web.Issues)
	}
	db := samples[1]
	if db.ServerKey != "root@[2001:db8::2]:2222" || db.Online {
		t.Fatalf("offline sample identity = %#v", db)
	}
	if db.CPU != nil || db.Memory != nil || db.Disk != nil || db.NetRX != nil || db.NetTX != nil || db.Load1 != nil {
		t.Fatalf("offline sample must contain metric gaps: %#v", db)
	}
	if len(db.Issues) != 1 || db.Issues[0] != "[crit] offline" {
		t.Fatalf("offline issues = %#v", db.Issues)
	}
}

func TestHistorySinkWritesMappedSamplesAndReturnsWriterFailure(t *testing.T) {
	// Given: a collector snapshot and a history writer that reports a storage failure.
	wantErr := errors.New("disk full")
	writer := &recordingHistoryWriter{err: wantErr}
	collector := &Collector{cfg: &config.Config{Servers: []config.Server{
		{Name: "web", Host: "web.example", User: "deploy", Port: 22},
		{Name: "db", Host: "db.example", User: "deploy", Port: 22},
	}}}
	snapshot := Snapshot{Time: time.Unix(1_700_000_000, 0), Servers: []Metrics{
		{Name: "web", Online: true, CPUPct: 12},
		{Name: "db", Online: false},
	}}

	// When: the collector-created sink writes the snapshot.
	err := collector.HistorySink(writer)(context.Background(), snapshot)

	// Then: writing stops at the first failure and returns it unchanged.
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(writer.samples) != 1 || writer.samples[0].ServerKey != "deploy@web.example:22" {
		t.Fatalf("samples = %#v", writer.samples)
	}
}
