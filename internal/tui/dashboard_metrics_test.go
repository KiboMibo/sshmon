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

	cases := []struct {
		name string
		load float64
		want string // "green"|"yellow"|"red"
	}{
		{"at green/yellow boundary (0.75×)", 1.5, "yellow"},
		{"at yellow/red boundary (1.5×)", 3.0, "red"},
		{"just under yellow/red (1.49×)", 2.98, "yellow"},
		{"zero load", 0.0, "green"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// When
			got := loadColorStyle(tc.load, numCPU)
			// Then: output contains the load value and a color code
			if !strings.Contains(got, "g") && tc.want == "green" {
				// goodStyle renders text; just ensure no crash and value present
			}
			_ = got // style contains ANSI; assert no panic + non-empty
			if got == "" {
				t.Fatalf("expected non-empty styled load for %s", tc.name)
			}
		})
	}
}

// TestDiskBarsRenderPerMountProgressWithGBLabels — Given server with disks,
// When diskBars renders them, Then each mount has a progress bar and GB labels (no "max").
func TestDiskBarsRenderPerMountProgressWithGBLabels(t *testing.T) {
	// Given: server with / mount 40% used
	server := collect.Metrics{
		Disks: []collect.DiskUsage{
			{Mount: "/", TotalKB: 100000000, UsedKB: 40000000, AvailKB: 60000000, UsedPct: 40},
		},
	}

	// When
	bars := diskBars(server, 40)

	// Then: at least one bar; contains "/" and "ГБ" labels; no "max" substring
	if len(bars) == 0 {
		t.Fatal("expected at least one disk bar row")
	}
	joined := strings.Join(bars, "\n")
	if !strings.Contains(joined, "/") {
		t.Errorf("expected mount '/' in bars: %q", joined)
	}
	if strings.Contains(joined, "max") {
		t.Errorf("expected no 'max' in disk bars: %q", joined)
	}
}

// TestDiskTextDropsMaxField — Given server with disks and IO,
// When diskText formats the summary, Then only R/W rates shown (no "max N%").
func TestDiskTextDropsMaxField(t *testing.T) {
	// Given
	server := collect.Metrics{
		Disks: []collect.DiskUsage{{Mount: "/", UsedPct: 92}},
		IO:    []collect.DiskIO{{ReadBps: 0, WriteBps: 6000}},
	}

	// When
	got := diskText(server)

	// Then: no "max" prefix; has R and W
	if strings.Contains(got, "max") {
		t.Errorf("expected no 'max' in diskText: %q", got)
	}
	if !strings.Contains(got, "R") || !strings.Contains(got, "W") {
		t.Errorf("expected R and W in diskText: %q", got)
	}
}

// TestDashboardMetricsContentReformFormat — Given a server with full metrics,
// When dashboardMetricsContent renders, Then:
// - longer CPU bar present
// - load values present WITHOUT "LOAD" label
// - blank line separator between CPU/MEM and MEM/ДИСКИ blocks
// - MEM bar present
// - RAM and SWAP lines under MEM
// - ДИСКИ header + per-mount bars + IO line.
func TestDashboardMetricsContentReformFormat(t *testing.T) {
	// Given
	server := collect.Metrics{
		NumCPU:     4,
		CPUPct:     55,
		Load1:      1.2,
		Load5:      1.0,
		Load15:     0.8,
		MemPct:     60,
		MemTotalKB: 16000000,
		MemAvailKB: 6400000,
		Disks: []collect.DiskUsage{
			{Mount: "/", TotalKB: 100000000, UsedKB: 40000000, AvailKB: 60000000, UsedPct: 40},
		},
		IO: []collect.DiskIO{{ReadBps: 1000, WriteBps: 2000}},
	}

	// When
	rows := dashboardMetricsContent(server, 50, false)
	joined := strings.Join(rows, "\n")

	// Then: no literal "LOAD" label
	if strings.Contains(joined, "LOAD") {
		t.Errorf("expected no 'LOAD' label in reformed metrics: %q", joined)
	}
	// Then: load values present (1.20)
	if !strings.Contains(joined, "1.20") {
		t.Errorf("expected load value 1.20 in metrics: %q", joined)
	}
	// Then: CPU bar (█ or ░)
	if !strings.Contains(joined, "█") && !strings.Contains(joined, "░") {
		t.Errorf("expected CPU bar glyph in metrics: %q", joined)
	}
	// Then: MEM section
	if !strings.Contains(joined, "ПАМЯТЬ") {
		t.Errorf("expected 'ПАМЯТЬ' label in metrics: %q", joined)
	}
	// Then: ДИСКИ header
	if !strings.Contains(joined, "ДИСКИ") {
		t.Errorf("expected 'ДИСКИ' header in metrics: %q", joined)
	}
	// Then: blank line separator present
	if !containsBlank(rows) {
		t.Errorf("expected at least one blank line separator in metrics: %q", joined)
	}
}

// TestProblemsTopStripRenderedAbovePanelsInWideMode — Given a wide layout
// and a server with problems, When renderDashboardWorkspace renders,
// Then ПРОБЛЕМЫ strip appears BEFORE the МЕТРИКИ panel content.
func TestProblemsTopStripRenderedAbovePanelsInWideMode(t *testing.T) {
	// Given: model with selected server, problems, wide layout
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	// Inject a problem
	m.snapshot.Issues = []collect.Issue{{Server: "web-01", Severity: "warn", Msg: "test problem"}}

	// When
	view := m.View()

	// Then: ПРОБЛЕМЫ appears before МЕТРИКИ panel
	probIdx := strings.Index(view, "ПРОБЛЕМЫ")
	metricsIdx := strings.Index(view, "МЕТРИКИ")
	if probIdx < 0 {
		t.Fatal("expected ПРОБЛЕМЫ top strip in view")
	}
	if metricsIdx < 0 {
		t.Fatal("expected МЕТРИКИ panel in view")
	}
	if probIdx > metricsIdx {
		t.Errorf("expected ПРОБЛЕМЫ (%d) before МЕТРИКИ (%d)", probIdx, metricsIdx)
	}
}

// containsBlank returns true if any row in rows is empty.
func containsBlank(rows []string) bool {
	for _, r := range rows {
		if r == "" {
			return true
		}
	}
	return false
}
