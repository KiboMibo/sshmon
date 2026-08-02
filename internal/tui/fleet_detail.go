package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

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
	lines = append(lines, "", titleStyle.Render("ТОП ПО ПАМЯТИ"))
	lines = append(lines, m.fleetTopMemoryLines(server, width)...)
	lines = append(lines,
		"",
		titleStyle.Render("ДЕЙСТВИЯ"),
		dimStyle.Render("enter детали    l логи"),
		dimStyle.Render("p процессы      x ssh"),
	)
	return lines
}

// fleetTopMemoryLines — три самых прожорливых по памяти процесса выбранного
// хоста (макет 3a). Данные берутся из той же диагностики, что и экран
// процессов: второго состояния и второго SSH-запроса на хост не заводим.
func (m Model) fleetTopMemoryLines(server collect.Metrics, width int) []string {
	items := sortProcesses(m.processes.items, processSortMemory)
	if len(items) == 0 {
		switch m.processes.status {
		case diagnosticsLoading:
			return []string{dimStyle.Render("загрузка…")}
		case diagnosticsUnsupported:
			return []string{dimStyle.Render("ps недоступен")}
		case diagnosticsError:
			// Причину видно только здесь: до отдельного экрана процессов
			// пользователь не доходит, а «нет данных» на сорванный ssh-запрос
			// выглядит как «на хосте нет процессов».
			return []string{dimStyle.Render(fitLine("ps: "+errText(m.processes.err), width))}
		default:
			return []string{dimStyle.Render("нет данных")}
		}
	}
	lines := make([]string, 0, fleetTopMemoryCount)
	for _, process := range items[:min(fleetTopMemoryCount, len(items))] {
		name, tail := processNameAndTail(process.Command)
		row := padLabel(truncateCells(name, 12), 12) + padLeft(processMemory(process, server), 6)
		if tail != "" {
			row += "  " + tail
		}
		lines = append(lines, fitLine(row, width))
	}
	return lines
}

const fleetTopMemoryCount = 3

// fleetSidebarVisible — виден ли сайдбар флота с разделом «ТОП ПО ПАМЯТИ».
// Отдельно от startFleetTopProcesses, потому что то же условие проверяет
// обработчик изменения размера окна: сайдбар появляется и исчезает вместе с
// шириной терминала. Раскрытая карточка сайдбар больше не убирает — см.
// renderFleetColumns.
func (m Model) fleetSidebarVisible() bool {
	return m.screen == screenFleet && m.fleet.preview && m.layout.twoColumn()
}

// processMemory переводит долю памяти в абсолютное значение по объёму памяти
// хоста: в макете колонка подписана «6.1G», а ps отдаёт только проценты.
func processMemory(process collect.Process, server collect.Metrics) string {
	if server.MemTotalKB <= 0 {
		return fmt.Sprintf("%.1f%%", process.MemPct)
	}
	return byteValue(process.MemPct / 100 * float64(server.MemTotalKB) * 1024)
}

// processNameAndTail делит командную строку на имя процесса и остаток
// аргументов: в колонку макета попадает «java» и поясняющий хвост.
func processNameAndTail(command string) (string, string) {
	name, tail, _ := strings.Cut(strings.TrimSpace(command), " ")
	if index := strings.LastIndexByte(name, '/'); index >= 0 {
		name = name[index+1:]
	}
	return name, strings.TrimSpace(tail)
}

// scheduleFleetTopProcesses откладывает запрос `ps` до паузы в нажатиях: сам
// запрос уйдёт из applyDebounce, если за inputDebounce курсор больше не двигали.
// Всё, что видно на экране, делаем сразу — иначе сайдбар 200 мс показывал бы
// процессы прежнего хоста под именем нового.
func (m *Model) scheduleFleetTopProcesses() tea.Cmd {
	m.ensureFleet()
	if m.processes.cancel != nil {
		m.processes.cancel()
		m.processes.cancel = nil
	}
	if !m.fleetSidebarVisible() {
		return nil
	}
	m.request++
	m.processes.generation, m.processes.status = m.request, diagnosticsLoading
	m.processes.items = nil
	return debounceTick(debounceTopProcesses, m.processes.generation)
}

// startFleetTopProcesses запрашивает процессы вновь выбранного хоста для
// сайдбара. Механизм общий с экраном процессов — то же состояние и то же
// сообщение с результатом, поэтому переход на экран процессов не начинает всё
// заново, а прошлый запрос снимается по контексту.
func (m *Model) startFleetTopProcesses() tea.Cmd {
	return m.requestFleetTopProcesses(true)
}

// refreshFleetTopProcesses — плановое обновление сайдбара по тику диагностики.
// Хост тот же, поэтому прошлый список остаётся на экране до ответа: обнуляй мы
// его, раздел мигал бы «загрузка…» каждый период опроса.
func (m *Model) refreshFleetTopProcesses() tea.Cmd {
	return m.requestFleetTopProcesses(false)
}

func (m *Model) requestFleetTopProcesses(reset bool) tea.Cmd {
	m.ensureFleet()
	// Отмена — до проверки видимости: сайдбар мог пропасть как раз сейчас
	// («v», сузившийся терминал), и уже идущий `ps` доигрывал бы по SSH ради
	// данных, которые некуда показать.
	if m.processes.cancel != nil {
		m.processes.cancel()
		m.processes.cancel = nil
	}
	// Сайдбара нет — нет и причины ходить по SSH: вне экрана флота и на узком
	// терминале раздел «ТОП ПО ПАМЯТИ» не рисуется. Условие живёт здесь, а не
	// на вызывающей стороне: точек вызова несколько (движение курсора, «v»,
	// «←», возврат с дашборда, первый размер окна).
	if !m.fleetSidebarVisible() {
		return nil
	}
	m.request++
	generation := m.request
	ctx, cancel := context.WithCancel(context.Background())
	m.processes.generation, m.processes.cancel, m.processes.status = generation, cancel, diagnosticsLoading
	if reset {
		// Список от прежнего хоста под именем нового читался бы как его процессы.
		m.processes.items = nil
	}
	return runDiagnostics(ctx, generation, screenProcesses, m.selectedName(), m.collector)
}

func worstIssue(issues []collect.Issue) collect.Issue {
	worst := issues[0]
	for _, issue := range issues {
		if issue.Severity == "crit" {
			return issue
		}
	}
	return worst
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
	worst := worstIssue(issues)
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
