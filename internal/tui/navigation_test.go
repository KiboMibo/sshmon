package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNavigationDrillsIntoServerAndReturnsToFleet(t *testing.T) {
	// Given the Fleet screen with a selected server.
	m := Model{screen: screenFleet, selected: 0, snapshot: snapshotWithServers("web")}
	// When Enter opens the server and Escape navigates back.
	m, _ = updateModel(t, m, key("enter"))
	if m.screen != screenDashboard {
		t.Fatalf("screen after enter = %v", m.screen)
	}
	m, _ = updateModel(t, m, key("esc"))
	// Then the root Fleet screen is restored.
	if m.screen != screenFleet {
		t.Fatalf("screen after escape = %v", m.screen)
	}
}

func TestDashboardShortcutsOpenOnlyDeepScreens(t *testing.T) {
	tests := []struct {
		msg  tea.KeyMsg
		want screenKind
	}{
		{key("p"), screenProcesses},
		{key("o"), screenPorts},
		{key("h"), screenHistory},
		{key("l"), screenLogs},
		{tea.KeyMsg{Type: tea.KeyCtrlH}, screenHistory},
		{tea.KeyMsg{Type: tea.KeyCtrlL}, screenLogs},
		{key("d"), screenContainers},
	}
	for _, tt := range tests {
		t.Run(tt.msg.String(), func(t *testing.T) {
			// Given a server Dashboard.
			m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web")}
			// When its diagnostic shortcut is pressed.
			m, _ = updateModel(t, m, tt.msg)
			// Then the corresponding explicit screen opens.
			if m.screen != tt.want {
				t.Fatalf("screen = %v, want %v", m.screen, tt.want)
			}
		})
	}

	// Given a server Dashboard, j and k stay tile scrolling instead of navigating away.
	for _, k := range []string{"j", "k"} {
		m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web")}
		m, _ = updateModel(t, m, key(k))
		if m.screen != screenDashboard {
			t.Fatalf("plain %q changed dashboard screen to %v", k, m.screen)
		}
	}

	// Given Fleet instead of Dashboard.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	// When a dashboard-only shortcut is pressed ("h" opens history only from the server screen).
	m, _ = updateModel(t, m, key("h"))
	// Then Fleet remains active.
	if m.screen != screenFleet {
		t.Fatalf("fleet shortcut changed screen to %v", m.screen)
	}
}

func TestFleetProcessesShortcutWorksWithoutExpandedCard(t *testing.T) {
	// Given: экран флота со свёрнутой карточкой — сайдбар обещает «p процессы».
	m := Model{
		screen:          screenFleet,
		snapshot:        snapshotWithServers("web", "db"),
		layout:          newLayout(120, 30),
		fleet:           newFleetModel(),
		dashboardSource: &fakeDashboardSource{},
		selected:        1,
	}

	// When: нажата «p».
	opened, cmd := updateModel(t, m, key("p"))

	// Then: открыт экран процессов выбранного сервера, раскрытие карточки не требуется.
	if opened.screen != screenProcesses || opened.fleet.expanded {
		t.Fatalf("screen=%v expanded=%v", opened.screen, opened.fleet.expanded)
	}
	if cmd == nil || opened.processes.status != diagnosticsLoading {
		t.Fatalf("диагностика не запущена: cmd=%v status=%v", cmd, opened.processes.status)
	}
	if opened.dashboard.server != "db" {
		t.Fatalf("workspace открыт не для выбранного сервера: %q", opened.dashboard.server)
	}
}

func TestFleetSidebarLoadsTopProcessesWhenShown(t *testing.T) {
	// Given: широкий экран флота с выключенным сайдбаром.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web"), layout: newLayout(120, 30), fleet: newFleetModel()}
	m.fleet.preview = false

	// When: сайдбар включают клавишей «v».
	shown, cmd := updateModel(t, m, key("v"))

	// Then: процессы выбранного хоста уже запрошены, а не ждут движения курсора.
	if !shown.fleet.preview {
		t.Fatalf("preview = %v", shown.fleet.preview)
	}
	if cmd == nil || shown.processes.status != diagnosticsLoading || shown.processes.cancel == nil {
		t.Fatalf("сайдбар не запросил процессы: cmd=%v status=%v", cmd, shown.processes.status)
	}

	// When: сайдбар выключают той же клавишей.
	hidden, cmd := updateModel(t, shown, key("v"))

	// Then: невидимый раздел по ssh не ходит.
	if hidden.fleet.preview || cmd != nil {
		t.Fatalf("скрытый сайдбар запросил процессы: preview=%v cmd=%v", hidden.fleet.preview, cmd)
	}

	// И первичная загрузка: размер терминала приходит первым сообщением после старта.
	fresh := Model{screen: screenFleet, snapshot: snapshotWithServers("web"), fleet: newFleetModel()}
	sized, cmd := updateModel(t, fresh, tea.WindowSizeMsg{Width: 120, Height: 30})
	if cmd == nil || sized.processes.status != diagnosticsLoading {
		t.Fatalf("первый WindowSizeMsg оставил сайдбар без данных: cmd=%v status=%v", cmd, sized.processes.status)
	}
}

func TestOverlayTakesEscapeAndQuitWorksOnEveryScreen(t *testing.T) {
	// Given Fleet with a global chat overlay.
	m := Model{screen: screenFleet}
	m, _ = updateModel(t, m, key("c"))
	if m.overlay != overlayChat {
		t.Fatalf("overlay = %v", m.overlay)
	}
	// When Escape and then q are pressed.
	m, quit := updateModel(t, m, key("esc"))
	if m.overlay != overlayNone || quit != nil {
		t.Fatalf("escape overlay=%v quit=%v", m.overlay, quit)
	}
	_, quit = updateModel(t, m, key("q"))
	// Then Escape closes only the overlay and q exits from Fleet.
	if quit == nil {
		t.Fatal("q on Fleet did not return tea.Quit")
	}

	// And ctrl+c exits from any other screen too, not just Fleet.
	for _, screen := range []screenKind{screenDashboard, screenLogs, screenHistory, screenProcesses} {
		deep := Model{screen: screen, snapshot: snapshotWithServers("web"), logs: newLogsScreen()}
		if _, quit := updateModel(t, deep, tea.KeyMsg{Type: tea.KeyCtrlC}); quit == nil {
			t.Fatalf("ctrl+c on screen %v did not return tea.Quit", screen)
		}
	}
}

func TestCtrlCQuitsWithOverlayOpenAndWhileTyping(t *testing.T) {
	// Given every overlay that owns a text input, plus the logs filter.
	overlays := []overlayKind{overlayChat, overlaySearch, overlayPalette, overlayPassphrase, overlayHelp}
	for _, kind := range overlays {
		m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
		m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		if kind == overlayPassphrase {
			m.passphrase = newPassphraseOverlay("web")
		}
		m.openOverlay(kind)

		// When ctrl+c is pressed while the overlay holds the focus.
		updated, quit := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})

		// Then the application quits instead of typing into the input.
		if quit == nil {
			t.Fatalf("ctrl+c with overlay %v did not return tea.Quit", kind)
		}
		if kind == overlaySearch && updated.search.input.Value() != "" {
			t.Fatalf("ctrl+c leaked into the search input: %q", updated.search.input.Value())
		}
	}

	// And the same holds while the logs filter is being typed.
	logs := Model{screen: screenLogs, snapshot: snapshotWithServers("web"), logs: newLogsScreen()}
	logs, _ = updateModel(t, logs, key("/"))
	if _, quit := updateModel(t, logs, tea.KeyMsg{Type: tea.KeyCtrlC}); quit == nil {
		t.Fatal("ctrl+c while filtering logs did not return tea.Quit")
	}
}

func TestEscapeReturnsDeepScreenToDashboardBeforeFleet(t *testing.T) {
	// Given a diagnostic screen.
	m := Model{screen: screenProcesses, snapshot: snapshotWithServers("web")}
	// When Escape is pressed twice.
	m, _ = updateModel(t, m, key("esc"))
	if m.screen != screenDashboard {
		t.Fatalf("first escape screen = %v", m.screen)
	}
	m, _ = updateModel(t, m, key("esc"))
	// Then navigation unwinds through Dashboard to Fleet.
	if m.screen != screenFleet {
		t.Fatalf("second escape screen = %v", m.screen)
	}
}

func updateModel(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	result, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T", updated)
	}
	return result, cmd
}

func key(value string) tea.KeyMsg {
	switch value {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
	}
}
