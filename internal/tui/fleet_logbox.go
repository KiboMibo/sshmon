package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

const fleetLogboxLines = 5

func (m *Model) openFleetLogbox() tea.Cmd {
	m.ensureFleet()
	m.fleet.logbox = true
	cmd := m.startLogsStream()
	m.markLogboxSeen()
	return cmd
}

// markLogboxSeen фиксирует отметку «просмотрено»: новыми считаются строки,
// пришедшие после открытия ящика, смены хоста или возврата к хвосту — то есть
// после момента, когда пользователь последний раз видел актуальный конец лога.
func (m *Model) markLogboxSeen() {
	m.fleet.logboxSeen = m.logs.buffer.Total()
}

func (m *Model) closeFleetLogbox() {
	m.fleet.logbox = false
	m.cancelLogsStream()
}

func (m *Model) handleFleetLogboxKey(key tea.KeyMsg) (tea.Cmd, bool) {
	if m.logs.filtering {
		return m.handleLogsKey(key)
	}
	switch key.String() {
	case "enter":
		m.fleet.logbox = false
		m.screen = screenLogs
		return nil, true
	case "esc":
		m.closeFleetLogbox()
		return nil, true
	case "up", "k":
		return m.moveFleetLogbox(-1), true
	case "down", "j":
		return m.moveFleetLogbox(1), true
	case "pgup":
		return m.moveFleetLogbox(-fleetPageSize), true
	case "pgdown":
		return m.moveFleetLogbox(fleetPageSize), true
	case "end":
		m.markLogboxSeen()
	}
	return m.handleLogsKey(key)
}

// moveFleetLogbox двигает выбор по списку хостов вместе с потоком логов:
// заголовок ящика подписан выбранным хостом, поэтому оставлять стрим на
// прежнем нельзя — строки не совпадали бы с заголовком.
func (m *Model) moveFleetLogbox(delta int) tea.Cmd {
	previous := m.selected
	cmd := m.moveFleetBy(delta)
	if m.selected == previous {
		return cmd
	}
	stream := m.startLogsStream()
	m.markLogboxSeen()
	return tea.Batch(cmd, stream)
}

func (m Model) fleetLogboxLines(width int) []string {
	if !m.fleet.logbox {
		return nil
	}
	visible := m.logs.visibleLines()
	body := []string{spread(dimStyle.Render(m.fleetLogboxStatus()), dimStyle.Render(m.fleetLogboxCount()), width-4)}
	start := max(0, len(visible)-fleetLogboxLines)
	for _, line := range visible[start:] {
		body = append(body, fitLine(highlightMatches(line, m.logs.filterInput.Value()), width-4))
	}
	if len(visible) == 0 {
		body = append(body, dimStyle.Render("строк пока нет"))
	}
	// В подсказке ящика только его собственные клавиши: enter/esc/↑↓ ушли в
	// общую строку статусбара внизу экрана.
	return panelBoxStyled("ЛОГИ · "+m.selectedName(), "/ фильтр · w уровень · s источник", width, body, dimStyle)
}

func (m Model) fleetLogboxStatus() string {
	return m.fleetLogboxSource() + " · " + m.fleetLogboxLevel() + " · " + m.logsState()
}

// fleetLogboxSource — короткое имя источника: в ящик помещается «postgres», а
// не «journal/postgres», и вид источника всё равно виден по имени юнита.
func (m Model) fleetLogboxSource() string {
	if len(m.logs.sources) == 0 {
		return "system"
	}
	source := m.logs.sources[m.logs.source]
	if source.Name != "" {
		return source.Name
	}
	return string(source.Kind)
}

// fleetLogboxLevel — «warn+»: уровень в ящике задаёт порог, а не единственную
// категорию, поэтому плюс к имени уровня.
func (m Model) fleetLogboxLevel() string {
	name := logLevelNames[m.logs.level]
	if m.logs.level == logLevelAll {
		return name
	}
	return name + "+"
}

// fleetLogboxCount — счётчик макета «12 новых · 214 из 8 412». Новыми считаются
// строки, пришедшие после последней отметки просмотра; переполнение буфера
// (строки старше maxLines выбрасываются) счётчик новых обнуляет само собой.
func (m Model) fleetLogboxCount() string {
	total := m.logs.buffer.Total()
	hint := fmt.Sprintf("%d из %d", len(m.logs.visibleLines()), total)
	if fresh := total - m.fleet.logboxSeen; fresh > 0 {
		hint = fmt.Sprintf("%d новых · ", fresh) + hint
	}
	return hint
}
