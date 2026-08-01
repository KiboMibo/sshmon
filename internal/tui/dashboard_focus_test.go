package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

// TestDashboardTileFocusCyclesWithTab verifies that tab advances focus and shift+tab steps back.
// Given: a server screen focused on services.
// When:  the user presses tab, tab, shift+tab, tab.
// Then:  focus walks the framed tiles only — services→docker→logs and back.
func TestDashboardTileFocusCyclesWithTab(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.dashboard.tileFocus = tileSystemd

	// When: press tab (next tile).
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)
	// Then: focus is now docker.
	if m.dashboard.tileFocus != tileDocker {
		t.Errorf("after tab: focus = %d, want %d (docker)", m.dashboard.tileFocus, tileDocker)
	}

	// When: press tab again.
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)
	// Then: focus is now logs.
	if m.dashboard.tileFocus != tileLogs {
		t.Errorf("after second tab: focus = %d, want %d (logs)", m.dashboard.tileFocus, tileLogs)
	}

	// When: press shift+tab (previous tile).
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = m2.(Model)
	// Then: focus steps back to docker.
	if m.dashboard.tileFocus != tileDocker {
		t.Errorf("after shift+tab: focus = %d, want %d (docker)", m.dashboard.tileFocus, tileDocker)
	}

	// When: press tab (next tile).
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)
	// Then: focus advances to logs.
	if m.dashboard.tileFocus != tileLogs {
		t.Errorf("after final tab: focus = %d, want %d (logs)", m.dashboard.tileFocus, tileLogs)
	}
}

// TestDashboardScrollJMovesWithinFocusedTile verifies that j/k scroll inside the focused tile.
// Given: a Dashboard model focused on the logs tile.
// When:  the user presses j then k.
// Then:  the log scroll offset increments then decrements.
func TestDashboardScrollJMovesWithinFocusedTile(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.dashboard.tileFocus = tileLogs
	m.dashboard.tileScrolls[tileLogs] = 0

	// When: press j (scroll down inside logs).
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = m2.(Model)
	// Then: logs scroll incremented.
	if m.dashboard.tileScrolls[tileLogs] == 0 {
		t.Errorf("after j: logs scroll still 0, want >0")
	}

	// When: press k (scroll up inside logs).
	prev := m.dashboard.tileScrolls[tileLogs]
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = m2.(Model)
	// Then: logs scroll decremented (never negative).
	if m.dashboard.tileScrolls[tileLogs] >= prev {
		t.Errorf("after k: scroll = %d, want < %d", m.dashboard.tileScrolls[tileLogs], prev)
	}
}

// TestDashboardSystemdFocusPreservesCursorBehavior verifies that focusing systemd and pressing j/k still moves the cursor.
// Given: a Dashboard model focused on systemd with multiple units loaded.
// When:  the user presses j.
// Then:  unitUI.cursor increments (legacy behavior preserved when focused on systemd).
func TestDashboardSystemdFocusPreservesCursorBehavior(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.dashboard.units.items = []collect.SystemdUnit{
		{Name: "sshd.service", Active: "active", Sub: "running"},
		{Name: "cron.service", Active: "active", Sub: "running"},
	}
	m.dashboard.tileFocus = tileSystemd
	m.ensureDashboardUnitUI()
	m.dashboard.unitUI.cursor = 0

	// When: press j (cursor down inside systemd).
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = m2.(Model)
	// Then: cursor moved past 0.
	if m.dashboard.unitUI.cursor == 0 {
		t.Errorf("after j on systemd: cursor still 0, want >0")
	}
}

// TestDashboardRendersFocusIndicatorOnActiveTile verifies the focused tile gets a green border.
// Given: a server screen with focus on the docker tile.
// When:  the border style is resolved per tile.
// Then:  the focused tile uses focusStyle (green) and others stay dim.
func TestDashboardRendersFocusIndicatorOnActiveTile(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.dashboard.tileFocus = tileDocker

	if m.tileBorderStyle(tileDocker).GetForeground() != focusStyle.GetForeground() {
		t.Errorf("focused tile must use the green focus border")
	}
	if m.tileBorderStyle(tileSystemd).GetForeground() != dimStyle.GetForeground() {
		t.Errorf("non-focused tile must keep the dim border")
	}
}

// TestDashboardLogsTakeTheRemainingHeight verifies the logs tile grows into the leftover height.
// Given: a tall server screen with a ready system log of fifteen lines.
// When:  the view is rendered.
// Then:  the logs tile shows every line — logs take what the middle row left.
func TestDashboardLogsTakeTheRemainingHeight(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 40)
	m.dashboard.logs.status = diagnosticsReady
	m.dashboard.logs.lines = make([]string, 15)
	for i := range m.dashboard.logs.lines {
		m.dashboard.logs.lines[i] = "log line " + string(rune('a'+i))
	}

	view := m.View()

	count := 0
	for _, line := range m.dashboard.logs.lines {
		if strings.Contains(view, line) {
			count++
		}
	}
	if count != len(m.dashboard.logs.lines) {
		t.Errorf("logs tile showed %d of %d lines:\n%s", count, len(m.dashboard.logs.lines), view)
	}
}
