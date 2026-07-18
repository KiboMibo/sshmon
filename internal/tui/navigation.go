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
	if m.screen == screenLogs {
		if cmd, handled := m.handleLogsKey(key); handled {
			return m, cmd
		}
	}
	if m.overlay != overlayNone {
		if value == "esc" {
			m.overlay = overlayNone
		}
		return m, nil
	}
	if m.screen == screenHistory {
		if cmd, handled := m.handleHistoryKey(value); handled {
			return m, cmd
		}
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
		if m.screen == screenFleet {
			m.ensureFleet()
			m.moveFleet(-1)
		}
	case "down", "j":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.moveFleet(1)
		}
	case "pgup":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.moveFleet(-fleetPageSize)
		}
	case "pgdown":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.moveFleet(fleetPageSize)
		}
	case "g":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.fleet.filter.Group = cycleGroup(m.fleet.filter.Group, m.snapshot.Servers)
			m.selectNearestVisible()
		}
	case "!":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.fleet.filter.ProblemsOnly = !m.fleet.filter.ProblemsOnly
			m.selectNearestVisible()
		}
	case "v":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.fleet.preview = !m.fleet.preview
		}
	case "enter":
		if m.screen == screenFleet && len(m.snapshot.Servers) > 0 {
			m.screen = screenDashboard
			m.request++
		}
	case "p", "o", "h", "l", "d":
		if m.screen == screenDashboard {
			m.screen = dashboardDestination(value)
			if m.screen == screenProcesses || m.screen == screenPorts || m.screen == screenContainers {
				return m, m.startDiagnostics()
			}
			if m.screen == screenHistory {
				return m, m.startHistoryQuery()
			}
			if m.screen == screenLogs {
				return m, m.startLogsStream()
			}
			m.request++
		}
	case "esc":
		if isDeepScreen(m.screen) {
			m.cancelDiagnostics()
			m.cancelHistoryQuery()
			m.cancelLogsStream()
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
