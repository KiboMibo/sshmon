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
	searching bool
	expanded  bool
	logbox    bool
	filter    fleetFilter
	preview   bool
	// logboxSeen — сколько строк было в буфере, когда пользователь последний раз
	// смотрел на ящик (открытие ящика, смена хоста, прокрутка к хвосту). Всё,
	// что пришло позже, счётчик показывает как «новые».
	logboxSeen  int
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
	// Строка подсказок внизу одна на весь экран, поэтому её место резервируем
	// до расчёта высоты списка.
	reserved := len(head) + 1
	if m.layout.twoColumn() {
		head = append(head, strings.Split(m.renderFleetColumns(reserved), "\n")...)
	} else {
		listLines, _ := m.fleetListLines(m.layout.width)
		head = append(head, listLines...)
	}
	return strings.Join(append(head, m.fleetFooter()), "\n")
}

// fleetFooter — единственная строка подсказок экрана (макет 3a): у панелей
// своих подсказок на нижних рамках больше нет, иначе одни и те же клавиши
// перечислены на экране дважды.
func (m Model) fleetFooter() string {
	switch {
	case m.fleet.logbox:
		// ↑↓ в ящике переключают хост вместе с потоком, а не строки лога.
		return dimStyle.Render("esc закрыть ящик · enter логи на весь экран · ↑↓ хост")
	case m.fleet.expanded:
		return dimStyle.Render("← свернуть · enter весь экран · / поиск · d контейнеры · ? ещё")
	default:
		return dimStyle.Render("↑↓ enter открыть · / поиск · f проблемы · ? ещё")
	}
}

func (m Model) renderFleetColumns(reserved int) string {
	// Рамка панели забирает две строки, остальное — список: без внешней рамки
	// экрана высота считается от полного терминала.
	contentH := max(1, m.layout.height-panelOverhead-reserved)
	// Раскрытая карточка и есть детали выбранного хоста, поэтому в этом режиме
	// сайдбар уступает ей место (макет 3b): иначе те же детали рисуются дважды,
	// а карточка ужимается в узкую левую колонку.
	if m.fleet.expanded || !m.fleet.preview {
		listLines, selectedRow := m.fleetListLines(m.layout.width - 4)
		scroll := fleetScroll(selectedRow, contentH, len(listLines))
		full := panelBoxStyled("СЕРВЕРЫ", "", m.layout.width,
			fitPanelHeight(listLines, contentH, scroll), dimStyle)
		return strings.Join(full, "\n")
	}
	rightW := max(30, m.layout.width/4)
	leftW := m.layout.width - rightW - 2
	listLines, selectedRow := m.fleetListLines(leftW - 4)
	scroll := fleetScroll(selectedRow, contentH, len(listLines))
	left := panelBoxStyled("СЕРВЕРЫ", "", leftW,
		fitPanelHeight(listLines, contentH, scroll), dimStyle)
	right := panelBoxStyled(m.fleetDetailTitle(), "", rightW,
		fitPanelHeight(m.fleetDetailContent(rightW-4), contentH, 0), dimStyle)
	return joinBoxes(left, right)
}

func (m Model) fleetListLines(width int) ([]string, int) {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	cols := fleetColumnLayout(width, m.fleet.expanded)
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

const (
	fleetStateWidth = 13 // «⚠ память 98%» — самая длинная формулировка колонки СОСТ
	fleetNameMin    = 12
	fleetNameMax    = 24
	fleetGapWidth   = 2
	fleetMarker     = "▍ " // маркер выделенной строки, ширина учтена в fixed()
)

type fleetColumns struct {
	width  int
	name   int
	state  int
	gap    string
	uptime bool
	docker bool
}

// fleetColumnLayout выбирает состав колонок по режиму и ширине: UPTIME и DOCKER
// принадлежат режиму деталей (макет 3b), в списке с сайдбаром их нет (макет 3a).
// На узком терминале они же уходят первыми — имя хоста и состояние должны
// оставаться читаемыми на любой ширине.
func fleetColumnLayout(width int, detailed bool) fleetColumns {
	cols := fleetColumns{
		width:  width,
		state:  fleetStateWidth,
		gap:    strings.Repeat(" ", fleetGapWidth),
		uptime: detailed,
		docker: detailed,
	}
	for cols.fixed() > width && (cols.docker || cols.uptime) {
		if cols.docker {
			cols.docker = false
			continue
		}
		cols.uptime = false
	}
	cols.name = fleetNameMin + max(0, min(fleetNameMax-fleetNameMin, width-cols.fixed()))
	return cols
}

// fixed — ширина строки при минимальном имени хоста: по ней решается, какие
// колонки ещё помещаются и сколько места остаётся имени.
func (c fleetColumns) fixed() int {
	total, count := fleetNameMin+c.state+4+4+6, 5
	if c.uptime {
		total, count = total+7, count+1
	}
	if c.docker {
		total, count = total+11, count+1
	}
	return total + fleetGapWidth*(count-1) + len([]rune(fleetMarker))
}

func (c fleetColumns) row(name, state, cpu, mem, load, uptime, docker string) string {
	cells := []string{
		// padLabel, а не «%-*s»: ячейка состояния приходит цветной, и ширину
		// нужно считать по терминальным ячейкам, а не по байтам ANSI-строки.
		padLabel(truncateCells(name, c.name), c.name),
		padLabel(state, c.state),
		fmt.Sprintf("%4s", cpu),
		fmt.Sprintf("%4s", mem),
		fmt.Sprintf("%6s", load),
	}
	if c.uptime {
		cells = append(cells, fmt.Sprintf("%7s", uptime))
	}
	if c.docker {
		cells = append(cells, docker)
	}
	return strings.Join(cells, c.gap)
}

func (c fleetColumns) header() string {
	return strings.Repeat(" ", len([]rune(fleetMarker))) +
		c.row("ХОСТ", "СОСТ", "CPU", "MEM", "LOAD", "UPTIME", "DOCKER")
}

func (m Model) renderFleetRow(index int, cols fleetColumns) string {
	server := m.snapshot.Servers[index]
	selected := index == m.selected
	issues := issuesForServer(m.snapshot.Issues, server.Name)
	text := statusRune(server, issues) + " " + truncateCells(statusText(server, issues), max(1, cols.state-2))
	state := statusStyle(server, issues).Render(text)
	if selected {
		// У выделенной строки состояние без своего цвета: вложенный ANSI-reset
		// обрывает фон подсветки.
		state = text
	}
	cpu, mem, load, uptime := "—", "—", "—", "—"
	if server.Online {
		cpu = fmt.Sprintf("%.0f%%", server.CPUPct)
		mem = fmt.Sprintf("%.0f%%", server.MemPct)
		load = fmt.Sprintf("%.2f", server.Load1)
		uptime = formatUptime(server.Uptime)
	}
	// Маркер, а не только фон: выделение обязано читаться и в монохромном
	// терминале, где подсветка фона может не отрисоваться вовсе.
	marker := strings.Repeat(" ", len([]rune(fleetMarker)))
	if selected {
		marker = fleetMarker
	}
	row := marker + cols.row(server.Name, state, cpu, mem, load, uptime, dockerCell(server.Docker))
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

func statusRune(server collect.Metrics, issues []collect.Issue) string {
	switch {
	case server.Time.IsZero():
		return "◌"
	case !server.Online:
		return "×"
	case len(issues) > 0:
		return "⚠"
	default:
		return "●"
	}
}

func statusStyle(server collect.Metrics, issues []collect.Issue) lipgloss.Style {
	switch {
	case server.Time.IsZero():
		return dimStyle
	case !server.Online:
		return criticalStyle
	case len(issues) > 0:
		if worstIssue(issues).Severity == "crit" {
			return criticalStyle
		}
		return warnStyle
	default:
		return goodStyle
	}
}

// statusText — текст колонки СОСТ рядом с глифом: состояние должно читаться
// и без цвета, а форма глифа различает только три случая из четырёх.
func statusText(server collect.Metrics, issues []collect.Issue) string {
	switch {
	case server.Time.IsZero():
		return "ожидание"
	case !server.Online:
		return "нет связи"
	case len(issues) > 0:
		return worstIssue(issues).Msg
	default:
		return "норма"
	}
}

func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	// compactDuration уже даёт вид макета («140д»); вторая реализация того же
	// формата разъезжалась бы с колонкой аптайма контейнеров.
	return compactDuration(d)
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
