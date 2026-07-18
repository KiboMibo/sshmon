package setup

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/config"
)

func setupHosts() []config.SSHHost {
	return []config.SSHHost{
		{Alias: "root-a", HostName: "10.0.0.1", Group: "main", SourcePath: "/home/u/.ssh/config", Position: 0},
		{Alias: "prod-a", HostName: "10.0.1.1", Group: "prod", SourcePath: "/home/u/.ssh/a/prod.conf", Position: 0},
		{Alias: "prod-b", HostName: "10.0.2.1", Group: "prod", SourcePath: "/home/u/.ssh/b/prod.conf", Position: 0},
	}
}

func press(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, _ := m.Update(msg)
	out, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", next)
	}
	return out
}

func runeKey(r rune) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestModelStartsWithCollapsedSourcesInInputOrder(t *testing.T) {
	// Given: hosts from three ordered source files.
	hosts := setupHosts()

	// When: the setup model is created.
	m := newModel(hosts)

	// Then: only three distinct source rows are visible and all are collapsed.
	if len(m.sources) != 3 || len(m.visible) != 3 {
		t.Fatalf("sources=%d visible=%d, want 3/3", len(m.sources), len(m.visible))
	}
	for _, source := range m.sources {
		if source.expanded {
			t.Fatalf("source %q starts expanded", source.path)
		}
	}
	if m.sources[0].group != "main" || m.sources[1].group != "prod" || m.sources[2].group != "prod" {
		t.Fatalf("source groups must keep input order: %+v", m.sources)
	}
}

func TestSourceSelectionCyclesThroughCheckedAndPartialStates(t *testing.T) {
	// Given: one source with two hosts.
	m := newModel([]config.SSHHost{
		{Alias: "a", Group: "prod", SourcePath: "/ssh/prod.conf", Position: 0},
		{Alias: "b", Group: "prod", SourcePath: "/ssh/prod.conf", Position: 1},
	})

	// When: the source is toggled, expanded, then one child is toggled off.
	m.toggleCurrent()
	m.toggleExpanded(0)
	m.cursor = 1
	m.toggleCurrent()

	// Then: the source is partial and exactly one host remains selected.
	if got := m.sourceState(0); got != statePartial {
		t.Fatalf("state=%v, want partial", got)
	}
	if got := len(m.selectedHosts()); got != 1 {
		t.Fatalf("selected=%d, want 1", got)
	}
}

func TestSelectionSurvivesCollapseAndExpand(t *testing.T) {
	// Given: an expanded source with one selected host.
	m := newModel(setupHosts()[1:])
	m.toggleExpanded(0)
	m.cursor = 1
	m.toggleCurrent()

	// When: the source is collapsed and expanded again.
	m.toggleExpanded(0)
	m.toggleExpanded(0)

	// Then: the host selection is preserved.
	if got := len(m.selectedHosts()); got != 1 {
		t.Fatalf("selected=%d after collapse cycle, want 1", got)
	}
}

func TestSaveKeyBlockedWithoutSelection(t *testing.T) {
	// Given: a model with nothing selected.
	m := newModel(setupHosts())

	// When: the save key is pressed.
	m = press(t, m, runeKey('s'))

	// Then: saving is blocked and the picker stays open.
	if m.done || !m.saveBlocked {
		t.Fatalf("done=%v saveBlocked=%v, want false/true", m.done, m.saveBlocked)
	}
}

func TestSaveKeyQuitsWithSelection(t *testing.T) {
	// Given: a model with everything selected.
	m := newModel(setupHosts())
	m.toggleAll()

	// When: the save key is pressed.
	next, cmd := m.Update(runeKey('s'))
	out := next.(model)

	// Then: the model is done and the program quits.
	if !out.done || cmd == nil {
		t.Fatalf("done=%v cmd=%v, want done and quit", out.done, cmd)
	}
}

func TestCancelKeysAbort(t *testing.T) {
	for _, msg := range []tea.Msg{
		runeKey('q'),
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyCtrlC},
	} {
		// Given: a fresh model.
		m := newModel(setupHosts())

		// When: a cancel key is pressed.
		m = press(t, m, msg)

		// Then: the picker aborts.
		if !m.abort {
			t.Fatalf("abort=false after %v", msg)
		}
	}
}

func TestEnterExpandsSourceAndTogglesHost(t *testing.T) {
	// Given: a collapsed source with two hosts.
	m := newModel(setupHosts()[1:])

	// When: Enter is pressed on the source row.
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// Then: the source expands without selecting anything.
	if !m.sources[0].expanded || m.selectedCount() != 0 {
		t.Fatalf("expanded=%v selected=%d, want true/0", m.sources[0].expanded, m.selectedCount())
	}

	// When: Enter is pressed on the first host row.
	m.cursor = 1
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// Then: only that host is toggled.
	if m.selectedCount() != 1 {
		t.Fatalf("selected=%d after Enter on host, want 1", m.selectedCount())
	}
}

func TestLeftOnHostMovesCursorToParentSource(t *testing.T) {
	// Given: an expanded source with the cursor on a host row.
	m := newModel(setupHosts()[1:])
	m.toggleExpanded(0)
	m.cursor = 1

	// When: the left key is pressed.
	m = press(t, m, tea.KeyMsg{Type: tea.KeyLeft})

	// Then: the cursor moves to the parent source and stays expanded.
	if m.cursor != 0 || !m.sources[0].expanded {
		t.Fatalf("cursor=%d expanded=%v, want 0/true", m.cursor, m.sources[0].expanded)
	}

	// When: the left key is pressed again on the source.
	m = press(t, m, tea.KeyMsg{Type: tea.KeyLeft})

	// Then: the source collapses.
	if m.sources[0].expanded {
		t.Fatal("source still expanded after left on source row")
	}
}

func TestToggleAllSelectsAndClears(t *testing.T) {
	// Given: a model with three hosts across sources.
	m := newModel(setupHosts())

	// When: toggle-all is pressed twice.
	m = press(t, m, runeKey('a'))
	first := m.selectedCount()
	m = press(t, m, runeKey('a'))

	// Then: all hosts select, then clear.
	if first != 3 || m.selectedCount() != 0 {
		t.Fatalf("first=%d second=%d, want 3/0", first, m.selectedCount())
	}
}

func TestSelectedHostsKeepInputOrder(t *testing.T) {
	// Given: all hosts selected across three sources.
	m := newModel(setupHosts())
	m.toggleAll()

	// When: the selection is read back.
	got := m.selectedHosts()

	// Then: hosts come back in source/declaration order.
	if len(got) != 3 || got[0].Alias != "root-a" || got[1].Alias != "prod-a" || got[2].Alias != "prod-b" {
		t.Fatalf("selection order broken: %+v", got)
	}
}
