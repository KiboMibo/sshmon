package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type paletteAction uint8

const (
	paletteOpenChat paletteAction = iota
	paletteOpenHelp
	paletteOpenProcesses
	paletteOpenPorts
	paletteOpenHistory
	paletteOpenLogs
	paletteOpenContainers
	paletteReconnect
	paletteOpenServer
)

type paletteItem struct {
	label  string
	action paletteAction
	server int
}

type paletteOverlay struct {
	input    textinput.Model
	items    []paletteItem
	selected int
}

func newPaletteOverlay() paletteOverlay {
	input := textinput.New()
	input.Placeholder = "команда или сервер"
	return paletteOverlay{input: input}
}

func paletteItems(m Model) []paletteItem {
	items := []paletteItem{{label: "Чат", action: paletteOpenChat}, {label: "Справка", action: paletteOpenHelp}}
	if m.screen == screenDashboard {
		items = append(items,
			paletteItem{label: "Переподключить", action: paletteReconnect},
			paletteItem{label: "Процессы", action: paletteOpenProcesses},
			paletteItem{label: "Порты", action: paletteOpenPorts},
			paletteItem{label: "История", action: paletteOpenHistory},
			paletteItem{label: "Логи", action: paletteOpenLogs},
			paletteItem{label: "Docker", action: paletteOpenContainers},
		)
	}
	for index, server := range m.snapshot.Servers {
		items = append(items, paletteItem{label: "Сервер: " + server.Name, action: paletteOpenServer, server: index})
	}
	return items
}

func (p *paletteOverlay) refresh(m Model) {
	query := strings.ToLower(strings.TrimSpace(p.input.Value()))
	p.items = nil
	for _, item := range paletteItems(m) {
		if query == "" || strings.Contains(strings.ToLower(item.label), query) {
			p.items = append(p.items, item)
		}
	}
	p.selected = min(max(0, p.selected), max(0, len(p.items)-1))
}

func (m *Model) handlePaletteKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "up", "ctrl+p":
		m.palette.selected = max(0, m.palette.selected-1)
		return nil
	case "down", "ctrl+n":
		m.palette.selected = min(max(0, len(m.palette.items)-1), m.palette.selected+1)
		return nil
	case "enter":
		if len(m.palette.items) == 0 {
			return nil
		}
		return m.executePalette(m.palette.items[m.palette.selected])
	default:
		var cmd tea.Cmd
		m.palette.input, cmd = m.palette.input.Update(key)
		m.palette.refresh(*m)
		return cmd
	}
}

func (m *Model) executePalette(item paletteItem) tea.Cmd {
	m.closeOverlay()
	switch item.action {
	case paletteOpenChat:
		return m.openOverlay(overlayChat)
	case paletteOpenHelp:
		return m.openOverlay(overlayHelp)
	case paletteOpenServer:
		m.selected, m.screen = item.server, screenDashboard
	case paletteOpenProcesses:
		m.screen = screenProcesses
		return m.startDiagnostics()
	case paletteOpenPorts:
		m.screen = screenPorts
		return m.startDiagnostics()
	case paletteOpenHistory:
		m.screen = screenHistory
		return m.startHistoryQuery()
	case paletteOpenLogs:
		m.screen = screenLogs
		return m.startLogsStream()
	case paletteOpenContainers:
		m.screen = screenContainers
		return m.startDiagnostics()
	case paletteReconnect:
		return m.startReconnect()
	}
	return nil
}

func (m Model) renderPalette() string {
	var lines []string
	for index, item := range m.palette.items {
		cursor := "  "
		if index == m.palette.selected {
			cursor = "▶ "
		}
		lines = append(lines, cursor+item.label)
	}
	return "Команды\n\n" + m.palette.input.View() + "\n" + strings.Join(lines, "\n") + "\n\n↑/↓ выбрать · enter выполнить · esc закрыть"
}
