package tui

import tea "github.com/charmbracelet/bubbletea"

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
		// Тот же путь, что и «l» из раскрытой карточки: без workspace экран
		// сервера под логами никогда не грузился, и «esc» высаживал на пустой
		// дашборд.
		_, cmd := m.openFromFleet(screenLogs)
		return cmd, true
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
		return nil, true
	case " ", "/", "w", "s", "left", "right", "r":
		// Белый список: в ящик уходят только клавиши самого потока. Остальное
		// («c» — чат, «t», «y», «n»/«N», «W») работает с выделенной строкой,
		// которой в ящике нет, и достаётся глобальному обработчику.
		return m.handleLogsKey(key)
	}
	return nil, false
}

// moveFleetLogbox двигает выбор по списку хостов вместе с потоком логов:
// заголовок ящика подписан выбранным хостом, поэтому оставлять стрим на
// прежнем нельзя — строки не совпадали бы с заголовком. Новый поток открывается
// через дебаунс: пробежка по списку не должна оставлять за собой по SSH-каналу
// на хост.
func (m *Model) moveFleetLogbox(delta int) tea.Cmd {
	previous := m.selected
	cmd := m.moveFleetBy(delta)
	if m.selected == previous {
		return cmd
	}
	stream := m.scheduleLogsStream()
	m.markLogboxSeen()
	return tea.Batch(cmd, stream)
}

// fleetLogboxLines рисует ящик в height строк списка, но не больше своей
// естественной высоты: рамка и строка состояния занимают три строки, остальное
// достаётся хвосту лога. Хотя бы одна строка лога остаётся всегда — иначе на
// низком терминале от ящика оставалась бы пустая рамка. Ящик врезан в список
// под строкой своего хоста, поэтому width — внутренняя ширина панели списка.
func (m Model) fleetLogboxLines(width, height int) []string {
	if !m.fleet.logbox {
		return nil
	}
	tail := max(1, min(fleetLogboxLines, height-panelOverhead-1))
	visible := m.logs.visibleLines()
	body := []string{spread(dimStyle.Render(m.fleetLogboxStatus()), dimStyle.Render(m.fleetLogboxCount()), width-4)}
	start := max(0, len(visible)-tail)
	for _, line := range visible[start:] {
		body = append(body, fitLine(highlightMatches(line, m.logs.filterInput.Value()), width-4))
	}
	if len(visible) == 0 {
		body = append(body, dimStyle.Render("строк пока нет"))
	}
	// В подсказке ящика только его собственные клавиши: enter/esc/↑↓ ушли в
	// общую строку статусбара внизу экрана.
	// «w warn+», а не «w уровень»: клавиша включает и выключает порог warn+,
	// перебор всех уровней живёт на «W» и в справке.
	return panelBoxStyled("ЛОГИ · "+m.selectedName(), "/ фильтр · w warn+ · s источник", width, body, dimStyle)
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
// Разряды группирует та же функция, что и счётчик полноэкранных логов: «8412»
// и «8 412» на соседних экранах читались бы как разные числа.
func (m Model) fleetLogboxCount() string {
	total := m.logs.buffer.Total()
	hint := groupDigits(len(m.logs.visibleLines())) + " из " + groupDigits(total)
	if fresh := total - m.fleet.logboxSeen; fresh > 0 {
		hint = groupDigits(fresh) + " новых · " + hint
	}
	return hint
}
