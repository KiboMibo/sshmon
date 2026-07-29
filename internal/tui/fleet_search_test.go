package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kibomibo/sshmon/internal/collect"
)

func searchTestModel() Model {
	return Model{screen: screenFleet, layout: newLayout(100, 30), snapshot: collect.Snapshot{Servers: []collect.Metrics{
		{Name: "vm-prod-emauth", Group: "vm"},
		{Name: "vm-prod-emarb", Group: "vm"},
		{Name: "rteam-web", Group: "rteam"},
	}}}
}

func TestFleetSearchFiltersWhileTyping(t *testing.T) {
	// Given a fleet listing servers from two groups.
	m := searchTestModel()
	// When the search is opened and a query is typed character by character.
	m, _ = updateModel(t, m, key("/"))
	m, _ = updateModel(t, m, key("e"))
	m, _ = updateModel(t, m, key("m"))
	// Then the filter applies immediately and the context line echoes the query.
	if !m.fleet.searching || m.fleet.filter.Query != "em" {
		t.Fatalf("searching=%v query=%q", m.fleet.searching, m.fleet.filter.Query)
	}
	if view := m.View(); !strings.Contains(view, "поиск > em") {
		t.Fatalf("fleet view = %q", view)
	}
}

func TestFleetSearchEnterKeepsQueryAndEscapeClearsIt(t *testing.T) {
	// Given a typed search query.
	m := searchTestModel()
	m, _ = updateModel(t, m, key("/"))
	m, _ = updateModel(t, m, key("e"))
	// When the query is confirmed with enter.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	// Then typing stops but the filter survives.
	if m.fleet.searching || m.fleet.filter.Query != "e" {
		t.Fatalf("after enter: searching=%v query=%q", m.fleet.searching, m.fleet.filter.Query)
	}
	// When the search is reopened and cancelled with escape.
	m, _ = updateModel(t, m, key("/"))
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	// Then the filter is dropped and the full list is back.
	if m.fleet.searching || m.fleet.filter.Query != "" {
		t.Fatalf("after esc: searching=%v query=%q", m.fleet.searching, m.fleet.filter.Query)
	}
}
