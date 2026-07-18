package setup

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

func manyHosts(n int) []config.SSHHost {
	hosts := make([]config.SSHHost, 0, n)
	for i := 0; i < n; i++ {
		hosts = append(hosts, config.SSHHost{
			Alias:      fmt.Sprintf("host-%02d", i),
			HostName:   fmt.Sprintf("10.0.0.%d", i+1),
			Group:      "prod",
			SourcePath: "/ssh/prod.conf",
			Position:   i,
		})
	}
	return hosts
}

func TestViewportScrollsWithCursor(t *testing.T) {
	// Given: one expanded source with 30 hosts in a 50x8 terminal.
	m := newModel(manyHosts(30))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.WindowSizeMsg{Width: 50, Height: 8})

	// When: the cursor moves 20 rows down.
	for i := 0; i < 20; i++ {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}

	// Then: the viewport height fits the terminal and follows the cursor.
	if m.viewport.Height != 5 {
		t.Fatalf("viewport.Height=%d, want 5 (height 8 - chrome 3)", m.viewport.Height)
	}
	if m.viewport.YOffset == 0 {
		t.Fatal("viewport did not scroll with cursor")
	}
	if m.cursor < m.viewport.YOffset || m.cursor >= m.viewport.YOffset+m.viewport.Height {
		t.Fatalf("cursor %d outside viewport window [%d,%d)", m.cursor, m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height)
	}
}

func TestResizeKeepsTinyTerminalUsable(t *testing.T) {
	// Given: an expanded source shown in a 1-row terminal.
	m := newModel(manyHosts(5))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.WindowSizeMsg{Width: 30, Height: 1})

	// Then: the viewport keeps a positive height.
	if m.viewport.Height < 1 {
		t.Fatalf("viewport.Height=%d, want >= 1", m.viewport.Height)
	}

	// When: the cancel key is pressed.
	next, cmd := m.Update(runeKey('q'))
	out := next.(model)

	// Then: the picker still aborts and quits.
	if !out.abort || cmd == nil {
		t.Fatalf("abort=%v cmd=%v, want abort and quit", out.abort, cmd)
	}
}

func TestResizeAfterCollapseKeepsCursorValid(t *testing.T) {
	// Given: an expanded source with the cursor on a deep host row.
	m := newModel(manyHosts(10))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.WindowSizeMsg{Width: 40, Height: 6})
	for i := 0; i < 8; i++ {
		m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}

	// When: left moves to the parent and left again collapses it.
	m = press(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyLeft})

	// Then: only the source row remains and the cursor stays in range.
	if len(m.visible) != 1 {
		t.Fatalf("visible=%d after collapse, want 1", len(m.visible))
	}
	if m.cursor != 0 {
		t.Fatalf("cursor=%d after collapse, want 0", m.cursor)
	}
	if m.viewport.YOffset != 0 {
		t.Fatalf("YOffset=%d after collapse, want 0", m.viewport.YOffset)
	}
}

func TestRenderRowsFitWidthWithoutWrapping(t *testing.T) {
	// Given: an expanded source holding a long Unicode alias and an IPv6 host.
	m := newModel([]config.SSHHost{
		{
			Alias:      "очень-длинный-псевдоним-сервера-юникод",
			HostName:   "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			User:       "deploy",
			Group:      "prod",
			SourcePath: "/ssh/prod.conf",
			Position:   0,
		},
	})
	m.toggleExpanded(0)

	// When: rows render into a 24-column terminal.
	out := m.renderRows(24)

	// Then: each visible row stays on one line within the width.
	lines := strings.Split(out, "\n")
	if len(lines) != len(m.visible) {
		t.Fatalf("lines=%d visible=%d, want equal", len(lines), len(m.visible))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 24 {
			t.Fatalf("line %d width=%d exceeds 24: %q", i, w, line)
		}
	}
}

func integrationHosts() []config.SSHHost {
	return []config.SSHHost{
		{Alias: "root-a", HostName: "10.0.0.1", Group: "main", SourcePath: "/home/u/.ssh/config", Position: 0},
		{Alias: "root-b", HostName: "10.0.0.2", Group: "main", SourcePath: "/home/u/.ssh/config", Position: 1},
		{Alias: "prod-a", HostName: "10.0.1.1", Group: "prod", SourcePath: "/home/u/.ssh/conf.d/prod.conf", Position: 0},
		{Alias: "prod-b", HostName: "10.0.1.2", Group: "prod", SourcePath: "/home/u/.ssh/conf.d/prod.conf", Position: 1},
	}
}

func TestSelectedServersCarryMainAndIncludeGroups(t *testing.T) {
	// Given: a main source with two hosts and a prod source with two hosts.
	m := newModel(integrationHosts())

	// When: the whole main source is selected and one prod host is picked.
	m.toggleSource(0)
	m.toggleExpanded(1)
	m.sources[1].hosts[0].selected = true

	// Then: converted servers keep input order and main/Include groups.
	servers := config.HostsToServers(m.selectedHosts())
	if len(servers) != 3 {
		t.Fatalf("servers=%d, want 3", len(servers))
	}
	if servers[0].Group != "main" || servers[1].Group != "main" || servers[2].Group != "prod" {
		t.Fatalf("groups=%q,%q,%q", servers[0].Group, servers[1].Group, servers[2].Group)
	}
	if servers[0].Name != "root-a" || servers[1].Name != "root-b" || servers[2].Name != "prod-a" {
		t.Fatalf("names=%q,%q,%q", servers[0].Name, servers[1].Name, servers[2].Name)
	}
}

func TestCancelAbortsDespiteSelection(t *testing.T) {
	// Given: a model with every host selected.
	m := newModel(integrationHosts())
	m.toggleAll()

	// When: the cancel key is pressed.
	m = press(t, m, runeKey('q'))

	// Then: the model aborts, which makes Run return nil before any write.
	if !m.abort {
		t.Fatal("q must abort even with selected hosts")
	}
	if m.done {
		t.Fatal("cancel must not mark the model as done")
	}
}

func TestSolidCheckboxGlyphs(t *testing.T) {
	// Given: every supported selection state.
	cases := []struct {
		state checkState
		want  string
	}{
		{state: stateEmpty, want: "□"},
		{state: statePartial, want: "▨"},
		{state: stateChecked, want: "■"},
	}

	// When/Then: each state renders as a high-contrast solid glyph.
	for _, tc := range cases {
		if got := checkGlyph(tc.state); got != tc.want {
			t.Errorf("checkGlyph(%v)=%q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestRenderRowsHighlightsCheckedAndPartialSelections(t *testing.T) {
	// Given: an expanded source with one of two hosts selected.
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile) })
	m := newModel([]config.SSHHost{
		{Alias: "prod-a", HostName: "10.0.1.1", Group: "prod", SourcePath: "/ssh/prod.conf", Position: 0},
		{Alias: "prod-b", HostName: "10.0.1.2", Group: "prod", SourcePath: "/ssh/prod.conf", Position: 1},
	})
	m.toggleExpanded(0)
	m.sources[0].hosts[0].selected = true

	// When: partially selected source and checked host rows are rendered.
	out := m.renderRows(80)

	// Then: both states use bold colored styles, not only thin glyph changes.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("renderRows has no ANSI highlight: %q", out)
	}
	if !stySelected.GetBold() || stySelected.GetForeground() != lipgloss.Color("42") {
		t.Fatalf("selected style=%v bold=%v, want green 42 bold", stySelected.GetForeground(), stySelected.GetBold())
	}
	if !styPartial.GetBold() || styPartial.GetForeground() != lipgloss.Color("214") {
		t.Fatalf("partial style=%v bold=%v, want yellow 214 bold", styPartial.GetForeground(), styPartial.GetBold())
	}
}
