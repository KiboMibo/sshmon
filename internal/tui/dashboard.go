package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
)

func (m Model) renderDashboard() string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return titleStyle.Render("sshmon · Дашборд") + "\n\n" + dimStyle.Render("сервер не выбран · esc назад")
	}
	server := m.snapshot.Servers[m.selected]
	width := max(40, m.layout.width)
	contentWidth := max(20, width-2)
	status := goodStyle.Render("● ДОСТУПЕН")
	if !server.Online {
		status = criticalStyle.Render("× НЕДОСТУПЕН")
	}
	if server.Time.IsZero() {
		status = dimStyle.Render("◌ ОЖИДАНИЕ")
	}

	lines := []string{
		titleStyle.Render("sshmon · " + server.Name),
		fitLine(fmt.Sprintf("%s · %s · данные %s · uptime %s", status, server.Hostname, dashboardAge(m.snapshot.Time, server.Time), server.Uptime.Round(time.Minute)), contentWidth),
	}
	if server.Err != "" {
		lines = append(lines, fitLine("ошибка SSH: "+server.Err, contentWidth))
	}
	lines = append(lines,
		"",
		titleStyle.Render("CPU")+"  "+percentLine("", server.CPUPct, contentWidth-7),
		fmt.Sprintf("LOAD     %.2f  %.2f  %.2f · %d ядер", server.Load1, server.Load5, server.Load15, server.NumCPU),
		"",
		titleStyle.Render("ПАМЯТЬ")+"  "+percentLine("", server.MemPct, contentWidth-10),
		memoryText(server),
		"SWAP     "+swapText(server),
		"",
		titleStyle.Render("СЕТЬ")+"    "+networkText(server),
		titleStyle.Render("ДИСКИ / IO")+"  "+diskText(server),
		"",
		titleStyle.Render("ПРОБЛЕМЫ")+"  "+m.dashboardIssues(server.Name),
	)
	if m.layout.wide {
		lines = append(lines, wideDeviceLines(server, contentWidth)...)
	}
	lines = append(lines, "", dimStyle.Render("p процессы · o порты · h история · l логи · d контейнеры · c чат · esc назад"))
	return strings.Join(lines, "\n")
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
	var used, read, write float64
	for _, disk := range server.Disks {
		used = max(used, disk.UsedPct)
	}
	for _, device := range server.IO {
		read += device.ReadBps
		write += device.WriteBps
	}
	return fmt.Sprintf("max %.0f%% · R %s/s · W %s/s", used, byteValue(read), byteValue(write))
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

func wideDeviceLines(server collect.Metrics, width int) []string {
	lines := make([]string, 0, len(server.Net)+len(server.Disks))
	for _, device := range server.Net {
		lines = append(lines, fitLine(fmt.Sprintf("  net %-10s rx %s/s tx %s/s", device.Iface, byteValue(device.RxBps), byteValue(device.TxBps)), width))
	}
	for _, disk := range server.Disks {
		lines = append(lines, fitLine(fmt.Sprintf("  disk %-10s %3.0f%% · свободно %s", disk.Mount, disk.UsedPct, byteValue(float64(disk.AvailKB)*1024)), width))
	}
	return lines
}
