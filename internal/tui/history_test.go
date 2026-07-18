package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/history"
)

func TestHistoryRangeChangeIncrementsGenerationAndIgnoresStaleResult(t *testing.T) {
	// Given an active history screen with a current result.
	m := Model{screen: screenHistory, snapshot: snapshotWithServers("web"), layout: newLayout(100, 24)}
	m.history.generation = 2
	m.history.points = []history.Point{{At: time.Unix(2, 0), Online: true}}

	// When an older result arrives and range key 2 is pressed.
	next, _ := updateModel(t, m, historyResultMsg{generation: 1, points: []history.Point{{At: time.Unix(1, 0)}}})
	next, cmd := updateModel(t, next, key("2"))

	// Then stale data is ignored and a fresh six-hour query is scheduled.
	if len(next.history.points) != 1 || !next.history.points[0].At.Equal(time.Unix(2, 0)) {
		t.Fatalf("stale result applied: %+v", next.history.points)
	}
	if next.history.selectedRange != 1 || next.history.generation <= 2 || cmd == nil {
		t.Fatalf("range request not refreshed: %+v", next.history)
	}
}

func TestHistoryGraphPreservesOfflineGapsAndCursorValue(t *testing.T) {
	// Given online points around one offline gap.
	a, b := 20.0, 80.0
	points := []history.Point{
		{At: time.Unix(10, 0), Online: true, CPU: &a},
		{At: time.Unix(20, 0), Online: false},
		{At: time.Unix(30, 0), Online: true, CPU: &b},
	}

	// When the CPU series and graph are rendered.
	values := historyMetricValues(points, historyMetricCPU)
	graph := renderHistoryGraph(values, 9)
	label := historyCursorLabel(points, historyMetricCPU, 2)

	// Then the gap stays visible and the cursor reports the exact sample.
	if values[1] != nil || !strings.Contains(graph, " ") {
		t.Fatalf("offline gap lost: values=%v graph=%q", values, graph)
	}
	if !strings.Contains(label, "00:00:30") || !strings.Contains(label, "80") {
		t.Fatalf("unexpected cursor label: %q", label)
	}
}

func TestHistoryScreenShowsFailSoftErrorWithoutChangingServerState(t *testing.T) {
	// Given an online server and a failed history query.
	m := Model{screen: screenHistory, snapshot: snapshotWithServers("web"), layout: newLayout(100, 24)}
	m.history.status = historyError
	m.history.err = errors.New("database unavailable")

	// When rendered.
	view := m.renderHistory()

	// Then the error is local to history and server health remains online.
	if !containsText(view, "database unavailable") || !m.snapshot.Servers[0].Online {
		t.Fatalf("unexpected fail-soft state:\n%s", view)
	}
}
