package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

// loadColorStyle formats a load average value with color keyed to NumCPU.
// Thresholds: green < 0.75×NumCPU, yellow 0.75–1.5×, red > 1.5×.
func loadColorStyle(load float64, numCPU int) string {
	if numCPU < 1 {
		numCPU = 1
	}
	ratio := load / float64(numCPU)
	switch {
	case ratio < 0.75:
		return goodStyle.Render(fmt.Sprintf("%.2f", load))
	case ratio <= 1.5:
		return warnStyle.Render(fmt.Sprintf("%.2f", load))
	default:
		return criticalStyle.Render(fmt.Sprintf("%.2f", load))
	}
}

// kbToGB converts kilobytes to a rounded gibibyte string.
func kbToGB(kb uint64) string {
	const kbPerGB = 1024 * 1024
	if kb == 0 {
		return "0 ГБ"
	}
	return fmt.Sprintf("%d ГБ", kb/kbPerGB)
}

// diskPctColor picks a style for a disk usage percentage.
func diskPctColor(pct float64) string {
	switch {
	case pct >= 90:
		return criticalStyle.Render(fmt.Sprintf("%.0f%%", pct))
	case pct >= 75:
		return warnStyle.Render(fmt.Sprintf("%.0f%%", pct))
	default:
		return goodStyle.Render(fmt.Sprintf("%.0f%%", pct))
	}
}

// diskBars renders per-mount progress bars with used/free GB labels.
// Each row: `<mount> <gauge> <used> / <total>`; no `max` aggregate.
func diskBars(server collect.Metrics, width int) []string {
	if len(server.Disks) == 0 {
		return []string{dimStyle.Render("диски не найдены")}
	}
	rows := make([]string, 0, len(server.Disks))
	barW := width / 3
	if barW < 8 {
		barW = 8
	}
	for _, d := range server.Disks {
		mount := d.Mount
		if len(mount) > 12 {
			mount = mount[:12]
		}
		bar := gauge(d.UsedPct, barW)
		label := fmt.Sprintf("%-12s %s %s / %s",
			mount, bar, kbToGB(d.UsedKB), kbToGB(d.TotalKB))
		rows = append(rows, fitLine(label, width))
	}
	return rows
}

// dashboardMetricsContent renders the reformed metrics panel body:
// longer CPU bar, load values (colored, no LOAD label), blank line,
// MEM bar with RAM/SWAP under it, blank line, ДИСКИ header + per-mount
// bars + IO line. Problems are NOT included here (moved to top strip).
func dashboardMetricsContent(server collect.Metrics, width int, compact bool) []string {
	barW := max(10, width-18)
	loadLine := fmt.Sprintf("%s  %s  %s  %s · %d ядер",
		loadColorStyle(server.Load1, server.NumCPU),
		loadColorStyle(server.Load5, server.NumCPU),
		loadColorStyle(server.Load15, server.NumCPU),
		dimStyle.Render("load"),
		server.NumCPU)
	indent := 8 + max(0, (barW-lipgloss.Width(loadLine))/2)
	loadLine = strings.Repeat(" ", indent) + loadLine

	rows := []string{
		fmt.Sprintf("%s  %s  %3.0f%%", titleStyle.Render(padLabel("CPU", 6)), gauge(server.CPUPct, barW), server.CPUPct),
		loadLine,
	}
	if !compact {
		rows = append(rows, "")
	}
	rows = append(rows,
		fmt.Sprintf("%s  %s  %3.0f%%", titleStyle.Render(padLabel("ПАМЯТЬ", 6)), gauge(server.MemPct, barW), server.MemPct),
		memoryText(server),
		"SWAP     "+swapText(server),
	)
	if !compact {
		rows = append(rows, "")
	}
	rows = append(rows, titleStyle.Render("ДИСКИ / IO")+"  "+diskText(server))
	rows = append(rows, diskBars(server, width)...)
	return rows
}

func padLabel(label string, width int) string {
	for lipgloss.Width(label) < width {
		label += " "
	}
	return label
}
