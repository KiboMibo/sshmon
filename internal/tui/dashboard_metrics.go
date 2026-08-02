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
// текущего процента (история для одного числа не нужна), а у CPU и NET —
// настоящий тренд из кольца в Model. У NET шкалы процентов нет (байты/с в них
// не переводятся), зато есть ряд суммарного rx+tx: historySparkline нормирует
// его по самой серии, и колонка перестаёт быть пустой без выдуманного «100 %».
// Пустой ряд отдаётся как nil: сплошная линия historySparkline читалась бы как
// «тренд ровный», хотя ряда ещё просто нет.
func serverMetricGrid(server collect.Metrics, cpuSeries, netSeries []*float64, width int) []string {
	var cpuTrend, netTrend, diskTrend metricTrend
	if len(cpuSeries) > 0 {
		cpuTrend = func(width int) string { return historySparkline(cpuSeries, width) }
	}
	if len(netSeries) > 0 {
		netTrend = func(width int) string { return historySparkline(netSeries, width) }
	}
	diskPct, diskDetails := metricNoPercent, dimStyle.Render("диски не найдены")
	if disk, ok := fullestDisk(server); ok {
		diskPct, diskDetails = disk.UsedPct, diskUsageDetails(disk)
		diskTrend = func(width int) string { return gauge(disk.UsedPct, width) }
	}
	return []string{
		metricRow("CPU", cpuTrend, server.CPUPct, cpuDetails(server), width),
		metricRow("MEM", func(width int) string { return gauge(server.MemPct, width) }, server.MemPct, memoryDetails(server), width),
		metricRow("NET", netTrend, metricNoPercent, networkDetails(server), width),
		metricRow("DISK", diskTrend, diskPct, diskDetails, width),
	}
}

// cpuTrendPoints — длина кольца тренда: точка на тик коллектора, минута с
// хвостом. Ряды живут в Model, а не в базе истории: экран перерисовывается на
// каждое событие, и запрос в БД на кадр этим трендам не окупается.
const cpuTrendPoints = 60

// recordTrends добавляет точку в кольца CPU и NET по каждому серверу свежего
// снапшота. Оба ряда собираются в один проход и по одним правилам: разойдись
// они длиной или составом хостов, две соседние строки сетки метрик показывали
// бы разные отрезки времени.
func (m *Model) recordTrends() {
	m.cpuTrends = appendTrendPoints(m.cpuTrends, m.snapshot.Servers, func(s collect.Metrics) float64 { return s.CPUPct })
	m.netTrends = appendTrendPoints(m.netTrends, m.snapshot.Servers, netTotalBps)
}

// appendTrendPoints возвращает новую карту рядов с добавленной точкой по
// каждому серверу. Карта пересобирается целиком, поэтому ряды серверов,
// пропавших из конфига или снапшота, не переносятся и память не растёт. Сервер
// offline даёт nil — разрыв в спарклайне, а не полку из последнего известного
// значения.
func appendTrendPoints(previous map[string][]*float64, servers []collect.Metrics, value func(collect.Metrics) float64) map[string][]*float64 {
	trends := make(map[string][]*float64, len(servers))
	for _, server := range servers {
		var point *float64
		if server.Online {
			sample := value(server)
			point = &sample
		}
		series := append(previous[server.Name], point)
		if len(series) > cpuTrendPoints {
			series = series[len(series)-cpuTrendPoints:]
		}
		trends[server.Name] = series
	}
	return trends
}

// netTotalBps — суммарный трафик хоста: спарклайн NET рисует одну линию, а
// интерфейсов на хосте несколько, и складывать их приходится до нормировки.
func netTotalBps(server collect.Metrics) float64 {
	var total float64
	for _, device := range server.Net {
		total += device.RxBps + device.TxBps
	}
	return total
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
