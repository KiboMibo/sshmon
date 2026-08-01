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
	return panelBoxStyled(title, hint, width, content, dimStyle)
}

func panelBoxStyled(title, hint string, width int, content []string, border lipgloss.Style) []string {
	if width < 6 {
		width = 6
	}
	lines := make([]string, 0, len(content)+2)
	lines = append(lines, borderLine("╭", "╮", title, width, border))
	inner := width - 4
	bar := border.Render("│")
	for _, row := range content {
		lines = append(lines, bar+" "+padCell(row, inner)+" "+bar)
	}
	return append(lines, borderLine("╰", "╯", hint, width, border))
}

func borderLine(left, right, label string, width int, border lipgloss.Style) string {
	if label == "" {
		return border.Render(left + strings.Repeat("─", width-2) + right)
	}
	fill := max(1, width-5-lipgloss.Width(label))
	return border.Render(left+"─ ") + titleStyle.Render(label) + border.Render(" "+strings.Repeat("─", fill)+right)
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

func joinBoxes(cols ...[]string) string {
	parts := make([]string, 0, max(0, len(cols)*2-1))
	for i, col := range cols {
		if i > 0 {
			parts = append(parts, "  ")
		}
		parts = append(parts, strings.Join(col, "\n"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
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

// containerStatusStyle делит контейнеры на те же три группы, что и счётчики
// collect.DockerCounts: запущен, штатно остановлен, всё остальное — проблема.
func containerStatusStyle(status string) lipgloss.Style {
	switch {
	case strings.HasPrefix(status, "Up"):
		return goodStyle
	case strings.HasPrefix(status, "Exited"), strings.HasPrefix(status, "Created"):
		return dimStyle
	default:
		return warnStyle
	}
}

// containerStatusDot — глиф состояния макета. Форма, а не только цвет:
// экран должен читаться и в монохромном терминале.
func containerStatusDot(status string) string {
	glyph := "⚠"
	switch {
	case strings.HasPrefix(status, "Up"):
		glyph = "●"
	case strings.HasPrefix(status, "Exited"), strings.HasPrefix(status, "Created"):
		glyph = "○"
	}
	return containerStatusStyle(status).Render(glyph)
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

// dashboardDockerContent — список контейнеров по макету: имя · статус ·
// аптайм · память. Проценты CPU/MEM ушли: рядом стоит сетка метрик хоста,
// и вторая процентная шкала в той же ширине только спорит с ней.
func (m Model) dashboardDockerContent(width int) []string {
	if !m.dashboardHasDocker() {
		return []string{criticalStyle.Render("DOCKER NOT RUNNING")}
	}
	const statusWidth, uptimeWidth, memoryWidth = 14, 5, 6
	nameWidth := max(8, min(24, width-statusWidth-uptimeWidth-memoryWidth-5))
	rows := make([]string, 0, len(m.dashboard.containers.items))
	for _, container := range m.dashboard.containers.items {
		style := containerStatusStyle(container.Status)
		memory := containerMemory(container.MemUsage)
		rows = append(rows, containerStatusDot(container.Status)+" "+
			padLabel(style.Render(truncateCells(container.Name, nameWidth)), nameWidth)+" "+
			padLabel(truncateCells(containerStatus(container.Status), statusWidth), statusWidth)+" "+
			padLabel(containerUptime(container.RunningFor), uptimeWidth)+" "+
			padLeft(memory, memoryWidth))
	}
	return rows
}

// padLeft выравнивает значение по правому краю колонки шириной width,
// считая ячейки терминала: значения в колонке памяти читают по разряду.
func padLeft(value string, width int) string {
	if pad := width - lipgloss.Width(value); pad > 0 {
		return strings.Repeat(" ", pad) + value
	}
	return value
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
		return []string{criticalStyle.Render(errText(m.dashboard.logs.err))}
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
