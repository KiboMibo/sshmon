package tui

import "fmt"

// fleetDetailTitle подписывает правую колонку Fleet именем выбранного сервера,
// либо нейтральным заголовком, когда выбор вне диапазона.
func (m Model) fleetDetailTitle() string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return "ПОДРОБНОСТИ"
	}
	return truncateCells(m.snapshot.Servers[m.selected].Name, 20)
}

// fleetDetailContent раскрывает подробности выбранного хоста на всю ширину
// правой колонки: имя хоста, статус, увеличенные бары CPU/MEM, нагрузку,
// аптайм и число проблем.
func (m Model) fleetDetailContent(width int) []string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return []string{dimStyle.Render("сервер не выбран")}
	}
	server := m.snapshot.Servers[m.selected]
	issues := issuesForServer(m.snapshot.Issues, server.Name)
	return []string{
		server.Hostname,
		statusText(server),
		"",
		percentLine("CPU", server.CPUPct, width),
		percentLine("MEM", server.MemPct, width),
		"",
		fmt.Sprintf("load    %.2f", server.Load1),
		fmt.Sprintf("uptime  %s", formatUptime(server.Uptime)),
		fmt.Sprintf("проблемы: %d", len(issues)),
	}
}
