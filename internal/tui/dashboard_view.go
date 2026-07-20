package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
)

func (m Model) renderDashboardWorkspace() string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return titleStyle.Render("sshmon · Дашборд") + "\n\n" + dimStyle.Render("сервер не выбран · esc назад")
	}
	server := m.snapshot.Servers[m.selected]
	lines := []string{
		titleStyle.Render("sshmon · " + server.Name),
		m.dashboardStatus(server),
	}
	if server.Err != "" {
		lines = append(lines, panelBox("ОШИБКА SSH", "r переподключить", m.layout.width, wrapWords(server.Err, m.layout.width-4))...)
	}
	if !server.Online && server.Err != "" {
		lines = append(lines, criticalStyle.Render("сервер недоступен — нажмите r для переподключения"))
	}
	if m.layout.wide {
		pw := m.dashboardPanelWidth()
		lines = append(lines,
			joinBoxes(
				panelBox("МЕТРИКИ", "p процессы · o порты · h история", pw, m.dashboardMetricsPanel(server)),
				panelBox("DOCKER", "d контейнеры", pw, m.dashboardDockerContent()),
			),
			joinBoxes(
				panelBox("СЕТЬ", "o порты", pw, dashboardNetworkContent(server)),
				panelBox("SYSTEMD", "f фильтр · j/k · enter journal", pw, m.dashboardUnitsContent()),
			),
		)
		lines = append(lines, panelBox(m.dashboardLogsTitle(), "l логи · x системный лог", m.layout.width, m.dashboardLogsContent())...)
	} else {
		lines = append(lines,
			dimStyle.Render("p процессы · o порты · h история"),
			dimStyle.Render("l логи · d контейнеры · f фильтр · x системный лог"),
		)
		lines = append(lines, m.dashboardMetricsPanel(server)...)
		lines = append(lines, m.dashboardDockerPanel()...)
		lines = append(lines, dashboardNetworkPanel(server)...)
		lines = append(lines, m.dashboardUnitsPanel()...)
		lines = append(lines, m.dashboardLogsPanel()...)
	}
	lines = append(lines, dimStyle.Render("j/k юнит · enter journal · r переподключить · c чат · esc назад"))
	return strings.Join(lines, "\n")
}

func (m Model) dashboardStatus(server collect.Metrics) string {
	status := goodStyle.Render("● ДОСТУПЕН")
	if !server.Online {
		status = criticalStyle.Render("× НЕДОСТУПЕН")
	}
	if server.Time.IsZero() {
		status = dimStyle.Render("◌ ОЖИДАНИЕ")
	}
	return fitLine(fmt.Sprintf("%s · %s · данные %s · uptime %s", status, server.Hostname, dashboardAge(m.snapshot.Time, server.Time), server.Uptime.Round(time.Minute)), m.layout.width)
}

func (m Model) dashboardMetricsPanel(server collect.Metrics) []string {
	width := max(20, m.dashboardPanelWidth())
	rows := []string{
		titleStyle.Render("CPU") + "  " + percentLine("", server.CPUPct, width-7),
		fmt.Sprintf("LOAD     %.2f  %.2f  %.2f · %d ядер", server.Load1, server.Load5, server.Load15, server.NumCPU),
		titleStyle.Render("ПАМЯТЬ") + "  " + percentLine("", server.MemPct, width-10),
		memoryText(server),
		"SWAP     " + swapText(server),
		titleStyle.Render("ДИСКИ / IO") + "  " + diskText(server),
	}
	rows = append(rows, diskTable(server)...)
	return append(rows, titleStyle.Render("ПРОБЛЕМЫ")+"  "+m.dashboardIssues(server.Name))
}

func (m Model) dashboardDockerPanel() []string {
	rows := []string{titleStyle.Render("DOCKER")}
	if len(m.dashboard.containers.items) == 0 || m.dashboard.containers.status == diagnosticsUnsupported || m.dashboard.containers.status == diagnosticsError {
		return append(rows, criticalStyle.Render("DOCKER NOT RUNNING"))
	}
	rows = append(rows, dimStyle.Render(fmt.Sprintf("%-16s %-12s %6s %6s", "ИМЯ", "СТАТУС", "CPU", "MEM")))
	for _, container := range m.dashboard.containers.items {
		rows = append(rows, fmt.Sprintf("%-16s %-12s %5.1f%% %5.1f%%", truncateCells(container.Name, 16), truncateCells(container.Status, 12), container.CPUPct, container.MemPct))
	}
	return rows
}

func dashboardNetworkPanel(server collect.Metrics) []string {
	rows := []string{titleStyle.Render("СЕТЬ") + "  " + networkText(server)}
	if table := netTable(server); len(table) > 0 {
		rows = append(rows, table...)
	}
	return rows
}

func (m Model) dashboardUnitsPanel() []string {
	rows := []string{titleStyle.Render("SYSTEMD")}
	if m.dashboard.unitUI.initialized && m.dashboard.unitUI.input.Value() != "" {
		rows = append(rows, "фильтр: "+m.dashboard.unitUI.input.Value())
	}
	units := m.filteredDashboardUnits()
	if len(units) == 0 {
		return append(rows, dimStyle.Render("юниты не найдены"))
	}
	cursor := min(m.dashboard.unitUI.cursor, len(units)-1)
	for index, unit := range units {
		prefix := "  "
		if index == cursor {
			prefix = "▶ "
		}
		state := strings.TrimSpace(unit.Active + " " + unit.Sub)
		rows = append(rows, fmt.Sprintf("%s%-24s %s", prefix, truncateCells(unit.Name, 24), state))
	}
	return rows
}

func (m Model) dashboardLogsPanel() []string {
	title := "ЛОГИ · SYSTEM"
	if m.dashboard.logs.source.Kind == collect.LogJournal {
		title = "ЛОГИ · " + m.dashboard.logs.source.Name
	}
	rows := []string{titleStyle.Render(title)}
	if m.dashboard.logs.err != nil {
		rows = append(rows, criticalStyle.Render(m.dashboard.logs.err.Error()))
	}
	if len(m.dashboard.logs.lines) == 0 {
		if m.dashboard.logs.status == diagnosticsLoading {
			return append(rows, dimStyle.Render("загрузка…"))
		}
		return append(rows, dimStyle.Render("нет строк"))
	}
	limit := 2
	if m.layout.wide {
		limit = 5
	}
	start := max(0, len(m.dashboard.logs.lines)-limit)
	for _, line := range m.dashboard.logs.lines[start:] {
		rows = append(rows, fitLine(line, m.layout.width))
	}
	return rows
}

func (m Model) dashboardPanelWidth() int {
	if !m.layout.wide {
		return m.layout.width
	}
	return max(32, (m.layout.width-2)/2)
}
