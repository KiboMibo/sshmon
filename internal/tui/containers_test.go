package tui

import (
	"testing"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestSortContainersUsesNameAsStableTieBreaker(t *testing.T) {
	// Given containers with equal CPU usage.
	items := []collect.Container{{Name: "worker", Image: "app:v1", Status: "Up", CPUPct: 10}, {Name: "api", Image: "app:v2", Status: "Up", CPUPct: 10}}

	// When sorted by CPU descending.
	got := sortContainers(items, containerSortCPU)

	// Then the name provides deterministic ordering for equal values.
	if got[0].Name != "api" || got[1].Name != "worker" {
		t.Fatalf("unexpected order: %+v", got)
	}
}

func TestRenderContainersIsObservationOnly(t *testing.T) {
	// Given a ready container screen.
	m := Model{screen: screenContainers, snapshot: snapshotWithServers("web"), layout: newLayout(100, 24)}
	m.containers.status = diagnosticsReady
	m.containers.items = []collect.Container{{Name: "api", Image: "app:v2", Status: "Up 2h", CPUPct: 4, MemPct: 12}}

	// When rendered.
	view := m.renderContainers()

	// Then state and resource columns are present and no mutation hint exists.
	for _, want := range []string{"ИМЯ", "ОБРАЗ", "СТАТУС", "CPU", "MEM", "api", "Up 2h"} {
		if !containsText(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	for _, forbidden := range []string{"start", "stop", "restart", "exec"} {
		if containsText(view, forbidden) {
			t.Fatalf("read-only view contains %q", forbidden)
		}
	}
}
