package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestDashboardFitsMandatorySectionsAtMinimumSize(t *testing.T) {
	// Given: an online server with every core metric and an active issue.
	now := time.Date(2026, 7, 18, 22, 0, 0, 0, time.UTC)
	m := Model{
		screen:   screenDashboard,
		selected: 0,
		snapshot: collect.Snapshot{
			Time: now,
			Servers: []collect.Metrics{{
				Name: "web", Hostname: "web-01", Online: true, Time: now.Add(-7 * time.Second),
				Uptime: 25 * time.Hour, NumCPU: 8, CPUPct: 42, Load1: 1.2, Load5: 1.1, Load15: 0.9,
				MemTotalKB: 8 << 20, MemAvailKB: 3 << 20, MemPct: 62, SwapTotalKB: 2 << 20, SwapFreeKB: 1 << 20,
				Disks: []collect.DiskUsage{{Mount: "/", UsedPct: 71, UsedKB: 71 << 20, TotalKB: 100 << 20}},
				IO:    []collect.DiskIO{{Dev: "sda", ReadBps: 2048, WriteBps: 4096}},
				Net:   []collect.NetRate{{Iface: "eth0", RxBps: 8192, TxBps: 4096}},
			}},
			Issues: []collect.Issue{{Server: "web", Severity: "warn", Msg: "disk high"}},
		},
	}

	// When: the dashboard is rendered at its minimum supported size.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.(Model).View()

	// Then: every mandatory panel and every deep-screen hint fits in the viewport.
	for _, want := range []string{"CPU", "LOAD", "ПАМЯТЬ", "SWAP", "СЕТЬ", "ДИСКИ / IO", "ПРОБЛЕМЫ", "p процессы", "o порты", "h история", "l логи", "d контейнеры"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, view)
		}
	}
	if lines := strings.Count(view, "\n") + 1; lines > 24 {
		t.Fatalf("dashboard uses %d lines at height 24:\n%s", lines, view)
	}
}

func TestDashboardRetainsLastMetricsWhenServerIsOfflineOrStale(t *testing.T) {
	// Given: an offline server whose last successful metrics are two minutes old.
	now := time.Date(2026, 7, 18, 22, 0, 0, 0, time.UTC)
	m := Model{
		screen: screenDashboard,
		snapshot: collect.Snapshot{Time: now, Servers: []collect.Metrics{{
			Name: "db", Hostname: "db-01", Online: false, Err: "dial timeout", Time: now.Add(-2 * time.Minute),
			CPUPct: 38, MemPct: 55, Load1: 0.7,
		}}},
	}

	// When: the dashboard is rendered and an unrelated history sink is unavailable.
	m.snapshot.HistoryErr = "database locked"
	m.layout = newLayout(100, 24)
	view := m.View()

	// Then: the server is offline but its last values and age remain visible.
	for _, want := range []string{"НЕДОСТУПЕН", "dial timeout", "38%", "55%", "2m"} {
		if !strings.Contains(view, want) {
			t.Fatalf("offline dashboard missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "сервер недоступен: database locked") {
		t.Fatalf("history subfeature error was presented as server outage:\n%s", view)
	}
}
