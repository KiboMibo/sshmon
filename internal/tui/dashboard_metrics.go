package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

// loadStyle — цвет числа load average по порогам, привязанным к числу ядер:
// зелёный < 0.75×NumCPU, жёлтый 0.75–1.5×, красный > 1.5×. Вынесен из
// loadColorStyle отдельно, чтобы тест сравнивал стиль, а не отрисованную
// строку: под go test у lipgloss профиль Ascii и цвета в выводе нет.
func loadStyle(load float64, numCPU int) lipgloss.Style {
	if numCPU < 1 {
		numCPU = 1
	}
	switch ratio := load / float64(numCPU); {
	case ratio < 0.75:
		return goodStyle
	case ratio <= 1.5:
		return warnStyle
	default:
		return criticalStyle
	}
}

func loadColorStyle(load float64, numCPU int) string {
	return loadStyle(load, numCPU).Render(fmt.Sprintf("%.2f", load))
}

// serverMetricGrid — четыре однородные строки макета: CPU / MEM / NET / DISK.
// Все четыре рисует один примитив metricRow, поэтому тренд, процент и детали
// стоят в одних и тех же колонках при любой ширине ≥ 60.
//
// Колонка тренда у каждой метрики своя по макету: у MEM и DISK это заливка
// текущего процента (история для одного числа не нужна), у NET её нет вовсе
// (байты/с в шкалу не переводятся), а у CPU — настоящий тренд из кольца в
// Model. Пустой ряд отдаётся как nil: сплошная линия historySparkline
// читалась бы как «тренд ровный», хотя ряда ещё просто нет.
func serverMetricGrid(server collect.Metrics, cpuSeries []*float64, width int) []string {
	var cpuTrend, diskTrend metricTrend
	if len(cpuSeries) > 0 {
		cpuTrend = func(width int) string { return historySparkline(cpuSeries, width) }
	}
	diskPct, diskDetails := metricNoPercent, dimStyle.Render("диски не найдены")
	if disk, ok := fullestDisk(server); ok {
		diskPct, diskDetails = disk.UsedPct, diskUsageDetails(disk)
		diskTrend = func(width int) string { return gauge(disk.UsedPct, width) }
	}
	return []string{
		metricRow("CPU", cpuTrend, server.CPUPct, cpuDetails(server), width),
		metricRow("MEM", func(width int) string { return gauge(server.MemPct, width) }, server.MemPct, memoryDetails(server), width),
		metricRow("NET", nil, metricNoPercent, networkDetails(server), width),
		metricRow("DISK", diskTrend, diskPct, diskDetails, width),
	}
}

// cpuTrendPoints — длина кольца тренда CPU: точка на тик коллектора, минута с
// хвостом. Ряд живёт в Model, а не в базе истории: экран перерисовывается на
// каждое событие, и запрос в БД на кадр этому тренду не окупается.
const cpuTrendPoints = 60

// recordCPUTrend добавляет точку CPU по каждому серверу свежего снапшота.
// Карта пересобирается целиком, поэтому ряды серверов, пропавших из конфига
// или снапшота, не переносятся и память не растёт. Сервер offline даёт nil —
// разрыв в спарклайне, а не полку из последнего известного значения.
func (m *Model) recordCPUTrend() {
	trends := make(map[string][]*float64, len(m.snapshot.Servers))
	for _, server := range m.snapshot.Servers {
		var point *float64
		if server.Online {
			value := server.CPUPct
			point = &value
		}
		series := append(m.cpuTrends[server.Name], point)
		if len(series) > cpuTrendPoints {
			series = series[len(series)-cpuTrendPoints:]
		}
		trends[server.Name] = series
	}
	m.cpuTrends = trends
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
