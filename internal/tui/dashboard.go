package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

func (m Model) renderDashboard() string {
	return m.renderDashboardWorkspace()
}

func dashboardAge(now, sampled time.Time) string {
	if sampled.IsZero() {
		return "—"
	}
	if now.IsZero() {
		now = time.Now()
	}
	age := max(time.Duration(0), now.Sub(sampled))
	if age >= time.Minute {
		return fmt.Sprintf("%dm", int(age.Minutes()))
	}
	return fmt.Sprintf("%ds", int(age.Seconds()))
}

func memoryText(server collect.Metrics) string {
	used := server.MemTotalKB - min(server.MemTotalKB, server.MemAvailKB)
	return fmt.Sprintf("RAM      %s / %s", byteValue(float64(used)*1024), byteValue(float64(server.MemTotalKB)*1024))
}

func swapText(server collect.Metrics) string {
	if server.SwapTotalKB == 0 {
		return "не используется"
	}
	used := server.SwapTotalKB - min(server.SwapTotalKB, server.SwapFreeKB)
	return fmt.Sprintf("%s / %s", byteValue(float64(used)*1024), byteValue(float64(server.SwapTotalKB)*1024))
}

func networkText(server collect.Metrics) string {
	var rx, tx float64
	for _, device := range server.Net {
		rx += device.RxBps
		tx += device.TxBps
	}
	return fmt.Sprintf("rx %s/s · tx %s/s", byteValue(rx), byteValue(tx))
}

func diskText(server collect.Metrics) string {
	var read, write float64
	for _, device := range server.IO {
		read += device.ReadBps
		write += device.WriteBps
	}
	return fmt.Sprintf("R %s/s · W %s/s", byteValue(read), byteValue(write))
}

func (m Model) dashboardIssues(name string) string {
	issues := issuesForServer(m.snapshot.Issues, name)
	if len(issues) == 0 {
		return goodStyle.Render("нет")
	}
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, fmt.Sprintf("[%s] %s", issue.Severity, issue.Msg))
	}
	return fitLine(strings.Join(parts, " · "), max(20, m.layout.width-12))
}

func (m Model) dashboardIssueText(name string) string {
	issues := issuesForServer(m.snapshot.Issues, name)
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, fmt.Sprintf("[%s] %s", issue.Severity, issue.Msg))
	}
	return strings.Join(parts, " · ")
}

func deviceTables(server collect.Metrics, layout layoutState) []string {
	net := netTable(server)
	disks := diskTable(server)
	if layout.wide && len(net) > 0 && len(disks) > 0 {
		left := lipgloss.NewStyle().Width(max(38, layout.width/2)).Render(strings.Join(net, "\n"))
		return []string{lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Join(disks, "\n"))}
	}
	return append(net, disks...)
}

func netTable(server collect.Metrics) []string {
	if len(server.Net) == 0 {
		return nil
	}
	rows := []string{dimStyle.Render(fmt.Sprintf("%-14s %10s %10s", "ИНТЕРФЕЙС", "RX/S", "TX/S"))}
	for _, device := range server.Net {
		rows = append(rows, fmt.Sprintf("%-14s %10s %10s", device.Iface, byteValue(device.RxBps), byteValue(device.TxBps)))
	}
	return rows
}

func diskTable(server collect.Metrics) []string {
	if len(server.Disks) == 0 {
		return nil
	}
	rows := []string{dimStyle.Render(fmt.Sprintf("%-16s %7s %10s", "ТОЧКА", "ЗАНЯТО", "СВОБОДНО"))}
	for _, disk := range server.Disks {
		rows = append(rows, fmt.Sprintf("%-16s %6.0f%% %10s", disk.Mount, disk.UsedPct, byteValue(float64(disk.AvailKB)*1024)))
	}
	return rows
}
