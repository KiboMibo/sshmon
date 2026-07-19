package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestViewWrapsScreenInRoundedFrame(t *testing.T) {
	// Given Fleet on a normal terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then the rounded frame surrounds the screen and nothing overflows the terminal width.
	for _, glyph := range []string{"╭", "╮", "╰", "╯", "│"} {
		if !strings.Contains(view, glyph) {
			t.Fatalf("view misses frame glyph %q:\n%s", glyph, view)
		}
	}
	for i, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("line %d width = %d > 80: %q", i, width, line)
		}
	}
}

func TestViewFillsTerminalHeight(t *testing.T) {
	// Given Fleet on a normal terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then the frame stretches to the full terminal height.
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("view has %d lines, want 24:\n%s", len(lines), view)
	}
	if !strings.Contains(lines[len(lines)-1], "╰") {
		t.Fatalf("last line is not the bottom border: %q", lines[len(lines)-1])
	}
}

func TestFleetHintsStayAboveBottomBorder(t *testing.T) {
	// Given Fleet on a full-height terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then Fleet hints occupy the final content row.
	assertFooterAboveBottomBorder(t, view, 24, "enter открыть")
}

func TestDashboardHintsStayAboveBottomBorder(t *testing.T) {
	// Given Dashboard on a wide full-height terminal.
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// When the view is rendered.
	view := m.View()

	// Then Dashboard hints occupy the final content row.
	assertFooterAboveBottomBorder(t, view, 30, "r переподключить")
}

func TestDeepScreenHintsStayAboveBottomBorder(t *testing.T) {
	// Given a process screen on a full-height terminal.
	m := Model{screen: screenProcesses, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the view is rendered.
	view := m.View()

	// Then deep-screen hints occupy the final content row.
	assertFooterAboveBottomBorder(t, view, 24, "esc назад")
}

func TestOverlayKeepsHintsAboveBottomBorder(t *testing.T) {
	// Given Help open over Fleet on a full-height terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, key("?"))

	// When the view is rendered.
	view := m.View()

	// Then Help stays visible while Fleet hints remain on the final content row.
	if !strings.Contains(view, "Справка") {
		t.Fatalf("view misses help overlay:\n%s", view)
	}
	assertFooterAboveBottomBorder(t, view, 24, "enter открыть")
}

func TestOverlayRendersInsideFrame(t *testing.T) {
	// Given an open help overlay on a normal terminal.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateModel(t, m, key("?"))

	// When the view is rendered.
	view := m.View()

	// Then the overlay stays inside the full-height frame.
	if !strings.Contains(view, "Справка") {
		t.Fatalf("view misses help overlay:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("view with overlay has %d lines, want 24:\n%s", len(lines), view)
	}
	if !strings.Contains(lines[len(lines)-1], "╰") {
		t.Fatalf("overlay leaked below the frame, last line: %q", lines[len(lines)-1])
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

func assertFooterAboveBottomBorder(t *testing.T, view string, height int, footer string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, view)
	}
	if !strings.Contains(lines[len(lines)-2], footer) {
		t.Fatalf("last content row misses %q: %q", footer, lines[len(lines)-2])
	}
	if !strings.Contains(lines[len(lines)-1], "╰") {
		t.Fatalf("last line is not the bottom border: %q", lines[len(lines)-1])
	}
}
