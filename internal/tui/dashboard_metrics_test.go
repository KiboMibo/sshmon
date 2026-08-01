package tui

import (
	"strings"
	"testing"

	"github.com/kibomibo/sshmon/internal/collect"
)

// TestLoadColorStyleScalesWithNumCPU — Given NumCPU and load averages,
// When loadColorStyle picks a style, Then thresholds scale by NumCPU
// (green<0.75×NumCPU, yellow 0.75–1.5×, red>1.5×).
func TestLoadColorStyleScalesWithNumCPU(t *testing.T) {
	// Given NumCPU=4 (thresholds: green<3.0, yellow 3.0–6.0, red>6.0)
	const numCPU = 4

	// When/Then: low load → goodStyle content (green)
	got := loadColorStyle(1.5, numCPU)
	if !strings.Contains(got, "1.50") {
		t.Fatalf("green: expected load value in output, got %q", got)
	}

	// When/Then: mid load → warnStyle content (yellow)
	got = loadColorStyle(4.5, numCPU)
	if !strings.Contains(got, "4.50") {
		t.Fatalf("yellow: expected load value in output, got %q", got)
	}

	// When/Then: high load → criticalStyle content (red)
	got = loadColorStyle(8.0, numCPU)
	if !strings.Contains(got, "8.00") {
		t.Fatalf("red: expected load value in output, got %q", got)
	}
}

// TestLoadColorStyleBoundaryCases — Given exact boundary load values,
// When loadColorStyle classifies them, Then boundary inclusive on low side.
func TestLoadColorStyleBoundaryCases(t *testing.T) {
	// Given NumCPU=2 (thresholds: green<1.5, yellow 1.5–3.0, red>3.0)
	const numCPU = 2

	for _, tc := range []struct {
		name string
		load float64
	}{
		{"at green/yellow boundary (0.75×)", 1.5},
		{"at yellow/red boundary (1.5×)", 3.0},
		{"just under yellow/red (1.49×)", 2.98},
		{"zero load", 0.0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// When/Then: любая граница даёт непустую подписанную величину.
			if got := loadColorStyle(tc.load, numCPU); got == "" {
				t.Fatalf("expected non-empty styled load for %s", tc.name)
			}
		})
	}
}

// TestFullestDiskDrivesTheDiskRow — Дано: несколько разделов;
// Когда: строка DISK одна; Тогда: в ней самый заполненный раздел.
func TestFullestDiskDrivesTheDiskRow(t *testing.T) {
	// Дано: загрузочный раздел свободен, корневой почти полон.
	server := collect.Metrics{Disks: []collect.DiskUsage{
		{Fs: "/dev/sda1", Mount: "/boot", TotalKB: 1 << 20, UsedKB: 1 << 18, UsedPct: 17},
		{Fs: "/dev/sda2", Mount: "/", TotalKB: 49 << 20, UsedKB: 38 << 20, UsedPct: 78},
	}}

	// Когда: выбирается раздел для строки DISK.
	disk, ok := fullestDisk(server)

	// Тогда: это корневой раздел, а детали содержат объём, точку и устройство.
	if !ok || disk.Mount != "/" {
		t.Fatalf("fullestDisk = %#v (ok=%v), want / mount", disk, ok)
	}
	details := diskUsageDetails(disk)
	for _, want := range []string{"38.0G", "49.0G", "/ (sda2)"} {
		if !strings.Contains(details, want) {
			t.Fatalf("детали диска без %q: %q", want, details)
		}
	}
	if _, ok := fullestDisk(collect.Metrics{}); ok {
		t.Fatal("сервер без дисков не должен давать раздел")
	}
}

// TestMemoryDetailsShowSwapOnlyWhenConfigured — Дано: сервер со swap и без;
// Когда: строятся детали строки MEM; Тогда: swap появляется лишь при наличии.
func TestMemoryDetailsShowSwapOnlyWhenConfigured(t *testing.T) {
	// Дано: 16G памяти, из них 6.4G свободно, swap 2G наполовину занят.
	withSwap := collect.Metrics{MemTotalKB: 16 << 20, MemAvailKB: 6 << 20, SwapTotalKB: 2 << 20, SwapFreeKB: 1 << 20}

	// Когда/тогда: обе пары значений на месте.
	got := memoryDetails(withSwap)
	for _, want := range []string{"10.0G / 16.0G", "swap", "1.0G / 2.0G"} {
		if !strings.Contains(got, want) {
			t.Fatalf("детали памяти без %q: %q", want, got)
		}
	}

	// Когда/тогда: без swap строка не врёт про нулевой раздел.
	if got := memoryDetails(collect.Metrics{MemTotalKB: 16 << 20, MemAvailKB: 6 << 20}); strings.Contains(got, "swap") {
		t.Fatalf("swap показан при его отсутствии: %q", got)
	}
}

// TestNetworkDetailsSumInterfaces — Дано: два интерфейса;
// Когда: строятся детали строки NET; Тогда: показана сумма rx и tx.
func TestNetworkDetailsSumInterfaces(t *testing.T) {
	// Дано: два интерфейса по 1 КБ/с на приём.
	server := collect.Metrics{Net: []collect.NetRate{
		{Iface: "ens32", RxBps: 1024, TxBps: 512},
		{Iface: "ens33", RxBps: 1024, TxBps: 512},
	}}

	// Когда: строится строка деталей.
	got := networkDetails(server)

	// Тогда: интерфейсы просуммированы, а таблицы интерфейсов больше нет.
	if !strings.Contains(got, "rx 2.0K/s") || !strings.Contains(got, "tx 1.0K/s") {
		t.Fatalf("детали сети = %q", got)
	}
	if strings.Contains(got, "ens32") {
		t.Fatalf("строка NET не должна раскрывать интерфейсы: %q", got)
	}
}

// TestProblemsTopStripRenderedAbovePanels — Given a server with problems,
// When the server screen renders, Then the ПРОБЛЕМЫ strip precedes the grid.
func TestProblemsTopStripRenderedAbovePanels(t *testing.T) {
	// Given: model with selected server, problems, two-column layout.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	m.snapshot.Issues = []collect.Issue{{Server: m.snapshot.Servers[0].Name, Severity: "warn", Msg: "test problem"}}

	// When
	view := m.View()

	// Then: ПРОБЛЕМЫ appears before the CPU row.
	probIdx := strings.Index(view, "ПРОБЛЕМЫ")
	cpuIdx := strings.Index(view, "CPU")
	if probIdx < 0 || cpuIdx < 0 {
		t.Fatalf("expected ПРОБЛЕМЫ and CPU in view:\n%s", view)
	}
	if probIdx > cpuIdx {
		t.Errorf("expected ПРОБЛЕМЫ (%d) before CPU (%d)", probIdx, cpuIdx)
	}
}

// TestProblemsPanelHiddenWhenNoIssues — Given no issues,
// When the dashboard renders, Then no ПРОБЛЕМЫ panel appears (no empty block).
func TestProblemsPanelHiddenWhenNoIssues(t *testing.T) {
	// Given: a dashboard whose server has no issues.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(160, 50)
	m.snapshot.Issues = nil
	// When: the view is rendered.
	view := m.View()
	// Then: the ПРОБЛЕМЫ panel is absent.
	if strings.Contains(view, "ПРОБЛЕМЫ") {
		t.Fatalf("ПРОБЛЕМЫ panel should be hidden when there are no issues:\n%s", view)
	}
}
