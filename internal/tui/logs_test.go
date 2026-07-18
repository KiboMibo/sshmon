package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/sshx"
)

type fakeLogStreamer struct {
	requests []collect.LogRequest
	streams  []sshx.Stream
}

func (f *fakeLogStreamer) StreamLogs(_ context.Context, request collect.LogRequest) (sshx.Stream, error) {
	f.requests = append(f.requests, request)
	if len(f.streams) == 0 {
		return sshx.Stream{}, errors.New("no fake stream")
	}
	stream := f.streams[0]
	f.streams = f.streams[1:]
	return stream, nil
}

func TestLogsOpenStartsStreamAndIgnoresStaleLines(t *testing.T) {
	// Given: a dashboard with one server and a controllable log stream.
	lines := make(chan string, 1)
	errs := make(chan error, 1)
	streamer := &fakeLogStreamer{streams: []sshx.Stream{{Lines: lines, Errors: errs, Close: func() error { return nil }}}}
	m := Model{
		screen:    screenDashboard,
		snapshot:  snapshotWithServers("web"),
		logSource: streamer,
		logs:      newLogsScreen(),
	}

	// When: the logs screen is opened and its first stream line arrives.
	updated, openCmd := updateModel(t, m, key("l"))
	opened := openCmd().(logsOpenedMsg)
	updated, waitCmd := updateModel(t, updated, opened)
	lines <- "fresh"
	lineMsg := waitCmd().(logLineMsg)
	updated, _ = updateModel(t, updated, lineMsg)
	updated, _ = updateModel(t, updated, logLineMsg{generation: opened.generation - 1, line: "stale"})

	// Then: one request was started and only the matching generation is visible.
	if len(streamer.requests) != 1 || streamer.requests[0].Server != "web" {
		t.Fatalf("requests = %#v", streamer.requests)
	}
	visible := updated.logs.buffer.Visible()
	if len(visible) != 1 || visible[0] != "fresh" {
		t.Fatalf("visible = %#v", visible)
	}
}

func TestLogsControlsPauseFilterCycleReconnectAndCancel(t *testing.T) {
	// Given: an active logs screen with two selectable sources.
	cancelled := 0
	m := Model{
		screen:   screenLogs,
		snapshot: snapshotWithServers("web"),
		logs:     newLogsScreen(),
	}
	m.logs.sources = []collect.LogSource{{Kind: collect.LogSystem}, {Kind: collect.LogJournal, Name: "nginx"}}
	m.logs.cancel = func() { cancelled++ }
	m.logs.buffer.Append("INFO ready")
	m.logs.buffer.Append("ERROR failed")

	// When: pause, filter, source-cycle, reconnect and escape are requested.
	m, _ = updateModel(t, m, key(" "))
	if !m.logs.paused || len(m.logs.buffer.Visible()) != 2 {
		t.Fatalf("pause state = %#v", m.logs)
	}
	m, _ = updateModel(t, m, key("/"))
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("error")})
	m, _ = updateModel(t, m, key("enter"))
	if got := m.logs.buffer.Visible(); len(got) != 1 || got[0] != "ERROR failed" {
		t.Fatalf("filtered = %#v", got)
	}
	oldGeneration := m.logs.generation
	m, _ = updateModel(t, m, key("s"))
	if m.logs.source != 1 || m.logs.generation <= oldGeneration {
		t.Fatalf("source=%d generation=%d", m.logs.source, m.logs.generation)
	}
	m.logs.cancel = func() { cancelled++ }
	beforeReconnect := m.logs.generation
	m, _ = updateModel(t, m, key("r"))
	if m.logs.generation <= beforeReconnect {
		t.Fatal("reconnect did not advance generation")
	}
	m.logs.cancel = func() { cancelled++ }
	m, _ = updateModel(t, m, key("esc"))

	// Then: leaving cancels the stream and returns to Dashboard.
	if m.screen != screenDashboard || cancelled < 3 {
		t.Fatalf("screen=%v cancelled=%d", m.screen, cancelled)
	}
}

func TestLogsBufferAndViewportStayBoundedWhilePausedAndResized(t *testing.T) {
	// Given: more than ten thousand lines in a paused log screen.
	m := Model{screen: screenLogs, logs: newLogsScreen()}
	for i := 0; i < 10_005; i++ {
		m.logs.buffer.Append("line")
	}
	m, _ = updateModel(t, m, key(" "))

	// When: the terminal is resized to its minimum supported height.
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 60, Height: 16})
	view := m.View()

	// Then: storage remains bounded and the viewport has a valid height.
	if len(m.logs.buffer.Visible()) != 10_000 {
		t.Fatalf("visible lines = %d", len(m.logs.buffer.Visible()))
	}
	if m.logs.viewport.Height <= 0 || view == "" {
		t.Fatalf("viewport height=%d view=%q", m.logs.viewport.Height, view)
	}
}
