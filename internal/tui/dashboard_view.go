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
	if len(issuesForServer(m.snapshot.Issues, server.Name)) > 0 {
		if m.layout.wide {
			lines = append(lines, panelBox("ПРОБЛЕМЫ", "r переподключить", m.layout.width, []string{m.dashboardIssues(server.Name)})...)
		} else {
			lines = append(lines, dimStyle.Render("ПРОБЛЕМЫ: "+m.dashboardIssues(server.Name)))
		}
	}
	if m.layout.wide {
		budget := max(2, m.layout.height-len(lines)-1-4)
		row1H := max(1, budget/3)
		row2H := max(1, budget-row1H)
		colW := (m.layout.width - 4) / 3
		metricsCol := m.tilePanel(tileMetrics, "МЕТРИКИ", "p процессы · o порты · ctrl+h история", colW,
			fitPanelHeight(dashboardMetricsContent(server, colW, false), row1H, m.dashboard.tileScrolls[tileMetrics]))
		netBody := append(fitPanelHeight(dashboardNetworkContent(server), max(1, row1H-1), m.dashboard.tileScrolls[tileNetwork]), networkText(server))
		netCol := m.tilePanel(tileNetwork, "СЕТЬ", "o порты", colW, netBody)
		systemdCol := m.tilePanel(tileSystemd, "SYSTEMD", "f фильтр · j/k · enter journal", colW,
			fitPanelHeight(m.dashboardUnitsContent(), row1H, m.systemdScroll(row1H)))
		lines = append(lines, joinBoxes(metricsCol, netCol, systemdCol))
		if m.dashboardHasDocker() {
			dockerW := (m.layout.width - 2) / 3
			dockerCol := m.tilePanel(tileDocker, "DOCKER", "d контейнеры", dockerW,
				fitPanelHeight(m.dashboardDockerContent(), row2H, m.dashboard.tileScrolls[tileDocker]))
			logsCol := m.tilePanel(tileLogs, m.dashboardLogsTitle(), "ctrl+l логи · x системный лог", m.layout.width-2-dockerW,
				fitLogsHeight(m.dashboardLogsContent(), row2H, m.dashboard.tileScrolls[tileLogs]))
			lines = append(lines, joinBoxes(dockerCol, logsCol))
		} else {
			lines = append(lines, m.tilePanel(tileLogs, m.dashboardLogsTitle(), "ctrl+l логи · x системный лог", m.layout.width,
				fitLogsHeight(m.dashboardLogsContent(), row2H, m.dashboard.tileScrolls[tileLogs]))...)
		}
		lines = append(lines, "")
	} else {
		lines = append(lines,
			dimStyle.Render("p процессы · o порты · ctrl+h история"),
			dimStyle.Render("ctrl+l логи · d контейнеры · f фильтр · x системный лог"),
		)
		lines = append(lines, m.dashboardMetricsPanel(server)...)
		lines = append(lines, m.dashboardDockerPanel()...)
		lines = append(lines, dashboardNetworkPanel(server)...)
		lines = append(lines, m.dashboardUnitsPanel()...)
		lines = append(lines, m.dashboardLogsPanel()...)
		lines = append(lines, dimStyle.Render("j/k юнит · enter journal · r переподключить · c чат · esc назад"))
	}
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
	return dashboardMetricsContent(server, width, !m.layout.wide)
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

func (m Model) tilePanel(tile uint8, title, hint string, width int, content []string) []string {
	return panelBoxStyled(title, hint, width, content, m.tileBorderStyle(tile))
}

func (m Model) dashboardHasDocker() bool {
	c := m.dashboard.containers
	return len(c.items) > 0 && c.status != diagnosticsUnsupported && c.status != diagnosticsError
}
