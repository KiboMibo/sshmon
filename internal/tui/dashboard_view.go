package tui

import "strings"

// renderDashboardWorkspace — единственная раскладка экрана сервера. Отдельного
// «узкого» рендера больше нет: состав экрана и клавиши одни и те же на 80 и на
// 160 колонках, различается только число колонок и высота плиток.
func (m Model) renderDashboardWorkspace() string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return titleStyle.Render("sshmon · Дашборд") + "\n\n" + dimStyle.Render("сервер не выбран · esc назад")
	}
	return strings.Join(m.serverScreenLines(m.snapshot.Servers[m.selected]), "\n")
}

func (m Model) tilePanel(tile uint8, title, hint string, width int, content []string) []string {
	return panelBoxStyled(title, hint, width, content, m.tileBorderStyle(tile))
}
