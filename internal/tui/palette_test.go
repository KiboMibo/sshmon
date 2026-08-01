package tui

import (
	"strings"
	"testing"
)

func TestPaletteListsAvailableActionsAndServers(t *testing.T) {
	// Given Fleet with two known servers.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web", "db")}

	// When palette items are built for the current context.
	items := paletteItems(m)
	joined := strings.Join(itemLabels(items), "\n")

	// Then servers and global actions exist, but Dashboard-only actions do not.
	for _, want := range []string{"web", "db", "Чат", "Справка"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("palette misses %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "Процессы") {
		t.Fatalf("unavailable Dashboard action leaked into Fleet palette: %s", joined)
	}
}

func TestPaletteFiltersAndOpensSelectedServer(t *testing.T) {
	// Given a palette query matching one server.
	m := Model{screen: screenFleet, snapshot: snapshotWithServers("web", "db"), palette: newPaletteOverlay()}
	m.overlay = overlayPalette
	m.palette.input.SetValue("db")
	m.palette.refresh(m)

	// When the selected palette item is executed.
	m, _ = updateModel(t, m, key("enter"))

	// Then that server is selected and Dashboard opens.
	if m.overlay != overlayNone || m.screen != screenDashboard || m.selected != 1 {
		t.Fatalf("overlay=%v screen=%v selected=%d", m.overlay, m.screen, m.selected)
	}
}

func TestPaletteServerSwitchRestartsDashboardWorkspace(t *testing.T) {
	// Given a Dashboard whose workspace requests belong to the previous host.
	cancelled := 0
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web", "db"), palette: newPaletteOverlay()}
	m.dashboard.containers.cancel = func() { cancelled++ }
	m.dashboard.units.cancel = func() { cancelled++ }
	m.dashboard.logs.cancel = func() { cancelled++ }
	m.overlay = overlayPalette
	m.palette.input.SetValue("db")
	m.palette.refresh(m)

	// When another server is opened from the palette.
	m, cmd := updateModel(t, m, key("enter"))

	// Then the stale workspace is cancelled and a new one starts for the new host.
	if cancelled != 3 {
		t.Fatalf("cancelled = %d, want 3 workspace requests", cancelled)
	}
	if cmd == nil || m.selected != 1 || m.screen != screenDashboard {
		t.Fatalf("cmd=%v selected=%d screen=%v", cmd, m.selected, m.screen)
	}
}

func itemLabels(items []paletteItem) []string {
	labels := make([]string, len(items))
	for index := range items {
		labels[index] = items[index].label
	}
	return labels
}
