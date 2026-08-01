package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

const fleetLogboxLines = 5

func (m *Model) openFleetLogbox() tea.Cmd {
	m.ensureFleet()
	m.fleet.logbox = true
	return m.startLogsStream()
}

func (m *Model) closeFleetLogbox() {
	m.fleet.logbox = false
	m.cancelLogsStream()
}

func (m *Model) handleFleetLogboxKey(key tea.KeyMsg) (tea.Cmd, bool) {
	if m.logs.filtering {
		return m.handleLogsKey(key)
	}
	switch key.String() {
	case "enter":
		m.fleet.logbox = false
		m.screen = screenLogs
		return nil, true
	case "esc":
		m.closeFleetLogbox()
		return nil, true
	case "up", "k":
		return m.moveFleetLogbox(-1), true
	case "down", "j":
		return m.moveFleetLogbox(1), true
	case "pgup":
		return m.moveFleetLogbox(-fleetPageSize), true
	case "pgdown":
		return m.moveFleetLogbox(fleetPageSize), true
	}
	return m.handleLogsKey(key)
}

// moveFleetLogbox двигает выбор по списку хостов вместе с потоком логов:
// заголовок ящика подписан выбранным хостом, поэтому оставлять стрим на
// прежнем нельзя — строки не совпадали бы с заголовком.
func (m *Model) moveFleetLogbox(delta int) tea.Cmd {
	previous := m.selected
	cmd := m.moveFleetBy(delta)
	if m.selected == previous {
		return cmd
	}
	return tea.Batch(cmd, m.startLogsStream())
}

func (m Model) fleetLogboxLines(width int) []string {
	if !m.fleet.logbox {
		return nil
	}
	visible := m.logs.visibleLines()
	body := []string{spread(dimStyle.Render(m.fleetLogboxStatus()), dimStyle.Render(m.logsCountHint()), width-4)}
	start := max(0, len(visible)-fleetLogboxLines)
	for _, line := range visible[start:] {
		body = append(body, fitLine(highlightMatches(line, m.logs.filterInput.Value()), width-4))
	}
	if len(visible) == 0 {
		body = append(body, dimStyle.Render("строк пока нет"))
	}
	return panelBoxStyled("ЛОГИ · "+m.selectedName(), "↑↓ хост · / фильтр · w уровень · s источник · enter весь экран · esc",
		width, body, dimStyle)
}

func (m Model) fleetLogboxStatus() string {
	source := "system"
	if len(m.logs.sources) > 0 {
		source = logSourceLabel(m.logs.sources[m.logs.source])
	}
	return source + " · " + logLevelNames[m.logs.level] + " · " + m.logsState()
}
