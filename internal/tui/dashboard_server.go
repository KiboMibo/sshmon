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

	serverFooterHints      = "esc назад · p процессы · h история · x ssh · r обновить · ctrl+r переподключить · ? ещё"
	serverFooterHintsShort = "esc назад · p процессы · l логи · x ssh · r обновить · ? ещё"
)

func (m Model) serverScreenLines(server collect.Metrics) []string {
	width := m.layout.width
	lines := []string{m.serverHeader(server, width)}
	if len(issuesForServer(m.snapshot.Issues, server.Name)) > 0 {
		lines = append(lines, panelBox("ПРОБЛЕМЫ", "r обновить · ctrl+r переподключить", width, wrapWords(m.dashboardIssueText(server.Name), width-4))...)
	}
	lines = append(lines, "")
	lines = append(lines, serverMetricGrid(server, width)...)
	lines = append(lines, "")

	// Честный остаток: сколько строк реально осталось под средний ряд и логи
	// после шапки, проблем и сетки метрик. Единица — статусбар внизу.
	budget := max(0, m.layout.height-len(lines)-1)
	body, truncated := m.serverBody(server, width, budget)
	lines = append(lines, body...)
	return append(lines, m.serverFooter(server, width, truncated))
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
	ports := serverPortLines(server)
	if m.layout.twoColumn() {
		return m.serverBodyColumns(server, width, budget, units, ports)
	}
	return m.serverBodyStacked(server, width, budget, units, ports)
}

// serverBodyColumns — DOCKER слева, СЕРВИСЫ и ПОРТЫ справа, логи под ними.
// Колонки ряда равной высоты, поэтому высота ПОРТОВ вычитается из СЕРВИСОВ.
func (m Model) serverBodyColumns(server collect.Metrics, width, budget int, units, ports []string) ([]string, bool) {
	docker := m.dashboardHasDocker()
	dockerWidth, rightWidth := 0, width
	if docker {
		dockerWidth = (width - 2) / 2
		rightWidth = width - 2 - dockerWidth
	}
	dockerRows := m.dashboardDockerContent(dockerWidth - 4)

	desired := len(units) + len(ports) + 2*panelOverhead
	if docker {
		desired = max(desired, len(dockerRows)+panelOverhead)
	}
	// Ряд берёт столько, сколько нужно содержимому, но не больше половины
	// остатка: по макету низ экрана принадлежит логам.
	limit := max(minPanelHeight, min(budget-minLogsHeight, budget/2))
	rowHeight := max(minPanelHeight, min(desired, limit))

	// Порты — отдельная плитка под СЕРВИСАМИ: обе с рамками, поэтому в ряд
	// ниже шести строк они не влезают и уступают место сервисам.
	portsHeight, truncated := 0, true
	if rowHeight >= 2*minPanelHeight {
		portsHeight = min(len(ports)+panelOverhead, rowHeight-minPanelHeight)
		truncated = false
	}
	servicesHeight := rowHeight - portsHeight

	right := m.tilePanel(tileSystemd, m.servicesTitle(), "f фильтр · j/k · enter journal", rightWidth,
		fitPanelHeight(units, servicesHeight-panelOverhead, m.systemdScroll(servicesHeight-panelOverhead)))
	if portsHeight > 0 {
		right = append(right, panelBox(portsTitle(server), "o порты", rightWidth,
			fitPanelHeight(ports, portsHeight-panelOverhead, 0))...)
	}

	body := right
	if docker {
		left := m.tilePanel(tileDocker, m.dockerTitle(), "d контейнеры", dockerWidth,
			fitPanelHeight(dockerRows, rowHeight-panelOverhead, m.dashboard.tileScrolls[tileDocker]))
		body = strings.Split(joinBoxes(left, right), "\n")
	}
	return append(body, m.serverLogsPanel(width, budget-len(body))...), truncated
}

// serverBodyStacked — та же тройка в одну колонку: на 80 колонках DOCKER,
// СЕРВИСЫ и ПОРТЫ встают друг под друга, состав экрана при этом тот же.
func (m Model) serverBodyStacked(server collect.Metrics, width, budget int, units, ports []string) ([]string, bool) {
	docker := m.dashboardHasDocker()
	dockerRows := m.dashboardDockerContent(width - 4)

	// Каждая плитка получает желаемую высоту, но не за счёт чужого минимума:
	// иначе длинный список юнитов съедал бы ряд целиком. Первый резерв —
	// под логи, они по макету занимают остаток.
	free := budget - minLogsHeight
	dockerReserve := 0
	if docker {
		dockerReserve = minPanelHeight
	}
	servicesHeight := max(minPanelHeight, min(len(units)+panelOverhead, free-dockerReserve-minPanelHeight))
	free -= servicesHeight

	dockerHeight, truncated := 0, false
	if docker {
		dockerHeight = min(len(dockerRows)+panelOverhead, free-minPanelHeight)
		if dockerHeight < minPanelHeight {
			dockerHeight, truncated = 0, true
		}
		free -= dockerHeight
	}
	portsHeight := min(len(ports)+panelOverhead, free)
	if portsHeight < minPanelHeight {
		portsHeight, truncated = 0, true
	}
	free -= portsHeight

	body := []string(nil)
	switch {
	case dockerHeight > 0:
		body = append(body, m.tilePanel(tileDocker, m.dockerTitle(), "d контейнеры", width,
			fitPanelHeight(dockerRows, dockerHeight-panelOverhead, m.dashboard.tileScrolls[tileDocker]))...)
	case docker && free > 0:
		// Плитка не помещается — остаются счётчики: сколько контейнеров живо,
		// видно даже там, где на список строк уже нет.
		body = append(body, fitLine(titleStyle.Render(m.dockerTitle()), width))
	}
	body = append(body, m.tilePanel(tileSystemd, m.servicesTitle(), "f фильтр · j/k · enter journal", width,
		fitPanelHeight(units, servicesHeight-panelOverhead, m.systemdScroll(servicesHeight-panelOverhead)))...)
	if portsHeight > 0 {
		body = append(body, panelBox(portsTitle(server), "o порты", width, fitPanelHeight(ports, portsHeight-panelOverhead, 0))...)
	}
	return append(body, m.serverLogsPanel(width, budget-len(body))...), truncated
}

// serverBodyCompact — аварийная раскладка для терминала, где высоты на плитки
// уже нет: сводная строка со счётчиками вместо среднего ряда и логи без рамки.
// Блоки не пропадают, а сжимаются до заголовков.
func (m Model) serverBodyCompact(server collect.Metrics, width, budget int) []string {
	if budget <= 0 {
		return nil
	}
	summary := []string{m.servicesTitle(), portsTitle(server)}
	if m.dashboardHasDocker() {
		summary = append(summary, m.dockerTitle())
	}
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
// «хвост включён» звучит одинаково на обоих экранах. Подсказки называют
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
	return spread(name, dimStyle.Render(serverFacts(server)), width)
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

func serverFacts(server collect.Metrics) string {
	facts := make([]string, 0, 6)
	for _, fact := range []string{server.Group, server.Hostname, server.OS} {
		if fact != "" {
			facts = append(facts, fact)
		}
	}
	if server.NumCPU > 0 {
		facts = append(facts, fmt.Sprintf("%d ядер", server.NumCPU))
	}
	if server.MemTotalKB > 0 {
		facts = append(facts, byteValue(float64(server.MemTotalKB)*1024))
	}
	if server.Uptime > 0 {
		facts = append(facts, "up "+compactDuration(server.Uptime))
	}
	return strings.Join(facts, " · ")
}

func serverPortLines(server collect.Metrics) []string {
	if len(server.Ports) == 0 {
		return []string{dimStyle.Render("портов нет")}
	}
	rows := make([]string, 0, len(server.Ports))
	for _, port := range server.Ports {
		rows = append(rows, fmt.Sprintf("%-22s %s", truncateCells(port.Local, 22), port.Process))
	}
	return rows
}
