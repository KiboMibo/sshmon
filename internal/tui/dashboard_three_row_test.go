package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kibomibo/sshmon/internal/collect"
)

func TestDashboardWidePanelsNeverOverflowTerminalWidth(t *testing.T) {
	// Given a wide dashboard with loaded Docker, systemd, and log data at several terminal sizes.
	for _, size := range []struct{ w, h int }{{120, 30}, {160, 40}} {
		m := dashboardWorkspaceFixture()
		m.dashboard.containers.items = []collect.Container{{Name: "api", Status: "Up", CPUPct: 3, MemPct: 4}}
		m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: size.w, Height: size.h})

		// When the bordered dashboard is rendered.
		view := m.View()

		// Then every visual line fits the terminal width and panel borders are drawn.
		for i, line := range strings.Split(view, "\n") {
			if width := lipgloss.Width(line); width > size.w {
				t.Fatalf("%dx%d line %d width = %d > %d: %q", size.w, size.h, i, width, size.w, line)
			}
		}
		for _, glyph := range []string{"╭", "╮", "╰", "╯"} {
			if !strings.Contains(view, glyph) {
				t.Fatalf("%dx%d view misses panel border glyph %q:\n%s", size.w, size.h, glyph, view)
			}
		}
	}
}

func TestDashboardThreeRowWideLayoutShowsWorkspacePanels(t *testing.T) {
	// Given a wide dashboard with metrics and loaded Docker, systemd, and log data.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	m.dashboard.containers.items = []collect.Container{{Name: "api", Status: "Up", CPUPct: 3, MemPct: 4}}

	// When the dashboard is rendered.
	view := m.View()

	// Then all three workspace rows expose their operational data and controls.
	for _, want := range []string{"CPU", "ДИСКИ / IO", "ПРОБЛЕМЫ", "DOCKER", "api", "СЕТЬ", "SYSTEMD", "sshd.service", "ЛОГИ · SYSTEM", "system ready", "f фильтр", "x системный лог"} {
		if !strings.Contains(view, want) {
			t.Fatalf("wide dashboard missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardThreeRowNarrowLayoutStacksPanelsInSemanticOrder(t *testing.T) {
	// Given a narrow dashboard whose Docker source is unavailable.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(80, 24)
	m.dashboard.containers.status = diagnosticsUnsupported

	// When the dashboard is rendered.
	view := m.View()

	// Then panels stack metrics, Docker, network, systemd, and logs without exceeding the terminal.
	sections := []string{"CPU", "DOCKER NOT RUNNING", "СЕТЬ", "SYSTEMD", "ЛОГИ · SYSTEM"}
	previous := -1
	for _, section := range sections {
		position := strings.Index(view, section)
		if position <= previous {
			t.Fatalf("section %q is missing or out of order:\n%s", section, view)
		}
		previous = position
	}
	if lines := strings.Count(view, "\n") + 1; lines > 24 {
		t.Fatalf("narrow dashboard uses %d lines:\n%s", lines, view)
	}
}

func TestDashboardUnitFilterNavigationAndJournalSelection(t *testing.T) {
	// Given a dashboard with three running services and a recording log source.
	source := &fakeDashboardSource{lines: []string{"journal line"}}
	m := dashboardWorkspaceFixture()
	m.dashboardSource = source
	m.dashboard.units.items = []collect.SystemdUnit{{Name: "nginx.service"}, {Name: "sshd.service"}, {Name: "cron.service"}}

	// When the operator filters services, moves once, and opens the selected journal.
	m, _ = updateModel(t, m, key("f"))
	for _, value := range []string{".", "s", "e", "r", "v", "i", "c", "e"} {
		m, _ = updateModel(t, m, key(value))
	}
	m, _ = updateModel(t, m, key("enter"))
	m, _ = updateModel(t, m, key("j"))
	m, cmd := updateModel(t, m, key("enter"))
	if cmd == nil {
		t.Fatal("journal selection did not start a snapshot request")
	}
	m, followup := updateModel(t, m, cmd())

	// Then the second filtered unit is selected through one static 50-line journal request.
	if followup != nil || len(source.logRequests) != 1 {
		t.Fatalf("followup=%v requests=%d", followup, len(source.logRequests))
	}
	request := source.logRequests[0]
	if request.Source.Kind != collect.LogJournal || request.Source.Name != "sshd.service" || !slices.Equal(source.logLines, []int{50}) {
		t.Fatalf("request=%#v lines=%#v", request, source.logLines)
	}
	if !strings.Contains(m.View(), "ЛОГИ · sshd.service") {
		t.Fatalf("selected journal is not identified:\n%s", m.View())
	}
}

func TestDashboardUnitClearReturnsToSystemLog(t *testing.T) {
	// Given a dashboard showing a selected service journal and an active unit filter.
	source := &fakeDashboardSource{lines: []string{"system line"}}
	m := dashboardWorkspaceFixture()
	m.dashboardSource = source
	m.dashboard.logs.source = collect.LogSource{Kind: collect.LogJournal, Name: "nginx.service"}
	m, _ = updateModel(t, m, key("f"))
	m, _ = updateModel(t, m, key("n"))
	m, _ = updateModel(t, m, key("enter"))

	// When the operator clears the unit selection.
	m, cmd := updateModel(t, m, key("x"))
	if cmd == nil {
		t.Fatal("clear selection did not start the system snapshot")
	}
	m, followup := updateModel(t, m, cmd())

	// Then the filter is cleared and logs return through one static system request.
	if followup != nil || len(source.logRequests) != 1 || source.logRequests[0].Source.Kind != collect.LogSystem {
		t.Fatalf("followup=%v requests=%#v", followup, source.logRequests)
	}
	view := m.View()
	if !strings.Contains(view, "ЛОГИ · SYSTEM") || strings.Contains(view, "фильтр: n") {
		t.Fatalf("system reset did not clear unit filter:\n%s", view)
	}
}

func dashboardWorkspaceFixture() Model {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	return Model{
		screen:   screenDashboard,
		selected: 0,
		snapshot: collect.Snapshot{Time: now, Servers: []collect.Metrics{{
			Name: "web", Hostname: "web-01", Online: true, Time: now, Uptime: 25 * time.Hour,
			NumCPU: 4, CPUPct: 25, Load1: 0.4, Load5: 0.3, Load15: 0.2,
			MemTotalKB: 4 << 20, MemAvailKB: 2 << 20, MemPct: 50,
			Disks: []collect.DiskUsage{{Mount: "/", UsedPct: 60, AvailKB: 20 << 20}},
			IO:    []collect.DiskIO{{Dev: "sda", ReadBps: 1024, WriteBps: 2048}},
			Net:   []collect.NetRate{{Iface: "eth0", RxBps: 4096, TxBps: 2048}},
		}}},
		dashboard: dashboardWorkspace{
			tileFocus: tileSystemd,
			units:     dashboardUnitsState{items: []collect.SystemdUnit{{Name: "sshd.service", Active: "active", Sub: "running"}}, status: diagnosticsReady},
			logs:      dashboardLogState{lines: []string{"system ready"}, source: collect.LogSource{Kind: collect.LogSystem}, status: diagnosticsReady},
		},
	}
}
