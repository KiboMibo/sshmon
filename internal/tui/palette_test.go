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

func itemLabels(items []paletteItem) []string {
	labels := make([]string, len(items))
	for index := range items {
		labels[index] = items[index].label
	}
	return labels
}
