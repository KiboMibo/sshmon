package tui

import (
	"fmt"
	"strings"

	"github.com/kibomibo/sshmon/internal/collect"
)

func (m Model) serverScreenLines(server collect.Metrics) []string {
	width := m.layout.width
	lines := []string{m.serverHeader(server, width)}
	if len(issuesForServer(m.snapshot.Issues, server.Name)) > 0 {
		lines = append(lines, panelBox("ПРОБЛЕМЫ", "r переподключить", width, wrapWords(m.dashboardIssueText(server.Name), width-4))...)
	}
	lines = append(lines, "")
	for _, row := range m.serverMetricGrid(server, width) {
		lines = append(lines, fitLine(row, width))
	}
	lines = append(lines, "")

	budget := max(8, m.layout.height-len(lines)-5)
	logsH := max(3, budget/2)
	midH := max(4, budget-logsH)

	var dockerCol []string
	rightW := width
	if m.dashboardHasDocker() {
		dockerW := (width - 2) / 2
		rightW = width - 2 - dockerW
		dockerCol = m.tilePanel(tileDocker, "DOCKER", "d контейнеры", dockerW,
			fitPanelHeight(m.serverDockerContent(), midH, m.dashboard.tileScrolls[tileDocker]))
	}

	ports := serverPortLines(server)
	portsH := min(len(ports), max(1, midH/3))
	servicesH := max(1, midH-2-portsH)
	rightCol := m.tilePanel(tileSystemd, "СЕРВИСЫ", "f фильтр · j/k · enter journal", rightW,
		fitPanelHeight(m.dashboardUnitsContent(), servicesH, m.systemdScroll(servicesH)))
	rightCol = append(rightCol, panelBox("ПОРТЫ", "o порты", rightW, fitPanelHeight(ports, portsH, 0))...)

	if dockerCol != nil {
		lines = append(lines, joinBoxes(dockerCol, rightCol))
	} else {
		lines = append(lines, rightCol...)
	}
	lines = append(lines, m.tilePanel(tileLogs, m.dashboardLogsTitle(), "ctrl+l логи · x системный лог", width,
		fitLogsHeight(m.dashboardLogsContent(), logsH, m.dashboard.tileScrolls[tileLogs]))...)
	return append(lines, dimStyle.Render("esc назад · p процессы · o порты · d контейнеры · ctrl+h история · r переподключить · ? ещё"))
}

func (m Model) serverHeader(server collect.Metrics, width int) string {
	state := goodStyle.Render("● ДОСТУПЕН")
	if issues := issuesForServer(m.snapshot.Issues, server.Name); len(issues) > 0 {
		state = warnStyle.Render("⚠ " + issues[0].Msg)
	}
	if !server.Online {
		state = criticalStyle.Render("× НЕДОСТУПЕН")
	}
	if server.Time.IsZero() {
		state = dimStyle.Render("◌ ОЖИДАНИЕ")
	}
	return spread(titleStyle.Render(server.Name)+"  "+state, dimStyle.Render(m.serverFacts(server)), width)
}

func (m Model) serverFacts(server collect.Metrics) string {
	facts := make([]string, 0, 5)
	for _, fact := range []string{server.Group, server.Hostname} {
		if fact != "" {
			facts = append(facts, fact)
		}
	}
	if server.NumCPU > 0 {
		facts = append(facts, fmt.Sprintf("%d ядер", server.NumCPU))
	}
	if server.MemTotalKB > 0 {
		facts = append(facts, byteValue(float64(server.MemTotalKB)*1024))
	}
	if server.Uptime > 0 {
		facts = append(facts, "up "+formatUptime(server.Uptime))
	}
	facts = append(facts, "данные "+dashboardAge(m.snapshot.Time, server.Time))
	return strings.Join(facts, " · ")
}

func (m Model) serverMetricGrid(server collect.Metrics, width int) []string {
	grid := dashboardMetricsContent(server, width, true)
	return append(grid, dashboardNetworkPanel(server)...)
}

func (m Model) serverDockerContent() []string {
	rows := m.dashboardDockerContent()
	if len(m.dashboard.containers.items) == 0 {
		return rows
	}
	return append([]string{containerCounts(m.dashboard.containers.items)}, rows...)
}

func serverPortLines(server collect.Metrics) []string {
	if len(server.Ports) == 0 {
		return []string{dimStyle.Render("портов нет")}
	}
	rows := make([]string, 0, len(server.Ports))
	for _, port := range server.Ports {
		rows = append(rows, fmt.Sprintf("%-22s %s", truncateCells(port.Local, 22), port.Process))
	}
	return rows
}
