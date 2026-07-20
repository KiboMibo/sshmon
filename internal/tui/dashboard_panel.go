package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

// panelBox frames content with a titled top border and a hinted bottom border,
// so each Dashboard cell is visually separated and documents its own controls.
func panelBox(title, hint string, width int, content []string) []string {
	if width < 6 {
		width = 6
	}
	lines := make([]string, 0, len(content)+2)
	lines = append(lines, borderLine("╭", "╮", title, width))
	inner := width - 4
	for _, row := range content {
		lines = append(lines, dimStyle.Render("│")+" "+padCell(row, inner)+" "+dimStyle.Render("│"))
	}
	return append(lines, borderLine("╰", "╯", hint, width))
}

func borderLine(left, right, label string, width int) string {
	if label == "" {
		return dimStyle.Render(left + strings.Repeat("─", width-2) + right)
	}
	fill := max(1, width-5-lipgloss.Width(label))
	return dimStyle.Render(left+"─ ") + titleStyle.Render(label) + dimStyle.Render(" "+strings.Repeat("─", fill)+right)
}

func padCell(value string, width int) string {
	if lipgloss.Width(value) > width {
		value = fitLine(value, width)
	}
	if pad := width - lipgloss.Width(value); pad > 0 {
		value += strings.Repeat(" ", pad)
	}
	return value
}

func joinBoxes(left, right []string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), "  ", strings.Join(right, "\n"))
}

func (m Model) dashboardDockerContent() []string {
	if len(m.dashboard.containers.items) == 0 || m.dashboard.containers.status == diagnosticsUnsupported || m.dashboard.containers.status == diagnosticsError {
		return []string{criticalStyle.Render("DOCKER NOT RUNNING")}
	}
	rows := []string{dimStyle.Render(fmt.Sprintf("%-16s %-12s %6s %6s", "ИМЯ", "СТАТУС", "CPU", "MEM"))}
	for _, container := range m.dashboard.containers.items {
		rows = append(rows, fmt.Sprintf("%-16s %-12s %5.1f%% %5.1f%%", truncateCells(container.Name, 16), truncateCells(container.Status, 12), container.CPUPct, container.MemPct))
	}
	return rows
}

func dashboardNetworkContent(server collect.Metrics) []string {
	return append([]string{networkText(server)}, netTable(server)...)
}

func (m Model) dashboardUnitsContent() []string {
	rows := []string(nil)
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

func (m Model) dashboardLogsContent() []string {
	if m.dashboard.logs.err != nil {
		return []string{criticalStyle.Render(m.dashboard.logs.err.Error())}
	}
	if len(m.dashboard.logs.lines) == 0 {
		if m.dashboard.logs.status == diagnosticsLoading {
			return []string{dimStyle.Render("загрузка…")}
		}
		return []string{dimStyle.Render("нет строк")}
	}
	start := max(0, len(m.dashboard.logs.lines)-5)
	rows := make([]string, 0, len(m.dashboard.logs.lines)-start)
	for _, line := range m.dashboard.logs.lines[start:] {
		rows = append(rows, fitLine(line, m.layout.width-4))
	}
	return rows
}

func (m Model) dashboardLogsTitle() string {
	if m.dashboard.logs.source.Kind == collect.LogJournal {
		return "ЛОГИ · " + m.dashboard.logs.source.Name
	}
	return "ЛОГИ · SYSTEM"
}
