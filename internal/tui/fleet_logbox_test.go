package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/sshx"
)

func logboxTestModel() (Model, *fakeLogStreamer) {
	streamer := &fakeLogStreamer{streams: []sshx.Stream{{
		Lines:  make(chan string, 1),
		Errors: make(chan error, 1),
		Close:  func() error { return nil },
	}}}
	m := Model{
		screen:    screenFleet,
		snapshot:  snapshotWithServers("web"),
		logSource: streamer,
		logs:      newLogsScreen(),
		layout:    newLayout(120, 30),
	}
	return m, streamer
}

func TestFleetLogboxOpensOverListAndEscapeCloses(t *testing.T) {
	// Given: the fleet screen with a server and a log stream available.
	m, streamer := logboxTestModel()

	// When: the log drawer is opened with "l" and the returned command runs.
	opened, cmd := updateModel(t, m, key("l"))
	if cmd != nil {
		cmd()
	}

	// Then: the drawer is visible above the list and a stream was requested.
	if !opened.fleet.logbox {
		t.Fatal("logbox not opened")
	}
	if view := opened.View(); !strings.Contains(view, "ЛОГИ ·") {
		t.Fatalf("drawer missing from view:\n%s", view)
	}
	if len(streamer.requests) != 1 || streamer.requests[0].Server != "web" {
		t.Fatalf("requests = %#v", streamer.requests)
	}

	// When: escape is pressed.
	closed, _ := updateModel(t, opened, tea.KeyMsg{Type: tea.KeyEsc})

	// Then: the drawer is gone and the fleet screen stays.
	if closed.fleet.logbox {
		t.Fatal("logbox still open")
	}
	if closed.screen != screenFleet {
		t.Fatalf("screen = %v", closed.screen)
	}
}

func TestFleetLogboxEnterGoesFullScreen(t *testing.T) {
	// Given: the fleet screen with an open log drawer.
	m, _ := logboxTestModel()
	opened, _ := updateModel(t, m, key("l"))

	// When: enter is pressed inside the drawer.
	full, _ := updateModel(t, opened, tea.KeyMsg{Type: tea.KeyEnter})

	// Then: the full logs screen takes over and the drawer closes.
	if full.screen != screenLogs {
		t.Fatalf("screen = %v", full.screen)
	}
	if full.fleet.logbox {
		t.Fatal("logbox still open")
	}
}

func TestLogsLevelAxisFiltersAndCounts(t *testing.T) {
	// Given: a logs screen holding lines of mixed severity.
	logs := newLogsScreen()
	logs.ensure()
	logs.buffer.Append("info started")
	logs.buffer.Append("WARN disk almost full")
	logs.buffer.Append("ERROR failed to write")

	// When: the level axis is switched to warn and above.
	logs.level = logLevelWarn
	visible := logs.visibleLines()

	// Then: only warn and error survive, and the counter reports both totals.
	if len(visible) != 2 {
		t.Fatalf("visible = %#v", visible)
	}
	m := Model{logs: logs}
	if hint := m.logsCountHint(); hint != "2 из 3 строк" {
		t.Fatalf("hint = %q", hint)
	}
}

func TestLogSourceLabelKeepsUnitName(t *testing.T) {
	// Given: a system source and a journal unit source.
	// When: both are labelled for the source axis.
	// Then: the unit name is kept and the plain system source stays short.
	if label := logSourceLabel(collect.LogSource{Kind: collect.LogSystem}); label != "system" {
		t.Fatalf("label = %q", label)
	}
	if label := logSourceLabel(collect.LogSource{Kind: collect.LogJournal, Name: "sshd.service"}); label != "journal/sshd.service" {
		t.Fatalf("label = %q", label)
	}
}
