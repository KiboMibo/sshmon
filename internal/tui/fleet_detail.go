package tui

import "github.com/kibomibo/sshmon/internal/collect"

// fleetDetailTitle подписывает правую колонку Fleet именем выбранного сервера,
// либо нейтральным заголовком, когда выбор вне диапазона.
func (m Model) fleetDetailTitle() string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return "ПОДРОБНОСТИ"
	}
	return truncateCells(m.snapshot.Servers[m.selected].Name, 20)
}

// fleetDetailContent раскрывает выбранный хост так же, как правая колонка
// макета: строка состояния, перечень причин «что не так» и действия, которые
// действительно доступны с этого экрана.
func (m Model) fleetDetailContent(width int) []string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return []string{dimStyle.Render("сервер не выбран")}
	}
	server := m.snapshot.Servers[m.selected]
	issues := issuesForServer(m.snapshot.Issues, server.Name)
	lines := []string{
		fleetDetailStatus(server, issues, width),
		server.Hostname,
		"",
		titleStyle.Render("ЧТО НЕ ТАК"),
	}
	lines = append(lines, fleetIssueLines(server, issues, width)...)
	lines = append(lines,
		"",
		titleStyle.Render("ДЕЙСТВИЯ"),
		dimStyle.Render("enter  детали"),
		dimStyle.Render("/      поиск"),
		dimStyle.Render("f      только проблемные"),
	)
	return lines
}

func fleetDetailStatus(server collect.Metrics, issues []collect.Issue, width int) string {
	if server.Time.IsZero() {
		return dimStyle.Render("◌ ожидание")
	}
	if !server.Online {
		return criticalStyle.Render("× нет связи")
	}
	if len(issues) == 0 {
		return goodStyle.Render("● норма")
	}
	worst := issues[0]
	for _, issue := range issues {
		if issue.Severity == "crit" {
			worst = issue
			break
		}
	}
	style := warnStyle
	if worst.Severity == "crit" {
		style = criticalStyle
	}
	return style.Render(truncateCells("⚠ "+worst.Msg, width))
}

func fleetIssueLines(server collect.Metrics, issues []collect.Issue, width int) []string {
	if !server.Online && server.Err != "" {
		return bulletLines(server.Err, width)
	}
	if len(issues) == 0 {
		return []string{dimStyle.Render("• замечаний нет")}
	}
	var lines []string
	for _, issue := range issues {
		lines = append(lines, bulletLines(issue.Msg, width)...)
	}
	return lines
}

func bulletLines(text string, width int) []string {
	rows := wrapWords(text, max(1, width-2))
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		if i == 0 {
			lines = append(lines, "• "+row)
			continue
		}
		lines = append(lines, "  "+row)
	}
	return lines
}
