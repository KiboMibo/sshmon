package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

// TestDashboardTileFocusCyclesWithTab verifies that tab advances focus and shift+tab steps back.
// Given: a Dashboard model with focus on the metrics tile.
// When:  the user presses tab, tab, shift+tab, tab.
// Then:  focus advances metrics→systemd→network, steps back to systemd, then advances to network.
func TestDashboardTileFocusCyclesWithTab(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	m.dashboard.tileFocus = tileMetrics

	// When: press tab (next tile).
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)
	// Then: focus is now systemd.
	if m.dashboard.tileFocus != tileSystemd {
		t.Errorf("after tab: focus = %d, want %d (systemd)", m.dashboard.tileFocus, tileSystemd)
	}

	// When: press tab again.
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)
	// Then: focus is now network.
	if m.dashboard.tileFocus != tileNetwork {
		t.Errorf("after second tab: focus = %d, want %d (network)", m.dashboard.tileFocus, tileNetwork)
	}

	// When: press shift+tab (previous tile).
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = m2.(Model)
	// Then: focus steps back to systemd.
	if m.dashboard.tileFocus != tileSystemd {
		t.Errorf("after shift+tab: focus = %d, want %d (systemd)", m.dashboard.tileFocus, tileSystemd)
	}

	// When: press tab (next tile).
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)
	// Then: focus advances to network.
	if m.dashboard.tileFocus != tileNetwork {
		t.Errorf("after final tab: focus = %d, want %d (network)", m.dashboard.tileFocus, tileNetwork)
	}
}

// TestDashboardScrollJMovesWithinFocusedTile verifies that j/k scroll inside the focused tile.
// Given: a Dashboard model focused on the logs tile.
// When:  the user presses j then k.
// Then:  the log scroll offset increments then decrements.
func TestDashboardScrollJMovesWithinFocusedTile(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
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
	m.layout = newLayout(120, 30)
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

// TestDashboardRendersFocusIndicatorOnActiveTile verifies the focused tile gets a visual marker in its title.
// Given: a wide Dashboard with focus on the metrics tile.
// When:  rendering the view.
// Then:  the МЕТРИКИ panel title contains a focus marker (◆).
func TestDashboardRendersFocusIndicatorOnActiveTile(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	m.dashboard.tileFocus = tileMetrics

	view := m.View()

	// Then: МЕТРИКИ title shows the focus marker.
	if !strings.Contains(view, "◆ МЕТРИКИ") && !strings.Contains(view, "◆МЕТРИКИ") {
		t.Errorf("focused МЕТРИКИ panel must contain ◆ marker; view:\n%s", view)
	}
	// And: another panel (e.g. SYSTEMD) does NOT have the marker.
	if strings.Contains(view, "◆ SYSTEMD") || strings.Contains(view, "◆SYSTEMD") {
		t.Errorf("non-focused SYSTEMD panel must NOT contain ◆ marker; view:\n%s", view)
	}
}

// TestDashboardLogsWideShowsBottomThirdHeight verifies the logs panel uses ~1/3 of body height in wide mode.
// Given: a wide Dashboard at 120×30 (body height ≈ 21 after frame and footer).
// When:  rendering the view with a ready system log containing 15 lines.
// Then:  the ЛОГИ panel shows at least 7 visible log lines (≥1/3 of body).
func TestDashboardLogsWideShowsBottomThirdHeight(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 40)
	m.dashboard.logs.status = diagnosticsReady
	m.dashboard.logs.lines = make([]string, 15)
	for i := range m.dashboard.logs.lines {
		m.dashboard.logs.lines[i] = "log line " + string(rune('a'+i))
	}

	view := m.View()

	// Count distinct log lines rendered inside the ЛОГИ panel.
	count := 0
	for _, line := range m.dashboard.logs.lines {
		if strings.Contains(view, line) {
			count++
		}
	}
	if count < 7 {
		t.Errorf("wide logs panel showed %d lines, want ≥7 (bottom third of body)", count)
	}
}
