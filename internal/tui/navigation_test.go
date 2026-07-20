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

	// Given a server Dashboard, plain h and l are freed for navigation, not history/logs.
	for _, k := range []string{"h", "l"} {
		m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web")}
		m, _ = updateModel(t, m, key(k))
		if m.screen != screenDashboard {
			t.Fatalf("plain %q changed dashboard screen to %v", k, m.screen)
		}
	}

	// Given Fleet instead of Dashboard.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
	// When a dashboard-only shortcut is pressed.
	m, _ = updateModel(t, m, key("p"))
	// Then Fleet remains active.
	if m.screen != screenFleet {
		t.Fatalf("fleet shortcut changed screen to %v", m.screen)
	}
}

func TestOverlayTakesEscapeAndFleetOwnsQuit(t *testing.T) {
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
