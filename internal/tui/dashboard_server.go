package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

const (
	// panelOverhead — верхняя и нижняя границы плитки: ровно столько строк
	// занимает рамка, всё остальное в бюджете высоты — содержимое.
	panelOverhead = 2
	// minPanelHeight — плитка ниже трёх строк бессмысленна: две рамки и строка.
	minPanelHeight = panelOverhead + 1
	// minLogsHeight — у плитки логов первая строка внутри рамки занята
	// состоянием хвоста, поэтому «минимум одна строка лога» — это четыре.
	minLogsHeight = panelOverhead + 2

	// serverFooterHints — список клавиш из макета. Короткий вариант для узких
	// терминалов — его подмножество, а не другой набор: клавиша не должна
	// менять смысл от ширины окна. «h история» и «ctrl+r переподключить»
	// живут в справке «?», в статусбар они по макету не входят.
	serverFooterHints      = "esc назад · d контейнеры · p процессы · l логи · x ssh · r обновить · ? ещё"
	serverFooterHintsShort = "esc назад · p процессы · l логи · x ssh · r обновить · ? ещё"

	// maxIssueRows — потолок высоты плитки ПРОБЛЕМЫ. Несколько заполненных
	// дисков или многострочный stderr от ssh иначе разворачивались бы на
	// пол-экрана, обнуляли бюджет тела и срезали сетку метрик.
	maxIssueRows = 3
)

func (m Model) serverScreenLines(server collect.Metrics) []string {
	width := m.layout.width
	lines := []string{m.serverHeader(server, width)}
	if len(issuesForServer(m.snapshot.Issues, server.Name)) > 0 {
		lines = append(lines, panelBox("ПРОБЛЕМЫ", "r обновить · ctrl+r переподключить", width, issueRows(m.dashboardIssueText(server.Name), width-4))...)
	}
	lines = append(lines, "")
	lines = append(lines, serverMetricGrid(server, m.cpuTrends[server.Name], width)...)
	lines = append(lines, "")

	// Честный остаток: сколько строк реально осталось под средний ряд и логи
	// после шапки, проблем и сетки метрик. Единица — статусбар внизу.
	budget := max(0, m.layout.height-len(lines)-1)
	body, truncated := m.serverBody(server, width, budget)
	lines = append(lines, body...)
	return append(lines, m.serverFooter(server, width, truncated))
}

// issueRows сворачивает список проблем в плитку не выше maxIssueRows строк.
// Последняя видимая строка уступает место счётчику: неполный список должен
// быть виден как неполный, иначе часть проблем пропадает молча.
func issueRows(text string, width int) []string {
	rows := wrapWords(text, width)
	if len(rows) <= maxIssueRows {
		return rows
	}
	return append(rows[:maxIssueRows-1], dimStyle.Render(fmt.Sprintf("… ещё строк: %d", len(rows)-maxIssueRows+1)))
}

// serverBody собирает средний ряд (DOCKER · СЕРВИСЫ · ПОРТЫ) и плитку логов
// ровно в budget строк. Логи по макету занимают остаток высоты, поэтому
// средний ряд берёт необходимый минимум, а излишек уходит логам. Когда
// остатка не хватает, состав сворачивается по приоритету: скрываются порты,
// docker схлопывается в счётчики, логи ужимаются — и об этом говорит признак
// усечения, а не молча обрезанный хвост экрана.
func (m Model) serverBody(server collect.Metrics, width, budget int) ([]string, bool) {
	if budget < minPanelHeight+minLogsHeight {
		return m.serverBodyCompact(server, width, budget), true
	}
	units := m.dashboardUnitsContent()
	if m.layout.twoColumn() {
		return m.serverBodyColumns(server, width, budget, units)
	}
	return m.serverBodyStacked(server, width, budget, units)
}

// serverBodyColumns — DOCKER слева, СЕРВИСЫ и ПОРТЫ справа, логи под ними.
// Колонки ряда равной высоты, поэтому высота ПОРТОВ вычитается из СЕРВИСОВ.
func (m Model) serverBodyColumns(server collect.Metrics, width, budget int, units []string) ([]string, bool) {
	dockerWidth := (width - 2) / 2
	rightWidth := width - 2 - dockerWidth
	dockerRows := m.dashboardDockerContent(dockerWidth - 4)
	// Ширина плитки известна только здесь, а от неё зависит число колонок
	// портов и, значит, их высота — поэтому список строится после раскладки.
	ports := serverPortLines(server, rightWidth-4)

	desired := max(len(units)+len(ports)+2*panelOverhead, len(dockerRows)+panelOverhead)
	// Ряд берёт столько, сколько нужно содержимому, но не больше половины
	// остатка: по макету низ экрана принадлежит логам. Половина уступает паре
	// плиток (СЕРВИСЫ + ПОРТЫ), иначе на бюджете 10–11 строк потолок budget/2
	// отнимал ПОРТЫ там, где логам всё равно оставалось с запасом. Верхняя
	// граница budget-minLogsHeight сохраняет логам их минимум при любом раскладе.
	limit := min(budget-minLogsHeight, max(2*minPanelHeight, budget/2))
	rowHeight := max(minPanelHeight, min(desired, limit))

	// Порты — отдельная плитка под СЕРВИСАМИ: обе с рамками, поэтому в ряд
	// ниже шести строк они не влезают и уступают место сервисам.
	portsHeight, truncated := 0, true
	if rowHeight >= 2*minPanelHeight {
		portsHeight = min(len(ports)+panelOverhead, rowHeight-minPanelHeight)
		// Список не влез по высоте — в заголовке плитки останется полное число
		// портов, поэтому неполноту показываем признаком усечения, а не молча.
		truncated = portsHeight-panelOverhead < len(ports)
	}
	servicesHeight := rowHeight - portsHeight

	right := m.tilePanel(tileSystemd, m.servicesTitle(), "f фильтр · j/k · enter journal", rightWidth,
		fitPanelHeight(units, servicesHeight-panelOverhead, m.systemdScroll(servicesHeight-panelOverhead)))
	if portsHeight > 0 {
		right = append(right, m.portsPanel(server, ports, rightWidth, portsHeight)...)
	}

	left := m.tilePanel(tileDocker, m.dockerTitle(), "d контейнеры", dockerWidth,
		fitPanelHeight(dockerRows, rowHeight-panelOverhead, m.dashboard.tileScrolls[tileDocker]))
	body := strings.Split(joinBoxes(left, right), "\n")
	return append(body, m.serverLogsPanel(width, budget-len(body))...), truncated
}

// serverBodyStacked — та же тройка в одну колонку: на 80 колонках DOCKER,
// СЕРВИСЫ и ПОРТЫ встают друг под друга, состав экрана при этом тот же.
func (m Model) serverBodyStacked(server collect.Metrics, width, budget int, units []string) ([]string, bool) {
	dockerRows := m.dashboardDockerContent(width - 4)
	ports := serverPortLines(server, width-4)

	// Каждая плитка получает желаемую высоту, но не за счёт чужого минимума:
	// иначе длинный список юнитов съедал бы ряд целиком. Первый резерв —
	// под логи, они по макету занимают остаток. Порядок деградации тот же,
	// что в колонках: первыми уходят ПОРТЫ, поэтому DOCKER берёт нужное ему
	// целиком, а ПОРТЫ довольствуются остатком.
	free := budget - minLogsHeight
	servicesHeight := max(minPanelHeight, min(len(units)+panelOverhead, free-2*minPanelHeight))
	free -= servicesHeight

	dockerHeight, truncated := min(len(dockerRows)+panelOverhead, free), false
	if dockerHeight < minPanelHeight {
		dockerHeight, truncated = 0, true
	}
	free -= dockerHeight
	portsHeight := min(len(ports)+panelOverhead, free)
	if portsHeight < minPanelHeight {
		portsHeight, truncated = 0, true
	} else if portsHeight-panelOverhead < len(ports) {
		truncated = true
	}
	free -= portsHeight

	body := []string(nil)
	switch {
	case dockerHeight > 0:
		body = append(body, m.tilePanel(tileDocker, m.dockerTitle(), "d контейнеры", width,
			fitPanelHeight(dockerRows, dockerHeight-panelOverhead, m.dashboard.tileScrolls[tileDocker]))...)
	case free > 0:
		// Плитка не помещается — остаются счётчики: сколько контейнеров живо,
		// видно даже там, где на список строк уже нет.
		body = append(body, fitLine(titleStyle.Render(m.dockerTitle()), width))
	}
	body = append(body, m.tilePanel(tileSystemd, m.servicesTitle(), "f фильтр · j/k · enter journal", width,
		fitPanelHeight(units, servicesHeight-panelOverhead, m.systemdScroll(servicesHeight-panelOverhead)))...)
	if portsHeight > 0 {
		body = append(body, m.portsPanel(server, ports, width, portsHeight)...)
	}
	return append(body, m.serverLogsPanel(width, budget-len(body))...), truncated
}

// portsPanel — плитка ПОРТЫ в обеих раскладках: она входит в обход фокуса, а
// значит обязана и подсвечивать рамку, и прокручиваться j/k. Прокрутка идёт по
// строкам многоколоночной раскладки, а не по записям: колонки читаются сверху
// вниз, и шаг «на одну запись» уводил бы список на треть экрана.
func (m Model) portsPanel(server collect.Metrics, ports []string, width, height int) []string {
	return m.tilePanel(tilePorts, portsTitle(server), "o порты", width,
		fitPanelHeight(ports, height-panelOverhead, m.dashboard.tileScrolls[tilePorts]))
}

// serverBodyCompact — аварийная раскладка для терминала, где высоты на плитки
// уже нет: сводная строка со счётчиками вместо среднего ряда и логи без рамки.
// Блоки не пропадают, а сжимаются до заголовков.
func (m Model) serverBodyCompact(server collect.Metrics, width, budget int) []string {
	if budget <= 0 {
		return nil
	}
	summary := []string{m.servicesTitle(), portsTitle(server), m.dockerTitle()}
	body := []string{fitLine(titleStyle.Render(strings.Join(summary, " · ")), width)}
	if budget == 1 {
		return body
	}
	body = append(body, fitLine(titleStyle.Render(m.dashboardLogsTitle())+"  "+dimStyle.Render(logsStateText(m.dashboard.logs.err, false)), width))
	if budget < 3 {
		return body
	}
	for _, line := range fitLogsHeight(m.dashboardLogsContent(), budget-2, m.dashboard.tileScrolls[tileLogs]) {
		body = append(body, fitLine(line, width))
	}
	return body
}

// serverLogsPanel — плитка логов на остаток высоты: первая строка внутри
// рамки несёт состояние хвоста и подсказки из макета, ниже — сами строки
// лога, привязанные к низу плитки.
func (m Model) serverLogsPanel(width, height int) []string {
	height = max(minLogsHeight, height)
	content := []string{m.serverLogsStatus(width - 4)}
	content = append(content, fitLogsHeight(m.dashboardLogsContent(), height-panelOverhead-1, m.dashboard.tileScrolls[tileLogs])...)
	return m.tilePanel(tileLogs, m.dashboardLogsTitle(), "", width, content)
}

// serverLogsStatus переиспользует строку состояния полноэкранных логов:
// «хвост вкл» звучит одинаково на обоих экранах. Подсказки называют
// фактические клавиши экрана сервера, а не раскладку полноэкранных логов.
func (m Model) serverLogsStatus(width int) string {
	return spread(dimStyle.Render(logsStateText(m.dashboard.logs.err, false)), dimStyle.Render("l логи · s источник"), width)
}

func (m Model) dockerTitle() string {
	return "DOCKER " + containerCountsCompact(m.dashboard.containers.items)
}

func (m Model) servicesTitle() string {
	return fmt.Sprintf("СЕРВИСЫ %d", len(m.filteredDashboardUnits()))
}

func portsTitle(server collect.Metrics) string {
	return fmt.Sprintf("ПОРТЫ %d", len(server.Ports))
}

// serverFooter — статусбар макета. Факта «данные 7s» в шапке макета нет, но
// терять свежесть нельзя: она уезжает в правый край статусбара, туда же, где
// появляется признак усечения экрана.
func (m Model) serverFooter(server collect.Metrics, width int, truncated bool) string {
	state := "данные " + dashboardAge(m.snapshot.Time, server.Time)
	if truncated {
		state = "↕ усечено · " + state
	}
	// Ширины не хватает — сокращаются подсказки, а не состояние: полный список
	// клавиш всё равно лежит в справке «?», а признак усечения экрана и
	// свежесть данных больше нигде не показаны.
	hints := serverFooterHints
	if lipgloss.Width(hints)+lipgloss.Width(state)+1 > width {
		hints = serverFooterHintsShort
	}
	hints = fitLine(hints, max(1, width-lipgloss.Width(state)-1))
	return spread(dimStyle.Render(hints), dimStyle.Render(state), width)
}

func (m Model) serverHeader(server collect.Metrics, width int) string {
	name := titleStyle.Render(server.Name) + "  " + serverStateText(server, m.snapshot.Issues)
	// Факты ужимаются до spread: тот при нехватке места возвращает только
	// левую часть, и на минимальных 60 колонках из шапки исчезали разом ОС,
	// ядра, память и uptime. Усечённый хвост полезнее пустой правой половины.
	facts := fitLine(dimStyle.Render(serverFacts(server, m.serverAddress(server.Name))), width-lipgloss.Width(name)-1)
	return spread(name, facts, width)
}

// serverAddress — адрес сервера из конфига: в шапке макета стоит 10.2.4.18,
// то есть то, куда реально ходит ssh.
func (m Model) serverAddress(name string) string {
	for _, server := range m.configServers() {
		if server.Name == name {
			return server.Host
		}
	}
	return ""
}

// serverStateText — тот же словарь состояний, что на экране флота: «норма»,
// а не «ДОСТУПЕН», иначе один и тот же сервер называется по-разному.
func serverStateText(server collect.Metrics, issues []collect.Issue) string {
	if server.Time.IsZero() {
		return dimStyle.Render("◌ ожидание")
	}
	if !server.Online {
		return criticalStyle.Render("× недоступен")
	}
	if own := issuesForServer(issues, server.Name); len(own) > 0 {
		return warnStyle.Render("⚠ " + own[0].Msg)
	}
	return goodStyle.Render("● норма")
}

func serverFacts(server collect.Metrics, address string) string {
	facts := make([]string, 0, 6)
	// Адрес важнее hostname: по нему сервер и находят, а самоназвание хоста
	// почти всегда повторяет имя из конфига слева в той же строке. Hostname
	// остаётся запасным вариантом, когда адреса в конфиге нет.
	host := address
	if host == "" {
		host = server.Hostname
	}
	for _, fact := range []string{server.Group, host, server.OS} {
		if fact != "" {
			facts = append(facts, fact)
		}
	}
	if server.NumCPU > 0 {
		facts = append(facts, coresText(server.NumCPU))
	}
	if server.MemTotalKB > 0 {
		facts = append(facts, byteValue(float64(server.MemTotalKB)*1024))
	}
	if server.Uptime > 0 {
		facts = append(facts, "up "+compactDuration(server.Uptime))
	}
	return strings.Join(facts, " · ")
}

// serverPortLines — записи портов, разложенные по ширине плитки. Одна колонка
// коротких «0.0.0.0:33364» оставляла бы четыре пятых панели пустыми, а список
// при этом обрезался по высоте.
func serverPortLines(server collect.Metrics, width int) []string {
	if len(server.Ports) == 0 {
		return []string{dimStyle.Render("портов нет")}
	}
	// Адреса выравниваются между собой, чтобы номера портов читались столбиком;
	// потолок в portLocalWidth не даёт длинному IPv6 растянуть все колонки.
	local := 0
	for _, port := range server.Ports {
		local = max(local, lipgloss.Width(port.Local))
	}
	local = min(local, portLocalWidth)
	entries := make([]string, 0, len(server.Ports))
	for _, port := range server.Ports {
		entry := truncateCells(port.Local, local)
		if port.Process != "" {
			entry = padCell(entry, local) + " " + port.Process
		}
		entries = append(entries, entry)
	}
	return portColumns(entries, width)
}

const (
	// portLocalWidth — потолок колонки адреса, прежняя ширина одноколоночного
	// списка.
	portLocalWidth = 22
	// portColumnGap — зазор между колонками портов: тот же, что между плитками.
	portColumnGap = 2
)

// portColumns раскладывает записи по колонкам с чтением сверху вниз, как `ls`:
// в списке адресов глаз идёт по колонке, а не прыгает по строке. Число колонок
// считается от фактической ширины и самой длинной записи, поэтому на узкой
// плитке остаётся одна колонка.
func portColumns(entries []string, width int) []string {
	if len(entries) == 0 {
		return nil
	}
	cell := 1
	for _, entry := range entries {
		cell = max(cell, lipgloss.Width(entry))
	}
	columns := 1
	if width > 0 {
		columns = max(1, (width+portColumnGap)/(cell+portColumnGap))
	}
	columns = min(columns, len(entries))
	rows := (len(entries) + columns - 1) / columns
	// Пересчёт под фактическое число строк: 5 записей в 4 колонки укладываются
	// в 2 строки, но занимают только 3 колонки — четвёртая осталась бы пустой.
	columns = (len(entries) + rows - 1) / rows

	out := make([]string, 0, rows)
	for row := range rows {
		line := strings.Builder{}
		for column := range columns {
			index := column*rows + row
			if index >= len(entries) {
				break
			}
			if column > 0 {
				line.WriteString(strings.Repeat(" ", portColumnGap))
			}
			// Хвост строки не добиваем: рамка плитки дополнит его сама.
			if index+rows < len(entries) {
				line.WriteString(padCell(entries[index], cell))
				continue
			}
			line.WriteString(entries[index])
		}
		out = append(out, line.String())
	}
	return out
}
