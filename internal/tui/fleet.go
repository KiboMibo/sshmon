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

// fleetScroll — окно списка вокруг выделенной строки. span — сколько строк
// занимает выделенное вместе с врезанным под ним блоком (карточка или ящик
// логов): центрируем блок целиком, иначе он вылезал бы за нижний край. Если
// блок в окно не влезает вовсе, строка встаёт наверх окна — потерять её нельзя,
// потерять хвост блока можно.
func fleetScroll(selectedRow, span, height, total int) int {
	if total <= height {
		return 0
	}
	scroll := selectedRow - max(0, (height-max(1, span))/2)
	if scroll > total-height {
		scroll = total - height
	}
	if scroll > selectedRow {
		scroll = selectedRow
	}
	if scroll < 0 {
		scroll = 0
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
	// тесноте: рамка панели и две строки. Плитки групп ужимаются до него, а не
	// за его счёт.
	fleetListMin = panelOverhead + 2
	// fleetLogboxMin — минимальная высота ящика логов: рамка, строка состояния
	// и одна строка лога. Ящик врезан в список, поэтому это добавка к минимуму
	// самого списка, а не отдельная доля экрана.
	fleetLogboxMin = panelOverhead + 2
)

func (m Model) renderFleet() string {
	m.ensureFleet()
	width := m.layout.width
	// Высота делится честно: сумма частей обязана уложиться в терминал, иначе
	// composeScreen срежет низ последней панели вместе с её рамкой. Первыми
	// уступают плитки групп, список хостов не исчезает никогда.
	budget := max(fleetListMin, m.layout.height-fleetFixedLines)
	// С открытым ящиком список держит за собой и его минимальную высоту: ящик
	// врезан в список, и без этой добавки плитки групп срезали бы ему рамку.
	listMin := fleetListMin
	if m.fleet.logbox {
		listMin += fleetLogboxMin
	}
	head := []string{m.fleetHeader(width)}
	// Плитки групп и две колонки — вопрос ширины, а не отдельного режима:
	// состав экрана флота один и тот же, ужимается только раскладка.
	if m.layout.twoColumn() {
		if tiles := m.fleetGroupBox(width); len(tiles) <= budget-listMin {
			head = append(head, tiles...)
			budget -= len(tiles)
		}
	}
	visible := len(groupedServers(m.snapshot, m.configServers(), m.fleet.filter))
	head = append(head, m.fleetContextLine(visible, m.fleetGroupTotal(), width))
	if m.layout.twoColumn() {
		head = append(head, strings.Split(m.renderFleetColumns(budget), "\n")...)
	} else {
		// Прокрутка нужна и здесь: ниже 100 колонок это единственная раскладка
		// списка, и без окна выделенная строка уезжает за нижний край кадра.
		listLines, selectedRow, span := m.fleetListLines(width, budget-1)
		head = append(head, fitPanelHeight(listLines, budget, fleetScroll(selectedRow, span, budget, len(listLines)))...)
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
		listLines, selectedRow, span := m.fleetListLines(m.layout.width-4, contentH-1)
		scroll := fleetScroll(selectedRow, span, contentH, len(listLines))
		full := panelBoxStyled("СЕРВЕРЫ", "", m.layout.width,
			fitPanelHeight(listLines, contentH, scroll), dimStyle)
		return strings.Join(full, "\n")
	}
	rightW := max(30, m.layout.width/4)
	leftW := m.layout.width - rightW - 2
	listLines, selectedRow, span := m.fleetListLines(leftW-4, contentH-1)
	scroll := fleetScroll(selectedRow, span, contentH, len(listLines))
	left := panelBoxStyled("СЕРВЕРЫ", "", leftW,
		fitPanelHeight(listLines, contentH, scroll), dimStyle)
	right := panelBoxStyled(m.fleetDetailTitle(), "", rightW,
		fitPanelHeight(m.fleetDetailContent(rightW-4), contentH, 0), dimStyle)
	return joinBoxes(left, right)
}

// fleetListLines собирает строки списка. boxHeight — сколько строк остаётся под
// врезанный блок выбранного хоста. Кроме строк возвращает номер выделенной
// строки и span — её высоту вместе с блоком: по ним считается прокрутка окна.
func (m Model) fleetListLines(width, boxHeight int) ([]string, int, int) {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	cols := fleetColumnLayout(width, m.fleet.expanded)
	lines := []string{dimStyle.Render(cols.header())}
	selectedRow, span := 0, 1
	previousGroup := ""
	for _, index := range visible {
		if group := m.snapshot.Servers[index].Group; group != "" && group != previousGroup {
			lines = append(lines, titleStyle.Render(group))
			previousGroup = group
		}
		if index != m.selected {
			lines = append(lines, m.renderFleetRow(index, cols))
			continue
		}
		selectedRow = len(lines)
		lines = append(lines, m.renderFleetRow(index, cols))
		inset := m.fleetInsetLines(m.snapshot.Servers[index], width, boxHeight)
		lines = append(lines, inset...)
		span = 1 + len(inset)
	}
	if len(visible) == 0 {
		lines = append(lines, dimStyle.Render("  серверы не найдены"))
	}
	if note := m.fleetHiddenNote(len(visible), m.fleetGroupTotal()); note != "" {
		lines = append(lines, note)
	}
	return lines, selectedRow, span
}

// fleetInsetLines — блок, врезанный в список сразу под выбранной строкой.
// Ящик логов и раскрытая карточка занимают одно и то же место, поэтому при обоих
// включённых режимах показываем ящик: его открывают последним и именно к нему
// уходят ↑↓, а детали хоста всё равно видны в сайдбаре и на экране сервера.
func (m Model) fleetInsetLines(server collect.Metrics, width, height int) []string {
	if m.fleet.logbox {
		return m.fleetLogboxLines(width, height)
	}
	if m.fleet.expanded {
		return m.fleetCardLines(server, width)
	}
	return nil
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
	// lead и inner — зазоры блока чисел: lead стоит между «имя + состояние» и
	// первой числовой колонкой, inner — между самими числами. Свободная ширина
	// панели делится между ними поровну. Прежняя единственная распорка перед
	// блоком уводила числа к правой рамке, где они слипались в «4%  51%  0.00»,
	// а середина строки оставалась пустой.
	lead   int
	inner  int
	uptime bool
	docker bool
}

// fleetNumericWidths — ширины числовых колонок в порядке отрисовки. DISK стоит
// сразу за MEM: три процентные шкалы читаются вместе, а load — уже другая
// величина. UPTIME и DOCKER принадлежат режиму деталей.
func (c fleetColumns) numericWidths() []int {
	widths := []int{4, 4, 4, 6} // cpu, mem, disk, load
	if c.uptime {
		widths = append(widths, 7)
	}
	if c.docker {
		widths = append(widths, fleetDockerWidth)
	}
	return widths
}

// fleetColumnLayout выбирает состав колонок по режиму и ширине: UPTIME и DOCKER
// принадлежат режиму деталей (макет 3b), в списке с сайдбаром их нет (макет 3a).
// На узком терминале они же уходят первыми — имя хоста, состояние и три
// процентные колонки должны оставаться читаемыми на любой ширине.
func fleetColumnLayout(width int, detailed bool) fleetColumns {
	cols := fleetColumns{
		width:  width,
		state:  fleetStateWidth,
		gap:    strings.Repeat(" ", fleetGapWidth),
		lead:   fleetGapWidth,
		inner:  fleetGapWidth,
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
	// Имя хоста длиннее fleetNameMax не читается лучше, поэтому весь остаток
	// ширины уходит в зазоры блока чисел — поровну, чтобы интервалы между
	// колонками росли вместе с панелью, а не только отступ перед ними. Остаток
	// от деления достаётся lead: так последняя колонка остаётся у правого края
	// панели, вплотную к краю заголовка.
	extra := max(0, width-cols.fixed()-(cols.name-fleetNameMin))
	gaps := len(cols.numericWidths())
	cols.lead += extra/gaps + extra%gaps
	cols.inner += extra / gaps
	return cols
}

// fixed — ширина строки при минимальном имени хоста и базовых зазорах: по ней
// решается, какие колонки ещё помещаются и сколько места остаётся имени.
func (c fleetColumns) fixed() int {
	total := fleetNameMin + c.state
	widths := c.numericWidths()
	for _, w := range widths {
		total += w
	}
	// Зазоров на один больше, чем числовых колонок: между именем и состоянием,
	// перед блоком чисел и между самими числами.
	return total + fleetGapWidth*(len(widths)+1) + len([]rune(fleetMarker))
}

func (c fleetColumns) row(name, state, cpu, mem, disk, load, uptime, docker string) string {
	// padLabel/padLeft, а не «%-*s» и «%7s»: ячейка состояния приходит цветной,
	// «—» и «140д» — многобайтные, и ширину нужно считать по терминальным
	// ячейкам, а не по байтам строки.
	values := []string{cpu, mem, disk, load}
	if c.uptime {
		values = append(values, uptime)
	}
	if c.docker {
		values = append(values, docker)
	}
	row := padLabel(truncateCells(name, c.name), c.name) + c.gap +
		padLabel(state, c.state) + strings.Repeat(" ", c.lead)
	for i, width := range c.numericWidths() {
		if i > 0 {
			row += strings.Repeat(" ", c.inner)
		}
		if c.docker && i == len(values)-1 {
			// Ячейка контейнеров держит ширину даже пустой и выровнена влево:
			// без этого последняя колонка гуляла бы по строке и заголовок
			// DOCKER стоял бы не над ней.
			row += padLabel(truncateCells(values[i], width), width)
			continue
		}
		row += padLeft(values[i], width)
	}
	return row
}

func (c fleetColumns) header() string {
	return strings.Repeat(" ", len([]rune(fleetMarker))) +
		c.row("ХОСТ", "СОСТ", "CPU", "MEM", "DISK", "LOAD", "UPTIME", "DOCKER")
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
	cpu, mem, disk, load, uptime := "—", "—", "—", "—", "—"
	if server.Online {
		cpu = fmt.Sprintf("%.0f%%", server.CPUPct)
		mem = fmt.Sprintf("%.0f%%", server.MemPct)
		// Формат и отсутствие своего цвета — как у MEM: у выделенной строки
		// вложенный ANSI-reset оборвал бы фон подсветки, поэтому цветом в этой
		// таблице говорит только колонка СОСТ.
		if usage, ok := rootDiskUsage(server.Disks); ok {
			disk = fmt.Sprintf("%.0f%%", usage.UsedPct)
		}
		load = fmt.Sprintf("%.2f", server.Load1)
		uptime = formatUptime(server.Uptime)
	}
	// Маркер, а не только фон: выделение обязано читаться и в монохромном
	// терминале, где подсветка фона может не отрисоваться вовсе.
	marker := strings.Repeat(" ", len([]rune(fleetMarker)))
	if selected {
		marker = fleetMarker
	}
	row := marker + cols.row(server.Name, state, cpu, mem, disk, load, uptime, dockerCell(server.Docker))
	// fitLine до Width: слишком длинную строку lipgloss переносит на вторую, и
	// подсветка выделения растекалась бы на две строки списка.
	return fleetRowStyle(selected).Width(cols.width).Render(fitLine(row, cols.width))
}

// dockerCell — ячейка колонки DOCKER, тот же вид «●7 ○2 ⚠1», что в заголовке
// плитки DOCKER экрана сервера: один и тот же факт не должен выглядеть на двух
// экранах по-разному. Прочерк означает «docker не ответил», «●0» — «docker есть,
// контейнеров нет»: одним прочерком эти состояния путать нельзя.
func dockerCell(d collect.DockerCounts) string {
	if !d.Known {
		return "—"
	}
	parts := make([]string, 0, 3)
	parts = append(parts, fmt.Sprintf("●%d", d.Running))
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
