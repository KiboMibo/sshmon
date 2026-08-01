package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
)

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

	// When the operator clears the unit selection ("s" since "x" is ssh everywhere).
	m, cmd := updateModel(t, m, key("s"))
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

func TestServerLogsTileCarriesTailStateAndHints(t *testing.T) {
	// Given a server screen with a loaded system log.
	m := serverScreenModel(120, 30)

	// When the logs tile is rendered.
	view := m.View()

	// Then the tile repeats the mockup status row next to its title.
	for _, want := range []string{"ЛОГИ · SYSTEM", "хвост включён", "l логи · s источник", "system ready"} {
		if !strings.Contains(view, want) {
			t.Fatalf("logs tile missing %q:\n%s", want, view)
		}
	}

	// And when the log request failed, the same row reports the error.
	m.dashboard.logs.err = errors.New("journalctl: no such unit")
	if !strings.Contains(m.View(), "ошибка: ") {
		t.Fatalf("logs tile hides the error state:\n%s", m.View())
	}
}

func dashboardWorkspaceFixture() Model {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	return Model{
		screen:   screenDashboard,
		selected: 0,
		layout:   newLayout(120, 30),
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
