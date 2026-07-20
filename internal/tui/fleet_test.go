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
	if strings.Contains(wide, "1:Overview") || !strings.Contains(wide, "●") {
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
	// Then navigation follows grouped display order and Fleet state changes deterministically.
	if m.selected != 0 || m.fleet.filter.Group != "prod" || !m.fleet.filter.ProblemsOnly || m.fleet.preview {
		t.Fatalf("selected=%d filter=%+v preview=%v", m.selected, m.fleet.filter, m.fleet.preview)
	}
}

func TestFleetGroupsServersUnderExplicitHeadings(t *testing.T) {
	// Given servers from two groups interleaved in config order.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Servers: []collect.Metrics{{Name: "alpha", Group: "prod"}, {Name: "bravo", Group: "data"}, {Name: "charlie", Group: "prod"}}}}
	m.layout = newLayout(80, 24)
	// When the Fleet screen is rendered.
	view := m.View()
	// Then each group heading appears exactly once and rows follow grouped order.
	if strings.Count(view, "prod") != 1 || strings.Count(view, "data") != 1 {
		t.Fatalf("group headings duplicated or missing: %q", view)
	}
	prod, alpha, charlie, data, bravo := strings.Index(view, "prod"), strings.Index(view, "alpha"), strings.Index(view, "charlie"), strings.Index(view, "data"), strings.Index(view, "bravo")
	if !(prod < alpha && alpha < charlie && charlie < data && data < bravo) {
		t.Fatalf("grouped order broken: prod=%d alpha=%d charlie=%d data=%d bravo=%d", prod, alpha, charlie, data, bravo)
	}
}

func TestFleetRowShowsServerUptimeInsteadOfDataAge(t *testing.T) {
	// Given an online server reporting a multi-day uptime.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{{Name: "web", Group: "prod", Online: true, Time: time.Now().Add(-7 * time.Second), Uptime: 50*time.Hour + 30*time.Minute}}}}
	m.layout = newLayout(80, 24)
	// When the Fleet screen is rendered.
	view := m.View()
	// Then the table shows the server uptime column instead of the sample age.
	if !strings.Contains(view, "UPTIME") || !strings.Contains(view, "2d2h") || strings.Contains(view, "ВОЗРАСТ") {
		t.Fatalf("fleet view = %q", view)
	}
}

func TestFleetNavigationFollowsGroupedDisplayOrder(t *testing.T) {
	// Given interleaved groups so config order differs from display order.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Servers: []collect.Metrics{{Name: "alpha", Group: "prod"}, {Name: "bravo", Group: "data"}, {Name: "charlie", Group: "prod"}}}}
	// When moving down from the first displayed server.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	// Then selection lands on the next server in grouped display order.
	if m.selected != 2 {
		t.Fatalf("selected=%d, want charlie (2) as next in grouped order", m.selected)
	}
}

func TestFleetTableWidthTakesSixtyFivePercentWithFloor(t *testing.T) {
	// Given wide and minimal layout widths.
	// When computing the fleet table width.
	// Then the table takes 65%% of the width with a floor of 42 cells.
	if got := fleetTableWidth(120); got != 78 {
		t.Fatalf("fleetTableWidth(120)=%d, want 78", got)
	}
	if got := fleetTableWidth(60); got != 42 {
		t.Fatalf("fleetTableWidth(60)=%d, want 42", got)
	}
}
