package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestViewDrawsNoOuterFrameAndFitsTerminal(t *testing.T) {
	// Given Fleet on a normal terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then the screen occupies the terminal itself: no frame column on the left
	// and nothing overflows the terminal width.
	for i, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("line %d width = %d > 80: %q", i, width, line)
		}
		if strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "│") || strings.HasPrefix(line, "╰") {
			t.Fatalf("line %d still starts with an outer frame: %q", i, line)
		}
	}
}

func TestViewFillsTerminalHeight(t *testing.T) {
	// Given Fleet on a normal terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then the screen stretches to the full terminal height.
	if lines := strings.Split(view, "\n"); len(lines) != 24 {
		t.Fatalf("view has %d lines, want 24:\n%s", len(lines), view)
	}
}

func TestFleetHintsStayOnTheLastRow(t *testing.T) {
	// Given Fleet on a full-height terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then Fleet hints occupy the final row.
	assertFooterIsLastLine(t, view, 24, "enter открыть")
}

func TestDashboardHintsStayOnTheLastRow(t *testing.T) {
	// Given Dashboard on a full-height terminal.
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then Dashboard hints occupy the final row.
	assertFooterIsLastLine(t, view, 24, "r обновить")
}

func TestDeepScreenHintsStayOnTheLastRow(t *testing.T) {
	// Given a process screen on a full-height terminal.
	m := Model{screen: screenProcesses, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then deep-screen hints occupy the final row.
	assertFooterIsLastLine(t, view, 24, "esc назад")
}

func TestLogsHintsStayOnTheLastRow(t *testing.T) {
	// Given: полноэкранные логи на терминале полной высоты.
	m := Model{screen: screenLogs, snapshot: snapshotWithServers("web"), logs: newLogsScreen()}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	for i := range 40 {
		m.logs.buffer.Append("19:41:0" + string(rune('0'+i%10)) + " info строка")
	}
	m.logs.refresh()

	// When: кадр отрисован.
	view := m.View()

	// Then: кадр ровно по высоте терминала, подсказки — на последней строке.
	assertFooterIsLastLine(t, view, 24, "esc закрыть")
}

func TestOverlayKeepsHintsOnTheLastRow(t *testing.T) {
	// Given Help open over Fleet on a full-height terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, key("?"))

	// When the view is rendered.
	view := m.View()

	// Then Help stays visible while Fleet hints remain on the final row.
	if !strings.Contains(view, "КЛАВИШИ") {
		t.Fatalf("view misses help overlay:\n%s", view)
	}
	assertFooterIsLastLine(t, view, 24, "enter открыть")
}

func TestOverlayStaysInsideTerminalHeight(t *testing.T) {
	// Given an open help overlay on a normal terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, key("?"))

	// When the view is rendered.
	view := m.View()

	// Then the overlay neither pushes the screen past the terminal nor overflows it.
	if !strings.Contains(view, "КЛАВИШИ") {
		t.Fatalf("view misses help overlay:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("view with overlay has %d lines, want 24:\n%s", len(lines), view)
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("overlay line %d width = %d > 80: %q", i, width, line)
		}
	}
}

func TestTooSmallGateRendersWithoutFrame(t *testing.T) {
	// Given a terminal below the minimum supported size.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 59, Height: 15})

	// When the view is rendered.
	view := m.View()

	// Then only the resize hint is shown, without frame glyphs.
	if !strings.Contains(view, "увеличьте терминал") {
		t.Fatalf("view misses resize hint:\n%s", view)
	}
	if strings.ContainsAny(view, "╭╮╰╯│") {
		t.Fatalf("too-small gate must not draw a frame:\n%s", view)
	}
}

func assertFooterIsLastLine(t *testing.T, view string, height int, footer string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, view)
	}
	if !strings.Contains(lines[len(lines)-1], footer) {
		t.Fatalf("last row misses %q: %q", footer, lines[len(lines)-1])
	}
}
