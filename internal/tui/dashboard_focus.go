package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

// Плитки дашборда в порядке обхода фокуса (h/l/tab), как в lazydocker.
const (
	tileMetrics uint8 = iota
	tileSystemd
	tileNetwork
	tileDocker
	tileLogs
	numDashboardTiles
)

// cycleDashboardTile сдвигает фокус между плитками по кругу.
func (m *Model) cycleDashboardTile(delta int) {
	count := int(numDashboardTiles)
	next := (int(m.dashboard.tileFocus) + delta%count + count) % count
	m.dashboard.tileFocus = uint8(next)
}

// clampDashboardScroll не даёт скроллу плитки уйти ниже нуля.
func (m *Model) clampDashboardScroll(idx uint8) {
	if m.dashboard.tileScrolls[idx] < 0 {
		m.dashboard.tileScrolls[idx] = 0
	}
}

// tileBorderStyle подсвечивает рамку сфокусированной плитки зелёным.
func (m Model) tileBorderStyle(idx uint8) lipgloss.Style {
	if m.dashboard.tileFocus == idx {
		return focusStyle
	}
	return dimStyle
}

// handleDashboardFocusKey обрабатывает навигацию lazydocker-стиля, когда
// фильтр юнитов не активен: h/l/tab переключают плитки, j/k скроллят внутри.
func (m *Model) handleDashboardFocusKey(value string) (tea.Cmd, bool) {
	switch value {
	case "tab":
		m.cycleDashboardTile(1)
		return nil, true
	case "shift+tab":
		m.cycleDashboardTile(-1)
		return nil, true
	case "down", "j":
		return m.scrollFocusedTile(1), true
	case "up", "k":
		return m.scrollFocusedTile(-1), true
	case "enter":
		return m.activateFocusedTile(), true
	case "f":
		if m.dashboard.tileFocus == tileSystemd {
			m.dashboard.unitUI.active = true
			m.dashboard.unitUI.input.Focus()
			return textinput.Blink, true
		}
		return nil, true
	case "x":
		m.dashboard.tileScrolls[tileLogs] = 0
		m.dashboard.unitUI.input.Reset()
		m.dashboard.unitUI.input.Blur()
		m.dashboard.unitUI.active = false
		m.dashboard.unitUI.cursor = 0
		return m.startDashboardLog(collect.LogSource{Kind: collect.LogSystem}), true
	}
	return nil, false
}

// scrollFocusedTile двигает курсор systemd или скролл прочих плиток.
func (m *Model) scrollFocusedTile(delta int) tea.Cmd {
	if m.dashboard.tileFocus == tileSystemd {
		m.dashboard.unitUI.cursor += delta
		m.clampDashboardUnitCursor()
		return nil
	}
	m.dashboard.tileScrolls[m.dashboard.tileFocus] += delta
	m.clampDashboardScroll(m.dashboard.tileFocus)
	return nil
}

// activateFocusedTile открывает journal выбранного systemd-юнита.
func (m *Model) activateFocusedTile() tea.Cmd {
	if m.dashboard.tileFocus != tileSystemd {
		return nil
	}
	units := m.filteredDashboardUnits()
	if len(units) == 0 {
		return nil
	}
	m.clampDashboardUnitCursor()
	return m.startDashboardLog(collect.LogSource{Kind: collect.LogJournal, Name: units[m.dashboard.unitUI.cursor].Name})
}
