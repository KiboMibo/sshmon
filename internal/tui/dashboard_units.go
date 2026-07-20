package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

type dashboardUnitUI struct {
	input       textinput.Model
	initialized bool
	active      bool
	cursor      int
}

func (m *Model) ensureDashboardUnitUI() {
	if m.dashboard.unitUI.initialized {
		return
	}
	input := textinput.New()
	input.Placeholder = "имя или описание юнита"
	input.Width = max(12, min(40, m.layout.width/2))
	m.dashboard.unitUI = dashboardUnitUI{input: input, initialized: true}
}

func (m Model) filteredDashboardUnits() []collect.SystemdUnit {
	query := ""
	if m.dashboard.unitUI.initialized {
		query = strings.ToLower(strings.TrimSpace(m.dashboard.unitUI.input.Value()))
	}
	if query == "" {
		return append([]collect.SystemdUnit(nil), m.dashboard.units.items...)
	}
	units := make([]collect.SystemdUnit, 0, len(m.dashboard.units.items))
	for _, unit := range m.dashboard.units.items {
		haystack := strings.ToLower(unit.Name + " " + unit.Description)
		if strings.Contains(haystack, query) {
			units = append(units, unit)
		}
	}
	return units
}

func (m Model) systemdScroll(rowH int) int {
	units := m.filteredDashboardUnits()
	if len(units) == 0 {
		return 0
	}
	offset := 0
	if m.dashboard.unitUI.initialized && m.dashboard.unitUI.input.Value() != "" {
		offset = 1
	}
	cursorRow := offset + min(m.dashboard.unitUI.cursor, len(units)-1)
	if cursorRow >= rowH {
		return cursorRow - rowH + 1
	}
	return 0
}

func (m *Model) clampDashboardUnitCursor() {
	units := m.filteredDashboardUnits()
	if len(units) == 0 {
		m.dashboard.unitUI.cursor = 0
		return
	}
	m.dashboard.unitUI.cursor = max(0, min(m.dashboard.unitUI.cursor, len(units)-1))
}

func (m *Model) handleDashboardKey(key tea.KeyMsg) (tea.Cmd, bool) {
	m.ensureDashboardUnitUI()
	value := key.String()
	if m.dashboard.unitUI.active {
		switch value {
		case "esc", "enter":
			m.dashboard.unitUI.active = false
			m.dashboard.unitUI.input.Blur()
			m.clampDashboardUnitCursor()
			return nil, true
		default:
			var cmd tea.Cmd
			m.dashboard.unitUI.input, cmd = m.dashboard.unitUI.input.Update(key)
			m.clampDashboardUnitCursor()
			return cmd, true
		}
	}

	return m.handleDashboardFocusKey(value)
}
