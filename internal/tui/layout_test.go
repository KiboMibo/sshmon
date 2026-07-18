package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kibomibo/sshmon/internal/collect"
)

func TestLayoutContractsAtSupportedSizes(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		wide    bool
		message string
	}{
		{"too small", 59, 15, false, "увеличьте терминал"},
		{"minimum fleet", 60, 16, false, ""},
		{"wide fleet", 120, 30, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given Fleet at a terminal size boundary.
			m := Model{screen: screenFleet, snapshot: snapshotWithServers("web")}
			// When Bubble Tea reports the window size.
			m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			// Then the adaptive contract and resize message are deterministic.
			if m.layout.wide != tt.wide {
				t.Fatalf("wide = %v, want %v", m.layout.wide, tt.wide)
			}
			if tt.message != "" && !strings.Contains(m.View(), tt.message) {
				t.Fatalf("view = %q", m.View())
			}
		})
	}
}

func TestResizePreservesScreenAndSelection(t *testing.T) {
	// Given a selected server on Dashboard.
	m := Model{screen: screenDashboard, selected: 1, snapshot: snapshotWithServers("web", "db")}
	// When the terminal is resized across wide and narrow modes.
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	// Then the active screen and server selection survive.
	if m.screen != screenDashboard || m.selected != 1 || !strings.Contains(m.View(), "db") {
		t.Fatalf("screen=%v selected=%d view=%q", m.screen, m.selected, m.View())
	}
}

func snapshotWithServers(names ...string) collect.Snapshot {
	servers := make([]collect.Metrics, 0, len(names))
	for _, name := range names {
		servers = append(servers, collect.Metrics{Name: name, Online: true})
	}
	return collect.Snapshot{Servers: servers}
}
