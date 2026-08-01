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
		return focusStyle.Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"})
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

const (
	// fleetFixedLines — строки, которые есть на экране всегда: шапка, строка
	// контекста области видимости и строка подсказок внизу.
	fleetFixedLines = 3
	// fleetListMin — сколько строк список хостов удерживает за собой при любой
	// тесноте: рамка панели и две строки. Плитки групп и ящик логов ужимаются
	// до него, а не за его счёт — под ящиком в макете 3d всегда видны хосты.
	fleetListMin = panelOverhead + 2
	// fleetLogboxMin — минимальная высота ящика логов: рамка, строка состояния
	// и одна строка лога.
	fleetLogboxMin = panelOverhead + 2
)

func (m Model) renderFleet() string {
	m.ensureFleet()
	width := m.layout.width
	// Высота делится честно: сумма частей обязана уложиться в терминал, иначе
	// composeScreen срежет низ последней панели вместе с её рамкой. Уступают по
	// приоритету — сначала плитки групп, затем ящик логов, список хостов не
	// исчезает никогда.
	budget := max(fleetListMin, m.layout.height-fleetFixedLines)
	logboxReserve := 0
	if m.fleet.logbox {
		logboxReserve = fleetLogboxMin
	}
	head := []string{m.fleetHeader(width)}
	// Плитки групп и две колонки — вопрос ширины, а не отдельного режима:
	// состав экрана флота один и тот же, ужимается только раскладка.
	if m.layout.twoColumn() {
		if tiles := m.fleetGroupBox(width); len(tiles) <= budget-fleetListMin-logboxReserve {
			head = append(head, tiles...)
			budget -= len(tiles)
		}
	}
	visible := len(groupedServers(m.snapshot, m.configServers(), m.fleet.filter))
	head = append(head, m.fleetContextLine(visible, m.fleetGroupTotal(), width))
	logbox := m.fleetLogboxLines(width, budget-fleetListMin)
	head = append(head, logbox...)
	budget -= len(logbox)
	if m.layout.twoColumn() {
		head = append(head, strings.Split(m.renderFleetColumns(budget), "\n")...)
	} else {
		// Прокрутка нужна и здесь: ниже 100 колонок это единственная раскладка
		// списка, и без окна выделенная строка уезжает за нижний край кадра.
		listLines, selectedRow := m.fleetListLines(width)
		head = append(head, fitPanelHeight(listLines, budget, fleetScroll(selectedRow, budget, len(listLines)))...)
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

// renderFleetColumns рисует колонки списка в отведённые budget строк экрана:
// рамка панели забирает из них две, остальное достаётся содержимому.
func (m Model) renderFleetColumns(budget int) string {
	contentH := max(1, budget-panelOverhead)
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
	fleetStateWidth  = 13 // «⚠ память 98%» — самая длинная формулировка колонки СОСТ
	fleetNameMin     = 12
	fleetNameMax     = 24
	fleetGapWidth    = 2
	fleetDockerWidth = 11   // «●12 ○3 ⚠1» — самый широкий вид ячейки контейнеров
	fleetMarker      = "▍ " // маркер выделенной строки, ширина учтена в fixed()
)

type fleetColumns struct {
	width int
	name  int
	state int
	gap   string
	// stretch — распорка между блоком «имя + состояние» и блоком чисел. Без неё
	// таблица кончалась там, где кончалось самое длинное имя хоста, и правая
	// половина панели оставалась пустой.
	stretch int
	uptime  bool
	docker  bool
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
	// Остаток ширины уходит целиком в распорку: имя хоста длиннее fleetNameMax
	// не читается лучше, а блок чисел на правом краю панели совпадает с краем
	// заголовка и не висит в воздухе посреди рамки.
	cols.stretch = max(0, width-cols.fixed()-(cols.name-fleetNameMin))
	return cols
}

// fixed — ширина строки при минимальном имени хоста и без распорки: по ней
// решается, какие колонки ещё помещаются и сколько места остаётся имени.
func (c fleetColumns) fixed() int {
	total, count := fleetNameMin+c.state+4+4+6, 5
	if c.uptime {
		total, count = total+7, count+1
	}
	if c.docker {
		total, count = total+fleetDockerWidth, count+1
	}
	return total + fleetGapWidth*(count-1) + len([]rune(fleetMarker))
}

func (c fleetColumns) row(name, state, cpu, mem, load, uptime, docker string) string {
	// padLabel/padLeft, а не «%-*s» и «%7s»: ячейка состояния приходит цветной,
	// «—» и «140д» — многобайтные, и ширину нужно считать по терминальным
	// ячейкам, а не по байтам строки.
	left := []string{
		padLabel(truncateCells(name, c.name), c.name),
		padLabel(state, c.state),
	}
	right := []string{
		padLeft(cpu, 4),
		padLeft(mem, 4),
		padLeft(load, 6),
	}
	if c.uptime {
		right = append(right, padLeft(uptime, 7))
	}
	if c.docker {
		// Ячейка контейнеров держит ширину даже пустой: без неё последняя
		// колонка гуляла бы по строке и заголовок DOCKER стоял бы не над ней.
		right = append(right, padLabel(truncateCells(docker, fleetDockerWidth), fleetDockerWidth))
	}
	return strings.Join(left, c.gap) + c.gap + strings.Repeat(" ", c.stretch) + strings.Join(right, c.gap)
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
	// fitLine до Width: слишком длинную строку lipgloss переносит на вторую, и
	// подсветка выделения растекалась бы на две строки списка.
	return fleetRowStyle(selected).Width(cols.width).Render(fitLine(row, cols.width))
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
