package tui

import tea "github.com/charmbracelet/bubbletea"

type screenKind uint8

const (
	screenFleet screenKind = iota
	screenDashboard
	screenProcesses
	screenPorts
	screenHistory
	screenLogs
	screenContainers
)

type overlayKind uint8

const (
	overlayNone overlayKind = iota
	overlayChat
	overlaySearch
	overlayPalette
	overlayHelp
)

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := key.String()
	if m.overlay != overlayNone {
		if value == "esc" {
			m.overlay = overlayNone
		}
		return m, nil
	}

	switch value {
	case "c":
		m.overlay = overlayChat
	case "/":
		m.overlay = overlaySearch
	case ":":
		m.overlay = overlayPalette
	case "?":
		m.overlay = overlayHelp
	case "up", "k":
		if m.screen == screenFleet && m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.screen == screenFleet && m.selected+1 < len(m.snapshot.Servers) {
			m.selected++
		}
	case "enter":
		if m.screen == screenFleet && len(m.snapshot.Servers) > 0 {
			m.screen = screenDashboard
			m.request++
		}
	case "p", "o", "h", "l", "d":
		if m.screen == screenDashboard {
			m.screen = dashboardDestination(value)
			m.request++
		}
	case "esc":
		if isDeepScreen(m.screen) {
			m.screen = screenDashboard
			m.request++
		} else if m.screen == screenDashboard {
			m.screen = screenFleet
		}
	case "q", "ctrl+c":
		if m.screen == screenFleet {
			m.closeSubscription()
			return m, tea.Quit
		}
	}
	return m, nil
}

func dashboardDestination(key string) screenKind {
	switch key {
	case "p":
		return screenProcesses
	case "o":
		return screenPorts
	case "h":
		return screenHistory
	case "l":
		return screenLogs
	case "d":
		return screenContainers
	default:
		return screenDashboard
	}
}

func isDeepScreen(screen screenKind) bool {
	return screen >= screenProcesses && screen <= screenContainers
}

func overlayTitle(overlay overlayKind) string {
	switch overlay {
	case overlayChat:
		return "Чат"
	case overlaySearch:
		return "Поиск"
	case overlayPalette:
		return "Команды"
	case overlayHelp:
		return "Справка"
	default:
		return ""
	}
}
