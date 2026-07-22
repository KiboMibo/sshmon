package tui

import (
	"testing"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestSortProcessesUsesStableTieBreakers(t *testing.T) {
	// Given processes with equal CPU and different PIDs.
	items := []collect.Process{{PID: 3, Command: "worker", CPUPct: 20}, {PID: 1, Command: "api", CPUPct: 20}, {PID: 2, Command: "db", CPUPct: 10}}

	// When sorted by CPU descending.
	got := sortProcesses(items, processSortCPU)

	// Then equal values are ordered by PID and the input is not mutated.
	if got[0].PID != 1 || got[1].PID != 3 || got[2].PID != 2 {
		t.Fatalf("unexpected order: %+v", got)
	}
	if items[0].PID != 3 {
		t.Fatal("sort mutated input")
	}
}

func TestRenderProcessesShowsReadOnlyColumns(t *testing.T) {
	// Given a ready process screen.
	m := Model{screen: screenProcesses, snapshot: snapshotWithServers("web"), layout: newLayout(100, 24)}
	m.processes.status = diagnosticsReady
	m.processes.items = []collect.Process{{PID: 42, Command: "nginx -g daemon off", CPUPct: 7.5, MemPct: 2.5}}

	// When rendered.
	view := m.processes.view(m.screenContext())

	// Then PID, command, CPU and memory are visible without mutation controls.
	for _, want := range []string{"PID", "КОМАНДА", "CPU", "MEM", "42", "nginx"} {
		if !containsText(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
