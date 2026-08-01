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

// serverMetricGrid — четыре однородные строки макета: CPU / MEM / NET / DISK.
// Все четыре рисует один примитив metricRow, поэтому спарклайн, процент и
// детали стоят в одних и тех же колонках при любой ширине ≥ 60.
//
// Спарклайны пока пустые: ряды живут в internal/history и загружаются только
// экраном «История» под выбранный там сервер. Показывать на экране сервера
// чужой или замороженный тренд хуже, чем заглушку; как только появится ряд,
// привязанный к текущему выбору, его достаточно передать сюда.
func serverMetricGrid(server collect.Metrics, width int) []string {
	diskPct, diskDetails := metricNoPercent, dimStyle.Render("диски не найдены")
	if disk, ok := fullestDisk(server); ok {
		diskPct, diskDetails = disk.UsedPct, diskUsageDetails(disk)
	}
	return []string{
		metricRow("CPU", nil, server.CPUPct, cpuDetails(server), width),
		metricRow("MEM", nil, server.MemPct, memoryDetails(server), width),
		metricRow("NET", nil, metricNoPercent, networkDetails(server), width),
		metricRow("DISK", nil, diskPct, diskDetails, width),
	}
}

func cpuDetails(server collect.Metrics) string {
	return fmt.Sprintf("%s %s %s %s",
		dimStyle.Render("load"),
		loadColorStyle(server.Load1, server.NumCPU),
		loadColorStyle(server.Load5, server.NumCPU),
		loadColorStyle(server.Load15, server.NumCPU))
}

func memoryDetails(server collect.Metrics) string {
	used := server.MemTotalKB - min(server.MemTotalKB, server.MemAvailKB)
	details := fmt.Sprintf("%s / %s", byteValue(float64(used)*1024), byteValue(float64(server.MemTotalKB)*1024))
	if server.SwapTotalKB == 0 {
		return details
	}
	swapUsed := server.SwapTotalKB - min(server.SwapTotalKB, server.SwapFreeKB)
	return details + fmt.Sprintf("   %s %s / %s", dimStyle.Render("swap"),
		byteValue(float64(swapUsed)*1024), byteValue(float64(server.SwapTotalKB)*1024))
}

func networkDetails(server collect.Metrics) string {
	var rx, tx float64
	for _, device := range server.Net {
		rx += device.RxBps
		tx += device.TxBps
	}
	return fmt.Sprintf("%s %s/s   %s %s/s", dimStyle.Render("rx"), byteValue(rx), dimStyle.Render("tx"), byteValue(tx))
}

// fullestDisk выбирает самый заполненный раздел: строка DISK в сетке одна,
// и показывать в ней надо тот раздел, который кончится первым.
func fullestDisk(server collect.Metrics) (collect.DiskUsage, bool) {
	if len(server.Disks) == 0 {
		return collect.DiskUsage{}, false
	}
	fullest := server.Disks[0]
	for _, disk := range server.Disks[1:] {
		if disk.UsedPct > fullest.UsedPct {
			fullest = disk
		}
	}
	return fullest, true
}

func diskUsageDetails(disk collect.DiskUsage) string {
	details := fmt.Sprintf("%s / %s   %s",
		byteValue(float64(disk.UsedKB)*1024), byteValue(float64(disk.TotalKB)*1024), disk.Mount)
	if device := strings.TrimPrefix(disk.Fs, "/dev/"); device != "" {
		details += " (" + device + ")"
	}
	return details
}

func padLabel(label string, width int) string {
	for lipgloss.Width(label) < width {
		label += " "
	}
	return label
}
