package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

const fleetPageSize = 10

type fleetModel struct {
	searching   bool
	expanded    bool
	logbox      bool
	filter      fleetFilter
	preview     bool
	initialized bool
}

func newFleetModel() fleetModel {
	return fleetModel{preview: true, initialized: true}
}

func (m *Model) ensureFleet() {
	if !m.fleet.initialized {
		m.fleet = newFleetModel()
	}
}

func (m *Model) moveFleet(delta int) {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	if len(visible) == 0 {
		return
	}
	position := nearestPosition(visible, m.selected)
	position += delta
	if position < 0 {
		position = 0
	}
	if position >= len(visible) {
		position = len(visible) - 1
	}
	m.selected = visible[position]
}

func (m *Model) selectNearestVisible() {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	if len(visible) == 0 {
		return
	}
	m.selected = visible[nearestPosition(visible, m.selected)]
}

func nearestPosition(indices []int, selected int) int {
	best := 0
	bestDistance := abs(indices[0] - selected)
	for i := 1; i < len(indices); i++ {
		distance := abs(indices[i] - selected)
		if distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	return best
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func fleetRowStyle(selected bool) lipgloss.Style {
	if selected {
		return focusStyle.Copy().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"})
	}
	return dimStyle
}

func fleetScroll(selectedRow, height, total int) int {
	if total <= height {
		return 0
	}
	scroll := selectedRow - height/2
	if scroll < 0 {
		scroll = 0
	}
	if scroll > total-height {
		scroll = total - height
	}
	return scroll
}

func groupedServers(snapshot collect.Snapshot, servers []config.Server, filter fleetFilter) []int {
	visible := filterServers(snapshot, servers, filter)
	order := make([]string, 0)
	buckets := make(map[string][]int)
	for _, index := range visible {
		group := snapshot.Servers[index].Group
		if _, seen := buckets[group]; !seen {
			order = append(order, group)
		}
		buckets[group] = append(buckets[group], index)
	}
	grouped := make([]int, 0, len(visible))
	for _, group := range order {
		grouped = append(grouped, buckets[group]...)
	}
	return grouped
}

func (m Model) configServers() []config.Server {
	if m.config == nil {
		return nil
	}
	return m.config.Servers
}

func (m Model) renderFleet() string {
	m.ensureFleet()
	head := []string{m.fleetHeader(m.layout.width)}
	// Плитки групп и две колонки — вопрос ширины, а не отдельного режима:
	// состав экрана флота один и тот же, ужимается только раскладка.
	if m.layout.twoColumn() {
		head = append(head, m.fleetGroupBox(m.layout.width)...)
	}
	visible := len(groupedServers(m.snapshot, m.configServers(), m.fleet.filter))
	head = append(head, m.fleetContextLine(visible, m.fleetGroupTotal(), m.layout.width))
	head = append(head, m.fleetLogboxLines(m.layout.width)...)
	if m.layout.twoColumn() {
		return strings.Join(head, "\n") + "\n" + m.renderFleetColumns(len(head))
	}
	listLines, _ := m.fleetListLines(m.layout.width)
	footer := dimStyle.Render("↑↓ enter открыть · → детали · / поиск · f проблемы · tab группа · v панель · ? ещё")
	if m.fleet.expanded {
		footer = dimStyle.Render("← свернуть · l логи · p процессы · d контейнеры · x ssh · v панель")
	}
	if m.fleet.logbox {
		footer = dimStyle.Render("esc закрыть ящик · ↑↓ хост · s источник · space пауза · enter весь экран")
	}
	return strings.Join(append(head, listLines...), "\n") + "\n" + footer
}

func (m Model) renderFleetColumns(reserved int) string {
	// Рамка панели забирает две строки, остальное — список: без внешней рамки
	// экрана высота считается от полного терминала.
	contentH := max(1, m.layout.height-panelOverhead-reserved)
	if !m.fleet.preview {
		hint := "v боковая панель · / поиск · tab группа · f проблемы"
		if m.fleet.expanded {
			hint = "← свернуть · v боковая панель · l логи · p процессы · x ssh"
		}
		listLines, selectedRow := m.fleetListLines(m.layout.width - 4)
		scroll := fleetScroll(selectedRow, contentH, len(listLines))
		full := panelBoxStyled("СЕРВЕРЫ", hint, m.layout.width,
			fitPanelHeight(listLines, contentH, scroll), dimStyle)
		return strings.Join(full, "\n")
	}
	rightW := max(30, m.layout.width/4)
	leftW := m.layout.width - rightW - 2
	listLines, selectedRow := m.fleetListLines(leftW - 4)
	scroll := fleetScroll(selectedRow, contentH, len(listLines))
	leftHint := "enter открыть · → детали · / поиск"
	if m.fleet.expanded {
		leftHint = "← свернуть · l логи · p процессы · x ssh"
	}
	left := panelBoxStyled("СЕРВЕРЫ", leftHint, leftW,
		fitPanelHeight(listLines, contentH, scroll), dimStyle)
	right := panelBoxStyled(m.fleetDetailTitle(), "v скрыть · f проблемы · tab группа", rightW,
		fitPanelHeight(m.fleetDetailContent(rightW-4), contentH, 0), dimStyle)
	return joinBoxes(left, right)
}

func (m Model) fleetListLines(width int) ([]string, int) {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	cols := fleetColumnLayout(width)
	lines := []string{dimStyle.Render(cols.header())}
	selectedRow := 0
	previousGroup := ""
	for _, index := range visible {
		if group := m.snapshot.Servers[index].Group; group != "" && group != previousGroup {
			lines = append(lines, titleStyle.Render(group))
			previousGroup = group
		}
		if index == m.selected {
			selectedRow = len(lines)
		}
		lines = append(lines, m.renderFleetRow(index, cols))
		if index == m.selected && m.fleet.expanded {
			lines = append(lines, m.fleetCardLines(m.snapshot.Servers[index], width)...)
		}
	}
	if len(visible) == 0 {
		lines = append(lines, dimStyle.Render("  серверы не найдены"))
	}
	if note := m.fleetHiddenNote(len(visible), m.fleetGroupTotal()); note != "" {
		lines = append(lines, note)
	}
	return lines, selectedRow
}

type fleetColumns struct {
	width int
	name  int
	gap   string
}

func fleetColumnLayout(width int) fleetColumns {
	const numeric = 4 + 4 + 6 + 9 + 11
	const gaps = 5
	name := max(12, min(24, width-4-numeric-gaps))
	gap := max(1, min(10, (width-4-numeric-name)/gaps))
	name = max(name, width-4-numeric-gap*gaps)
	return fleetColumns{width: width, name: name, gap: strings.Repeat(" ", gap)}
}

func (c fleetColumns) row(name, cpu, mem, load, uptime, docker string) string {
	cells := []string{
		fmt.Sprintf("%-*s", c.name, truncateCells(name, c.name)),
		fmt.Sprintf("%4s", cpu),
		fmt.Sprintf("%4s", mem),
		fmt.Sprintf("%6s", load),
		fmt.Sprintf("%9s", uptime),
		docker,
	}
	return strings.Join(cells, c.gap)
}

func (c fleetColumns) header() string {
	return "    " + c.row("ИМЯ", "CPU", "MEM", "LOAD", "UPTIME", "DOCKER")
}

func (m Model) renderFleetRow(index int, cols fleetColumns) string {
	server := m.snapshot.Servers[index]
	selected := index == m.selected
	glyph := statusGlyph(server)
	if selected {
		// У выделенной строки глиф без своего цвета: вложенный ANSI-reset обрывает фон подсветки.
		glyph = statusRune(server)
	}
	cpu, mem, load, uptime := "—", "—", "—", "—"
	if server.Online {
		cpu = fmt.Sprintf("%.0f%%", server.CPUPct)
		mem = fmt.Sprintf("%.0f%%", server.MemPct)
		load = fmt.Sprintf("%.2f", server.Load1)
		uptime = formatUptime(server.Uptime)
	}
	row := "  " + glyph + " " + cols.row(server.Name, cpu, mem, load, uptime, dockerCell(server.Docker))
	return fleetRowStyle(selected).Width(cols.width).Render(row)
}

func dockerCell(d collect.DockerCounts) string {
	if d.Total() == 0 {
		return "—"
	}
	parts := make([]string, 0, 3)
	if d.Running > 0 {
		parts = append(parts, fmt.Sprintf("●%d", d.Running))
	}
	if d.Stopped > 0 {
		parts = append(parts, fmt.Sprintf("○%d", d.Stopped))
	}
	if d.Broken > 0 {
		parts = append(parts, fmt.Sprintf("⚠%d", d.Broken))
	}
	return strings.Join(parts, " ")
}

func statusRune(server collect.Metrics) string {
	if server.Time.IsZero() {
		return "◌"
	}
	if !server.Online {
		return "×"
	}
	return "●"
}

func statusGlyph(server collect.Metrics) string {
	if server.Time.IsZero() {
		return dimStyle.Render("◌")
	}
	if !server.Online {
		return criticalStyle.Render("×")
	}
	return goodStyle.Render("●")
}

func statusText(server collect.Metrics) string {
	if server.Time.IsZero() {
		return "ожидание"
	}
	if !server.Online {
		return "недоступен"
	}
	return "доступен"
}

func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	hours := int(d.Hours())
	if hours >= 24 {
		return fmt.Sprintf("%dd%dh", hours/24, hours%24)
	}
	return fmt.Sprintf("%dh%dm", hours, int(d.Minutes())%60)
}

func issuesForServer(issues []collect.Issue, name string) []collect.Issue {
	result := make([]collect.Issue, 0)
	for _, issue := range issues {
		if issue.Server == name {
			result = append(result, issue)
		}
	}
	return result
}

func truncateCells(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:max(1, width-1)]) + "…"
}
