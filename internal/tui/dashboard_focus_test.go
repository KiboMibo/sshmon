package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

// TestDashboardTileFocusCyclesWithTab verifies that tab advances focus and shift+tab steps back.
// Given: a server screen focused on services.
// When:  the user presses tab, tab, shift+tab, tab.
// Then:  focus walks the framed tiles only — services→docker→ports→logs and back.
func TestDashboardTileFocusCyclesWithTab(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.dashboard.tileFocus = tileSystemd

	// When/Then: tab обходит все плитки по порядку и возвращается к началу.
	for _, want := range []uint8{tileDocker, tilePorts, tileLogs, tileSystemd} {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = m2.(Model)
		if m.dashboard.tileFocus != want {
			t.Fatalf("after tab: focus = %d, want %d", m.dashboard.tileFocus, want)
		}
	}

	// When/Then: shift+tab идёт тем же кругом в обратную сторону.
	for _, want := range []uint8{tileLogs, tilePorts, tileDocker, tileSystemd} {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		m = m2.(Model)
		if m.dashboard.tileFocus != want {
			t.Fatalf("after shift+tab: focus = %d, want %d", m.dashboard.tileFocus, want)
		}
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

// TestDashboardPortsTileFocusesAndScrolls — Дано: экран сервера с длинным
// списком портов; Когда: фокус доходит до плитки ПОРТЫ и пользователь жмёт
// j/k; Тогда: плитка подсвечивает рамку и прокручивается по строкам своей
// многоколоночной раскладки, а не остаётся мёртвой.
func TestDashboardPortsTileFocusesAndScrolls(t *testing.T) {
	m := dashboardWorkspaceFixture()
	ports := make([]collect.Port, 0, 40)
	for i := range 40 {
		ports = append(ports, collect.Port{Proto: "tcp", Local: fmt.Sprintf("10.0.0.%d:%d", i, 8000+i)})
	}
	m.snapshot.Servers[0].Ports = ports
	m.dashboard.tileFocus = tilePorts

	// Тогда: рамка сфокусированной плитки зелёная, у соседей — тусклая.
	if m.tileBorderStyle(tilePorts).GetForeground() != focusStyle.GetForeground() {
		t.Fatalf("плитка ПОРТЫ не подсвечивает рамку при фокусе")
	}
	if m.tileBorderStyle(tileSystemd).GetForeground() != dimStyle.GetForeground() {
		t.Fatalf("соседняя плитка потеряла тусклую рамку")
	}

	before := stripANSI(m.View())
	if !strings.Contains(before, "ПОРТЫ 40") {
		t.Fatalf("плитки ПОРТЫ нет на экране:\n%s", before)
	}
	// Когда: j прокручивает содержимое сфокусированной плитки.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = m2.(Model)
	if m.dashboard.tileScrolls[tilePorts] != 1 {
		t.Fatalf("скролл портов = %d, want 1", m.dashboard.tileScrolls[tilePorts])
	}
	after := stripANSI(m.View())
	if after == before {
		t.Fatalf("прокрутка портов не изменила кадр:\n%s", after)
	}
	// И: k возвращает список назад и не уходит в минус.
	for range 3 {
		m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		m = m2.(Model)
	}
	if m.dashboard.tileScrolls[tilePorts] != 0 {
		t.Fatalf("скролл портов ушёл в %d", m.dashboard.tileScrolls[tilePorts])
	}
	if stripANSI(m.View()) != before {
		t.Fatalf("кадр не вернулся к началу списка портов")
	}
}

// TestDashboardTileCycleSurvivesEmptyPorts — Дано: хост без открытых портов;
// Когда: пользователь обходит плитки табом; Тогда: фокус не залипает на пустой
// плитке и обходит круг целиком в обе стороны.
func TestDashboardTileCycleSurvivesEmptyPorts(t *testing.T) {
	m := dashboardWorkspaceFixture()
	m.snapshot.Servers[0].Ports = nil
	m.dashboard.tileFocus = tilePorts

	if view := stripANSI(m.View()); !strings.Contains(view, "портов нет") {
		t.Fatalf("плитка ПОРТЫ без портов не объяснила пустоту:\n%s", view)
	}
	// j на пустой плитке ничего не ломает, а tab уводит фокус дальше.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = m2.(Model)
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = m2.(Model)
	if m.dashboard.tileFocus != tileLogs {
		t.Fatalf("фокус залип на пустой плитке ПОРТЫ: %d", m.dashboard.tileFocus)
	}
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = m2.(Model)
	if m.dashboard.tileFocus != tilePorts {
		t.Fatalf("shift+tab не вернулся к плитке ПОРТЫ: %d", m.dashboard.tileFocus)
	}
}
