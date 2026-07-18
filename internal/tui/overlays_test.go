package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOverlayCapturesKeysBeforeLogsAndPreservesInputOnResize(t *testing.T) {
	// Given a Logs screen with an active Search overlay.
	m := Model{screen: screenLogs, overlay: overlaySearch, search: newSearchOverlay()}
	m.search.input.SetValue("error")

	// When a log shortcut and a resize arrive while the overlay owns focus.
	m, _ = updateModel(t, m, key(" "))
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 90, Height: 25})

	// Then Logs remain unpaused and overlay content survives the resize.
	if m.logs.paused {
		t.Fatal("Logs shortcut escaped through active overlay")
	}
	if got := m.search.input.Value(); got != "error" {
		t.Fatalf("search input after resize = %q", got)
	}
}

func TestEscapeClosesOverlayBeforeNavigatingScreen(t *testing.T) {
	// Given Help over a deep screen.
	m := Model{screen: screenProcesses, overlay: overlayHelp}

	// When Escape is pressed once.
	m, _ = updateModel(t, m, key("esc"))

	// Then only Help closes and the deep screen stays active.
	if m.overlay != overlayNone || m.screen != screenProcesses {
		t.Fatalf("overlay=%v screen=%v", m.overlay, m.screen)
	}
}

func TestSearchOverlayAppliesFleetQueryAndSelectsMatch(t *testing.T) {
	// Given Fleet with a Search query matching only the second server.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web", "db"), fleet: newFleetModel(), search: newSearchOverlay()}
	m.overlay = overlaySearch
	m.search.input.SetValue("db")

	// When Search is submitted.
	m, _ = updateModel(t, m, key("enter"))

	// Then the filter is applied, the match is selected and the overlay closes.
	if m.overlay != overlayNone || m.fleet.filter.Query != "db" || m.selected != 1 {
		t.Fatalf("overlay=%v query=%q selected=%d", m.overlay, m.fleet.filter.Query, m.selected)
	}
}

func TestHelpContentDependsOnActiveScreen(t *testing.T) {
	// Given Fleet and Dashboard contexts.
	fleetHelp := helpText(screenFleet)
	dashboardHelp := helpText(screenDashboard)

	// When their key hints are rendered.
	// Then Dashboard advertises diagnostics while Fleet advertises search.
	if strings.Contains(fleetHelp, "p процессы") || !strings.Contains(fleetHelp, "/ поиск") {
		t.Fatalf("unexpected Fleet help: %q", fleetHelp)
	}
	if !strings.Contains(dashboardHelp, "p процессы") || !strings.Contains(dashboardHelp, "l логи") {
		t.Fatalf("unexpected Dashboard help: %q", dashboardHelp)
	}
}
