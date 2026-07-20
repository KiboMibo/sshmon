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

// wrapWords сворачивает text по словам так, чтобы каждая выходная строка
// помещалась в width ячеек терминала (с учётом ANSI-цветов через lipgloss.Width).
// Используется для длинных сообщений об ошибках в рамке panelBox вместо обрезки.
func wrapWords(text string, width int) []string {
	if width < 1 {
		return []string{text}
	}
	var out []string
	var line strings.Builder
	lineW := 0
	for _, word := range strings.Fields(text) {
		ww := lipgloss.Width(word)
		switch {
		case lineW == 0:
			if ww > width {
				out = append(out, fitLine(word, width))
				continue
			}
			line.WriteString(word)
			lineW = ww
		case lineW+1+ww <= width:
			line.WriteByte(' ')
			line.WriteString(word)
			lineW += 1 + ww
		default:
			out = append(out, line.String())
			line.Reset()
			if ww > width {
				out = append(out, fitLine(word, width))
				lineW = 0
				continue
			}
			line.WriteString(word)
			lineW = ww
		}
	}
	if lineW > 0 {
		out = append(out, line.String())
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func joinBoxes(left, right []string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), "  ", strings.Join(right, "\n"))
}

// fitPanelHeight приводит контент к ровно height строкам: короткий дополняется
// пустыми строками снизу, длинный прокручивается сверху по scroll. Так каждый
// ряд дашборда сохраняет фиксированную высоту без заполнителей NO DATA.
func fitPanelHeight(content []string, height, scroll int) []string {
	if height < 1 {
		height = 1
	}
	if len(content) <= height {
		out := make([]string, height)
		copy(out, content)
		return out
	}
	scroll = min(max(scroll, 0), len(content)-height)
	return content[scroll : scroll+height]
}

// fitLogsHeight окно высотой height, привязанное к НИЗУ (свежие логи снизу):
// scroll уводит окно к более старым строкам.
func fitLogsHeight(content []string, height, scroll int) []string {
	if height < 1 {
		height = 1
	}
	if len(content) <= height {
		out := make([]string, height)
		copy(out, content)
		return out
	}
	scroll = min(max(scroll, 0), len(content)-height)
	end := len(content) - scroll
	return content[end-height : end]
}

func containerStatusDot(status string) string {
	switch {
	case strings.HasPrefix(status, "Up"):
		return goodStyle.Render("●")
	case strings.HasPrefix(status, "Exited"):
		return criticalStyle.Render("●")
	default:
		return dimStyle.Render("●")
	}
}

func unitStateText(active, sub string) string {
	state := strings.TrimSpace(active + " " + sub)
	switch {
	case active == "active" && sub == "running":
		return goodStyle.Render(state)
	case active == "failed":
		return criticalStyle.Render(state)
	case active == "activating":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Render(state)
	default:
		return dimStyle.Render(state)
	}
}

func (m Model) dashboardDockerContent() []string {
	if len(m.dashboard.containers.items) == 0 || m.dashboard.containers.status == diagnosticsUnsupported || m.dashboard.containers.status == diagnosticsError {
		return []string{criticalStyle.Render("DOCKER NOT RUNNING")}
	}
	rows := []string{dimStyle.Render(fmt.Sprintf("%-2s %-14s %-10s %5s %5s  %s", " ", "ИМЯ", "СТАТУС", "CPU", "MEM", "ПОРТЫ"))}
	for _, container := range m.dashboard.containers.items {
		rows = append(rows, fmt.Sprintf("%s %-14s %-10s %4.0f%% %4.0f%%  %s",
			containerStatusDot(container.Status),
			truncateCells(container.Name, 14),
			truncateCells(container.Status, 10),
			container.CPUPct, container.MemPct,
			truncateCells(container.Ports, 20)))
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
		rows = append(rows, fmt.Sprintf("%s%-24s %s", prefix, truncateCells(unit.Name, 24), unitStateText(unit.Active, unit.Sub)))
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
	rows := make([]string, 0, len(m.dashboard.logs.lines))
	for _, line := range m.dashboard.logs.lines {
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
