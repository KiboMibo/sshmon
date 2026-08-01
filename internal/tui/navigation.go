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
	overlayPassphrase
)

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := key.String()
	// ctrl+c разбираем до всех остальных обработчиков: оверлеи и поля ввода
	// (чат, поиск, палитра, фильтры) забирают клавиши себе, и обещанный справкой
	// выход был бы из них недостижим. В набираемый текст ctrl+c не попадает.
	if value == "ctrl+c" {
		m.closeSubscription()
		return m, tea.Quit
	}
	if cmd, handled := m.handleOverlayKey(key); handled {
		return m, cmd
	}
	if m.screen == screenFleet && m.fleet.searching {
		return m, m.handleFleetSearchKey(key)
	}
	if m.screen == screenFleet && m.fleet.logbox {
		if cmd, handled := m.handleFleetLogboxKey(key); handled {
			return m, cmd
		}
	}
	if m.screen == screenLogs {
		if cmd, handled := m.handleLogsKey(key); handled {
			return m, cmd
		}
	}
	if m.screen == screenHistory {
		if cmd, handled := m.handleHistoryKey(value); handled {
			return m, cmd
		}
	}
	if m.screen == screenDashboard {
		if cmd, handled := m.handleDashboardKey(key); handled {
			return m, cmd
		}
	}

	switch value {
	case "c":
		return m, m.openOverlay(overlayChat)
	case "/":
		if m.screen == screenFleet {
			return m, m.openFleetSearch()
		}
		return m, m.openOverlay(overlaySearch)
	case ":":
		return m, m.openOverlay(overlayPalette)
	case "?":
		return m, m.openOverlay(overlayHelp)
	case "r":
		return m, m.refreshCurrentScreen()
	case "ctrl+r":
		return m, m.startReconnect()
	case "up", "k":
		if m.screen == screenFleet {
			return m, m.moveFleetBy(-1)
		}
	case "down", "j":
		if m.screen == screenFleet {
			return m, m.moveFleetBy(1)
		}
	case "pgup":
		if m.screen == screenFleet {
			return m, m.moveFleetBy(-fleetPageSize)
		}
	case "pgdown":
		if m.screen == screenFleet {
			return m, m.moveFleetBy(fleetPageSize)
		}
	case "right":
		if m.screen == screenFleet && len(m.snapshot.Servers) > 0 {
			m.ensureFleet()
			m.fleet.expanded = true
			return m, m.startFleetCardUnits()
		}
	case "left":
		if m.screen == screenFleet {
			m.fleet.expanded = false
		}
	case "g", "tab":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.fleet.filter.Group = cycleGroup(m.fleet.filter.Group, m.snapshot.Servers)
			m.selectNearestVisible()
		}
	case "a":
		if m.screen == screenFleet {
			m.ensureFleet()
			m.fleet.filter.Group = ""
			m.selectNearestVisible()
		}
	case "!", "f":
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
			return m, m.startDashboardWorkspace()
		}
	case "l", "ctrl+l":
		if m.screen == screenFleet {
			// На списке хостов «l» — ящик логов над списком (отдельная фича),
			// на весь экран уходим из раскрытой карточки или по ctrl+l.
			if m.fleet.expanded || value == "ctrl+l" {
				return m.openFromFleet(screenLogs)
			}
			return m, m.openFleetLogbox()
		}
		if m.screen == screenDashboard {
			m.screen = screenLogs
			return m, m.startLogsStream()
		}
	case "h", "ctrl+h":
		if m.screen == screenDashboard {
			m.screen = screenHistory
			return m, m.startHistoryQuery()
		}
	case "x":
		return m, m.startSSHSession()
	case "p", "o", "d":
		if m.screen == screenFleet && m.fleet.expanded {
			return m.openFromFleet(dashboardDestination(value))
		}
		if m.screen == screenDashboard {
			m.screen = dashboardDestination(value)
			return m, m.startDiagnostics()
		}
	case "esc":
		if isDeepScreen(m.screen) {
			m.cancelDiagnostics()
			m.cancelHistoryQuery()
			m.cancelLogsStream()
			m.screen = screenDashboard
			m.request++
		} else if m.screen == screenDashboard {
			m.cancelDashboardWorkspace()
			m.screen = screenFleet
		} else if m.screen == screenFleet {
			m.ensureFleet()
			m.fleet.filter = fleetFilter{}
			m.selectNearestVisible()
		}
	case "q":
		// Экраны с вводом текста (поиск, чат, фильтры) разбирают клавиши
		// раньше, поэтому здесь «q» уже не может попасть в набираемый текст.
		m.closeSubscription()
		return m, tea.Quit
	}
	return m, nil
}

// refreshCurrentScreen — «обновить сейчас» по макету: принудительный перезапрос
// данных активного экрана. Логи и история перехватывают «r» своими
// обработчиками раньше глобального, поэтому их здесь нет. Список хостов
// опрашивает коллектор по своему таймеру — руками обновлять нечего, кроме
// раскрытой карточки.
func (m *Model) refreshCurrentScreen() tea.Cmd {
	switch m.screen {
	case screenDashboard:
		return m.startDashboardWorkspace()
	case screenProcesses, screenPorts, screenContainers:
		return m.startDiagnostics()
	case screenFleet:
		if m.fleet.expanded {
			return m.startFleetCardUnits()
		}
	}
	return nil
}

func dashboardDestination(key string) screenKind {
	switch key {
	case "p":
		return screenProcesses
	case "o":
		return screenPorts
	case "d":
		return screenContainers
	default:
		return screenDashboard
	}
}

func isDeepScreen(kind screenKind) bool {
	return kind >= screenProcesses && kind <= screenContainers
}
