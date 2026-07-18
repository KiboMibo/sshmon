package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

func TestFleetRenderAdaptsPreviewAndHasNoTabs(t *testing.T) {
	// Given an online selected server with metrics and a problem.
	now := time.Now().Add(-7 * time.Second)
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{{Name: "web", Group: "prod", Online: true, Time: now, Hostname: "web-01", CPUPct: 42, MemPct: 51, Load1: 1.2}}, Issues: []collect.Issue{{Server: "web", Severity: "warn", Msg: "CPU"}}}
	m := Model{screen: screenFleet, snapshot: snapshot, config: &config.Config{Servers: []config.Server{{Name: "web", Host: "10.0.0.1", Group: "prod"}}}}
	// When rendered at wide and narrow sizes.
	m.layout = newLayout(120, 30)
	wide := m.View()
	m.layout = newLayout(80, 24)
	narrow := m.View()
	// Then wide includes preview, narrow hides it, and old tabs are absent.
	if !strings.Contains(wide, "web-01") || !strings.Contains(wide, "42%") || strings.Contains(narrow, "web-01") {
		t.Fatalf("wide=%q narrow=%q", wide, narrow)
	}
	if strings.Contains(wide, "1:Overview") || !strings.Contains(wide, "●") || !strings.Contains(wide, "7s") {
		t.Fatalf("fleet view = %q", wide)
	}
}

func TestFleetKeysClampAndToggleFilters(t *testing.T) {
	// Given a Fleet containing three grouped servers.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Servers: []collect.Metrics{{Name: "a", Group: "prod"}, {Name: "b", Group: "data"}, {Name: "c", Group: "prod"}}}}
	// When paging, cycling groups, toggling problems and preview.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	m, _ = updateModel(t, m, key("g"))
	m, _ = updateModel(t, m, key("!"))
	m, _ = updateModel(t, m, key("v"))
	// Then navigation is clamped and Fleet state changes deterministically.
	if m.selected != 2 || m.fleet.filter.Group != "prod" || !m.fleet.filter.ProblemsOnly || m.fleet.preview {
		t.Fatalf("selected=%d filter=%+v preview=%v", m.selected, m.fleet.filter, m.fleet.preview)
	}
}
