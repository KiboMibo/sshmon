package tui

import (
	"fmt"
	"strings"

	"github.com/kibomibo/sshmon/internal/collect"
)

func (m Model) fleetCardLines(server collect.Metrics, width int) []string {
	inner := max(20, width-4)
	content := []string{fleetCardSummary(server)}
	content = append(content, m.fleetCardBody(server, inner)...)
	// fitLine и здесь: рядом с карточкой теперь остаётся сайдбар, левая колонка
	// уже, и полный список подсказок в неё не влезает — его надо ужать, а не
	// дать рамке съесть хвост.
	content = append(content, dimStyle.Render(fitLine("[l] логи  [p] процессы  [o] порты  [d] контейнеры  [x] ssh  [←] свернуть", inner)))
	return panelBoxStyled("", "", width, content, dimStyle)
}

func fleetCardSummary(server collect.Metrics) string {
	parts := make([]string, 0, 4)
	if server.Hostname != "" {
		parts = append(parts, server.Hostname)
	}
	if server.NumCPU > 0 {
		parts = append(parts, coresText(server.NumCPU))
	}
	if server.MemTotalKB > 0 {
		parts = append(parts, byteValue(float64(server.MemTotalKB)*1024))
	}
	if server.Uptime > 0 {
		parts = append(parts, "up "+formatUptime(server.Uptime))
	}
	if len(parts) == 0 {
		return dimStyle.Render("данных пока нет")
	}
	return titleStyle.Render(strings.Join(parts, " · "))
}

func (m Model) fleetCardBody(server collect.Metrics, width int) []string {
	if !server.Online {
		return []string{criticalStyle.Render(fitLine(fleetOfflineReason(server), width))}
	}
	barWidth := width - 26
	lines := []string{
		percentLine("cpu", server.CPUPct, barWidth) + fmt.Sprintf("  load %.2f %.2f %.2f", server.Load1, server.Load5, server.Load15),
		percentLine("mem", server.MemPct, barWidth) + "  " + memoryTail(server),
	}
	if disk, ok := busiestDisk(server.Disks); ok {
		lines = append(lines, percentLine("disk", disk.UsedPct, barWidth)+"  "+diskTail(disk))
	}
	if net := netTail(server.Net); net != "" {
		lines = append(lines, fmt.Sprintf("%-7s %s", "net", net))
	}
	lines = append(lines, fmt.Sprintf("%-7s %s", "srv", m.servicesText()))
	lines = append(lines, fmt.Sprintf("%-7s %s", "docker", m.dockerText(server)))
	lines = append(lines, fmt.Sprintf("%-7s %s", "порты", portsTail(server.Ports)))
	for i, line := range lines {
		lines[i] = fitLine(line, width)
	}
	return lines
}

func fleetOfflineReason(server collect.Metrics) string {
	if server.Err != "" {
		return server.Err
	}
	return "нет связи"
}

func memoryTail(server collect.Metrics) string {
	if server.MemTotalKB <= 0 {
		return ""
	}
	used := (float64(server.MemTotalKB) - float64(server.MemAvailKB)) * 1024
	tail := byteValue(used) + " / " + byteValue(float64(server.MemTotalKB)*1024)
	if server.SwapTotalKB > 0 {
		swap := (float64(server.SwapTotalKB) - float64(server.SwapFreeKB)) * 1024
		tail += "  swap " + byteValue(swap) + " / " + byteValue(float64(server.SwapTotalKB)*1024)
	}
	return tail
}

// rootDiskUsage — значение колонки DISK: корень, если он есть в собранных
// данных, иначе самый заполненный раздел. Разделов на хосте бывает десяток, и
// спрашивают обычно про «/»; когда его в df нет (контейнер, отдельный
// дата-хост), показываем тот, что кончится первым. Раздел, переваливший порог,
// всё равно назовёт себя по имени в колонке СОСТ.
func rootDiskUsage(disks []collect.DiskUsage) (collect.DiskUsage, bool) {
	for _, disk := range disks {
		if disk.Mount == "/" {
			return disk, true
		}
	}
	return busiestDisk(disks)
}

func busiestDisk(disks []collect.DiskUsage) (collect.DiskUsage, bool) {
	if len(disks) == 0 {
		return collect.DiskUsage{}, false
	}
	busiest := disks[0]
	for _, disk := range disks[1:] {
		if disk.UsedPct > busiest.UsedPct {
			busiest = disk
		}
	}
	return busiest, true
}

func diskTail(disk collect.DiskUsage) string {
	tail := byteValue(float64(disk.UsedKB)*1024) + " / " + byteValue(float64(disk.TotalKB)*1024)
	if disk.Mount != "" {
		tail += " " + disk.Mount
	}
	return tail
}

func netTail(rates []collect.NetRate) string {
	if len(rates) == 0 {
		return ""
	}
	var rx, tx float64
	for _, rate := range rates {
		rx += rate.RxBps
		tx += rate.TxBps
	}
	return "rx " + byteValue(rx) + "/s  tx " + byteValue(tx) + "/s"
}

func (m Model) dockerText(server collect.Metrics) string {
	d := server.Docker
	if d.Total() == 0 {
		return dimStyle.Render(m.dockerEmptyText(server))
	}
	parts := []string{goodStyle.Render(fmt.Sprintf("● %d запущено", d.Running))}
	if d.Stopped > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("○ %d остановлено", d.Stopped)))
	}
	if d.Broken > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("⚠ %d %s", d.Broken, plural(d.Broken, "проблемный", "проблемных", "проблемных"))))
	}
	return strings.Join(parts, "  ")
}

// dockerEmptyText — почему в карточке не видно ни одного контейнера. Точную
// причину («нет прав», «демон не отвечает») знает только диагностика экрана
// сервера — её текст берём первым, если она про этот же хост, иначе два экрана
// про один хост рассказывают разное. Для остальных хостов остаётся факт из
// сэмпла: он различает «docker не ответил» и «контейнеров нет», а формулировки
// те же, что у dockerStateText.
func (m Model) dockerEmptyText(server collect.Metrics) string {
	if m.dashboard.server == server.Name && m.dashboard.containers.status != diagnosticsIdle {
		return dockerStateText(m.dashboard.containers)
	}
	if !server.Docker.Known {
		// Не «не установлен»: сэмпл глушит stderr и не отличает отсутствие
		// docker'а от отказа в правах. Разделяет их только диагностика выше.
		return "docker недоступен"
	}
	return "контейнеров нет"
}

func portsTail(ports []collect.Port) string {
	if len(ports) == 0 {
		return dimStyle.Render("нет")
	}
	const shown = 4
	parts := make([]string, 0, shown)
	for _, port := range ports[:min(shown, len(ports))] {
		entry := port.Local
		if port.Process != "" {
			entry += " " + port.Process
		}
		parts = append(parts, entry)
	}
	tail := strings.Join(parts, "  ")
	if len(ports) > shown {
		tail += dimStyle.Render(fmt.Sprintf("  +%d", len(ports)-shown))
	}
	return tail
}

func (m Model) servicesText() string {
	st := m.dashboard.units
	switch st.status {
	case diagnosticsLoading, diagnosticsIdle:
		return dimStyle.Render("…")
	case diagnosticsUnsupported:
		return dimStyle.Render("systemd недоступен")
	case diagnosticsError:
		return dimStyle.Render("сервисы недоступны")
	}
	if len(st.items) == 0 {
		return dimStyle.Render("сервисов нет")
	}
	parts := make([]string, 0, len(st.items))
	for _, u := range st.items {
		dot := goodStyle.Render("●")
		if u.Active == "failed" {
			dot = criticalStyle.Render("×")
		} else if u.Active != "active" {
			dot = dimStyle.Render("○")
		}
		parts = append(parts, dot+" "+u.Name)
	}
	const shown = 4
	tail := strings.Join(parts[:min(shown, len(parts))], "  ")
	if len(parts) > shown {
		tail += dimStyle.Render(fmt.Sprintf("  +%d", len(parts)-shown))
	}
	return tail
}

func containerCounts(items []collect.Container) string {
	var up, exited, other int
	for _, item := range items {
		switch {
		case strings.HasPrefix(item.Status, "Up"):
			up++
		case strings.HasPrefix(item.Status, "Exited"):
			exited++
		default:
			other++
		}
	}
	parts := []string{goodStyle.Render(fmt.Sprintf("● %d запущено", up))}
	if exited > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("○ %d остановлено", exited)))
	}
	if other > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("⚠ %d %s", other, plural(other, "проблемный", "проблемных", "проблемных"))))
	}
	return strings.Join(parts, "  ")
}
