package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestDiagnosticsLifecycleCancelsAndIgnoresStaleResponses(t *testing.T) {
	// Given an active processes request with observable cancellation.
	cancelled := false
	m := Model{screen: screenProcesses, snapshot: snapshotWithServers("web")}
	m.processes.generation = 2
	m.processes.cancel = func() { cancelled = true }
	m.processes.items = []collect.Process{{PID: 1, Command: "current"}}

	// When an old response arrives and the screen is left.
	next, _ := updateModel(t, m, processesResultMsg{generation: 1, items: []collect.Process{{PID: 2, Command: "stale"}}})
	next, _ = updateModel(t, next, key("esc"))

	// Then stale data is ignored and the active request is cancelled.
	if next.processes.items[0].Command != "current" || !cancelled || next.screen != screenDashboard {
		t.Fatalf("unexpected lifecycle state: %+v", next.processes)
	}
}

func TestDiagnosticsResponsesExposeUnsupportedAndErrorStates(t *testing.T) {
	// Given active port and container generations.
	m := Model{screen: screenPorts}
	m.ports.generation = 3
	m.containers.generation = 4

	// When supported tooling is absent and another request fails.
	next, _ := updateModel(t, m, portsResultMsg{generation: 3, err: collect.ErrUnsupported})
	next, _ = updateModel(t, next, containersResultMsg{generation: 4, err: errors.New("ssh down")})

	// Then the states remain distinct for useful rendering.
	if next.ports.status != diagnosticsUnsupported || next.containers.status != diagnosticsError {
		t.Fatalf("unexpected statuses: ports=%v containers=%v", next.ports.status, next.containers.status)
	}
}

func TestDiagnosticsCadenceIsActiveScreenSpecific(t *testing.T) {
	// Given each diagnostics screen.
	cases := []struct {
		screen screenKind
		want   time.Duration
	}{{screenProcesses, 2 * time.Second}, {screenPorts, 5 * time.Second}, {screenContainers, 5 * time.Second}}

	// When cadence is requested.
	for _, tc := range cases {
		if got := diagnosticsCadence(tc.screen); got != tc.want {
			t.Fatalf("screen %v cadence=%s want=%s", tc.screen, got, tc.want)
		}
	}

	// Then non-diagnostics screens do not schedule diagnostics polling.
	if got := diagnosticsCadence(screenDashboard); got != 0 {
		t.Fatalf("dashboard cadence=%s", got)
	}
}

func TestDiagnosticCommandHonorsContext(t *testing.T) {
	// Given an already cancelled request context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When a command checks its context before calling the collector.
	msg := runDiagnostics(ctx, 8, screenProcesses, "web", nil)()

	// Then cancellation is returned with the matching generation.
	result, ok := msg.(processesResultMsg)
	if !ok || result.generation != 8 || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("unexpected result: %#v", msg)
	}
}

func containsText(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
