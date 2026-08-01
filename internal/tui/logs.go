package tui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

type logStreamer interface {
	StreamLogs(context.Context, collect.LogRequest) (collect.LogStream, error)
}

// logSelectionStyle — фон выделенной строки. Маркер «▍» слева рисуется всегда:
// на монохромном терминале фон не виден, а выделение по макету обязано читаться.
var logSelectionStyle = lipgloss.NewStyle().Background(lipgloss.Color("236"))

// logsContextRadius — сколько строк показывать вокруг выделенной по клавише «c».
const logsContextRadius = 5

type logsScreen struct {
	buffer      *collect.LogBuffer
	sources     []collect.LogSource
	source      int
	level       logLevel
	paused      bool
	filtering   bool
	filterInput textinput.Model
	status      diagnosticsStatus
	err         error
	generation  uint64
	cancel      context.CancelFunc
	stream      collect.LogStream
	viewport    viewport.Model
	ready       bool
	lastLineAt  time.Time
	// cursor — индекс выделенной строки в теле экрана; -1 значит «выделения нет»,
	// и тогда экран следует за хвостом потока.
	cursor int
	// context — снимок окрестности выделенной строки без учёта фильтра;
	// nil означает, что режим контекста выключен.
	contextLines []string
	// hideTime — колонка времени скрыта клавишей «t».
	hideTime bool
	// notice — короткое подтверждение действия («строка скопирована»), живёт
	// до следующего нажатия клавиши.
	notice string
}

type logsOpenedMsg struct {
	generation uint64
	stream     collect.LogStream
	err        error
}

type logLineMsg struct {
	generation uint64
	line       string
}

type logErrorMsg struct {
	generation uint64
	err        error
}

func newLogsScreen() logsScreen {
	input := textinput.New()
	input.Placeholder = "фильтр логов"
	return logsScreen{
		buffer:      collect.NewLogBuffer(10_000),
		sources:     []collect.LogSource{{Kind: collect.LogSystem}},
		filterInput: input,
		cursor:      -1,
	}
}

func (l *logsScreen) ensure() {
	if l.buffer != nil && len(l.sources) > 0 {
		return
	}
	initialized := newLogsScreen()
	if l.buffer == nil {
		l.buffer = initialized.buffer
		// Зеро-значение экрана: выделения ещё не было, иначе нулевой cursor
		// подсветил бы первую строку сам собой.
		l.cursor = initialized.cursor
	}
	if len(l.sources) == 0 {
		l.sources = initialized.sources
	}
	if l.filterInput.Placeholder == "" {
		l.filterInput = initialized.filterInput
	}
}

// syncLogSources пересобирает список источников под выбранный сервер: системный
// журнал первым (он есть всегда и не зависит от systemd и docker), затем
// systemd-юниты и docker-контейнеры, уже собранные экраном сервера. Активный
// источник ищем в новом списке по значению — иначе каждая пересборка сбрасывала
// бы выбор на системный журнал.
func (m *Model) syncLogSources() {
	m.logs.ensure()
	current := m.logs.sources[0]
	if m.logs.source >= 0 && m.logs.source < len(m.logs.sources) {
		current = m.logs.sources[m.logs.source]
	}
	sources := []collect.LogSource{{Kind: collect.LogSystem}}
	// Юниты и контейнеры берём только от текущего хоста: после экрана сервера A
	// и возврата к списку они остаются в памяти, и на логах сервера B в оси
	// источников оказались бы чужие имена.
	if m.dashboard.server == m.selectedName() {
		for _, unit := range m.dashboard.units.items {
			if unit.Name == "" {
				continue
			}
			sources = append(sources, collect.LogSource{Kind: collect.LogJournal, Name: unit.Name})
		}
		for _, container := range m.dashboard.containers.items {
			if container.Name == "" {
				continue
			}
			sources = append(sources, collect.LogSource{Kind: collect.LogContainer, Name: container.Name})
		}
	}
	m.logs.sources = sources
	m.logs.source = max(0, slices.Index(sources, current))
}

func (m *Model) startLogsStream() tea.Cmd {
	m.logs.ensure()
	m.syncLogSources()
	m.cancelLogsStream()
	// Новый поток — новый хост или источник: строки прежнего под новым заголовком
	// выглядят как логи выбранного сервера, хотя пришли с другого.
	m.logs.buffer.Reset()
	m.logs.cursor = -1
	m.logs.contextLines = nil
	m.logs.refresh()
	m.request = max(m.request, m.logs.generation) + 1
	m.logs.generation = m.request
	m.logs.status = diagnosticsLoading
	m.logs.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.logs.cancel = cancel
	generation := m.logs.generation
	server := m.selectedName()
	source := m.logs.sources[m.logs.source]
	streamer := m.logSource
	return func() tea.Msg {
		if streamer == nil {
			return logsOpenedMsg{generation: generation, err: errors.New("поток логов недоступен")}
		}
		stream, err := streamer.StreamLogs(ctx, collect.NewLogRequest(server, source))
		return logsOpenedMsg{generation: generation, stream: stream, err: err}
	}
}

func (m *Model) cancelLogsStream() {
	if m.logs.cancel != nil {
		m.logs.cancel()
		m.logs.cancel = nil
	}
	if m.logs.stream.Close != nil {
		_ = m.logs.stream.Close()
		m.logs.stream = collect.LogStream{}
	}
}

func waitLogEvent(generation uint64, stream collect.LogStream) tea.Cmd {
	return func() tea.Msg {
		lines, errs := stream.Lines, stream.Errors
		for lines != nil || errs != nil {
			select {
			case line, ok := <-lines:
				if !ok {
					lines = nil
					continue
				}
				return logLineMsg{generation: generation, line: line}
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				return logErrorMsg{generation: generation, err: err}
			}
		}
		return logErrorMsg{generation: generation, err: io.EOF}
	}
}

func (m *Model) handleLogsKey(key tea.KeyMsg) (tea.Cmd, bool) {
	m.logs.ensure()
	value := key.String()
	if m.logs.filtering {
		switch value {
		case "esc":
			m.logs.filtering = false
			m.logs.filterInput.Blur()
			return nil, true
		case "enter":
			m.logs.filtering = false
			m.logs.filterInput.Blur()
			m.logs.buffer.SetFilter(m.logs.filterInput.Value())
			m.logs.refresh()
			return nil, true
		default:
			var cmd tea.Cmd
			m.logs.filterInput, cmd = m.logs.filterInput.Update(key)
			m.logs.buffer.SetFilter(m.logs.filterInput.Value())
			m.logs.refresh()
			return cmd, true
		}
	}
	if value != "y" {
		m.logs.notice = ""
	}
	switch value {
	case " ":
		m.logs.paused = !m.logs.paused
		m.logs.buffer.SetPaused(m.logs.paused)
		m.logs.refresh()
		return nil, true
	case "/":
		m.logs.filtering = true
		m.logs.filterInput.Focus()
		return textinput.Blink, true
	case "w":
		// По макету «w» — переключатель «только warn+», а не перебор уровней:
		// warn как нижняя граница включает и error (visibleLines сравнивает >=).
		if m.logs.level == logLevelWarn {
			m.logs.level = logLevelAll
		} else {
			m.logs.level = logLevelWarn
		}
		m.logs.refresh()
		return nil, true
	case "W":
		// Перебор всех четырёх уровней остался на shift+W: в футере макета его
		// нет, поэтому клавиша живёт только в справке.
		m.logs.level = (m.logs.level + 1) % logLevel(len(logLevelNames))
		m.logs.refresh()
		return nil, true
	case "s", "right":
		return m.cycleLogSource(1), true
	case "left":
		return m.cycleLogSource(-1), true
	case "r":
		return m.startLogsStream(), true
	case "n":
		m.logs.jumpMatch(1)
		return nil, true
	case "N":
		m.logs.jumpMatch(-1)
		return nil, true
	case "t":
		m.logs.hideTime = !m.logs.hideTime
		m.logs.refresh()
		return nil, true
	case "c":
		m.logs.toggleContext()
		return nil, true
	case "y":
		line, ok := m.logs.selectedLine()
		if !ok {
			m.logs.notice = "нет выделенной строки"
			return nil, true
		}
		m.logs.notice = "строка скопирована"
		return copyToClipboard(line), true
	case "esc":
		if m.logs.contextLines != nil {
			m.logs.toggleContext()
			return nil, true
		}
		return nil, false
	case "up", "k":
		m.logs.moveCursor(-1)
		return nil, true
	case "down", "j":
		m.logs.moveCursor(1)
		return nil, true
	case "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.logs.viewport, cmd = m.logs.viewport.Update(key)
		return cmd, true
	}
	return nil, false
}

func (m *Model) cycleLogSource(delta int) tea.Cmd {
	// Юниты и контейнеры могли догрузиться уже после открытия экрана: без
	// пересборки здесь список так и остался бы из одного системного журнала.
	m.syncLogSources()
	count := len(m.logs.sources)
	if count == 0 {
		return nil
	}
	m.logs.source = ((m.logs.source+delta)%count + count) % count
	return m.startLogsStream()
}

// osc52Copy — последовательность OSC 52: копирование делает сам терминал, поэтому
// оно работает и через ssh, и не требует зависимостей. Терминалы без поддержки
// просто игнорируют последовательность — ничего не ломается.
func osc52Copy(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
}

func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		// Пишем прямо в stdout: у bubbletea нет команды «отправить произвольную
		// escape-последовательность», а кадры рендерер печатает туда же.
		_, _ = os.Stdout.WriteString(osc52Copy(text))
		return nil
	}
}

type logLevel int

const (
	logLevelAll logLevel = iota
	logLevelInfo
	logLevelWarn
	logLevelError
)

var logLevelNames = [...]string{"all", "info", "warn", "error"}

func logLineLevel(line string) logLevel {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error"), strings.Contains(lower, "fatal"), strings.Contains(lower, "crit"):
		return logLevelError
	case strings.Contains(lower, "warn"):
		return logLevelWarn
	case strings.Contains(lower, "debug"), strings.Contains(lower, "trace"):
		return logLevelAll
	}
	return logLevelInfo
}

func (l logsScreen) visibleLines() []string {
	lines := l.buffer.Visible()
	if l.level == logLevelAll {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if logLineLevel(line) >= l.level {
			out = append(out, line)
		}
	}
	return out
}

// bodyLines — то, что показано в теле экрана: снимок контекста, если он включён,
// иначе отфильтрованные строки.
func (l logsScreen) bodyLines() []string {
	if l.contextLines != nil {
		return l.contextLines
	}
	return l.visibleLines()
}

// allLines — строки буфера без фильтра. Фильтр живёт внутри LogBuffer, поэтому
// снимаем его на время выборки и сразу возвращаем: экран однопоточный.
func (l *logsScreen) allLines() []string {
	filter := l.filterInput.Value()
	l.buffer.SetFilter("")
	lines := l.buffer.Visible()
	l.buffer.SetFilter(filter)
	return lines
}

func (l logsScreen) selectedLine() (string, bool) {
	lines := l.bodyLines()
	if l.cursor < 0 || l.cursor >= len(lines) {
		return "", false
	}
	return lines[l.cursor], true
}

func (l *logsScreen) moveCursor(delta int) {
	lines := l.bodyLines()
	if len(lines) == 0 {
		l.cursor = -1
		return
	}
	if l.cursor < 0 {
		// Первое нажатие выделяет хвост: пользователь смотрит на последние строки.
		l.cursor = len(lines) - 1
	} else {
		l.cursor = max(0, min(len(lines)-1, l.cursor+delta))
	}
	l.refresh()
}

// jumpMatch двигает выделение к следующему (delta=+1) или предыдущему (-1)
// совпадению фильтра. Фильтр в обычном режиме уже отсеял строки, поэтому там
// n/N идут по соседям; смысл появляется в режиме контекста, где фильтра нет.
func (l *logsScreen) jumpMatch(delta int) {
	lines := l.bodyLines()
	needle := strings.ToLower(l.filterInput.Value())
	start := l.cursor
	if start < 0 {
		if delta > 0 {
			start = -1
		} else {
			start = len(lines)
		}
	}
	for index := start + delta; index >= 0 && index < len(lines); index += delta {
		if needle == "" || strings.Contains(strings.ToLower(lines[index]), needle) {
			l.cursor = index
			l.refresh()
			return
		}
	}
	l.notice = "совпадений больше нет"
}

// toggleContext собирает снимок окрестности выделенной строки без учёта фильтра:
// смысл режима — увидеть как раз то, что фильтр прячет. Снимок статичный, хвост
// в него не дописывается, поэтому выход по «c»/«esc» возвращает живой поток.
func (l *logsScreen) toggleContext() {
	if l.contextLines != nil {
		l.contextLines = nil
		l.cursor = -1
		l.refresh()
		return
	}
	if _, ok := l.selectedLine(); !ok {
		l.notice = "нет выделенной строки"
		return
	}
	all := l.allLines()
	anchor := l.anchorIndex(all)
	if anchor < 0 {
		l.notice = "строка уже вытеснена из буфера"
		return
	}
	start := max(0, anchor-logsContextRadius)
	end := min(len(all), anchor+logsContextRadius+1)
	l.contextLines = append([]string(nil), all[start:end]...)
	l.cursor = anchor - start
	l.refresh()
}

// anchorIndex переводит позицию курсора в отфильтрованном теле в индекс той же
// строки в нефильтрованном буфере: идём по буферу тем же предикатом, что и
// visibleLines, и считаем совпадения. Поиск по тексту строки не годится —
// одинаковые строки в логе встречаются постоянно.
func (l logsScreen) anchorIndex(all []string) int {
	needle := strings.ToLower(l.filterInput.Value())
	seen := 0
	for index, line := range all {
		if needle != "" && !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		if l.level != logLevelAll && logLineLevel(line) < l.level {
			continue
		}
		if seen == l.cursor {
			return index
		}
		seen++
	}
	return -1
}

func highlightMatches(line, filter string) string {
	if filter == "" {
		return line
	}
	lower := strings.ToLower(line)
	needle := strings.ToLower(filter)
	// ponytail: ToLower может изменить длину строки (например, ß→ss), тогда
	// индексы из lower не совпадут с line — в этом случае не подсвечиваем.
	if len(lower) != len(line) {
		return line
	}
	var b strings.Builder
	for {
		idx := strings.Index(lower, needle)
		if idx < 0 {
			b.WriteString(line)
			return b.String()
		}
		b.WriteString(line[:idx])
		b.WriteString(warnStyle.Render(line[idx : idx+len(needle)]))
		line = line[idx+len(needle):]
		lower = lower[idx+len(needle):]
	}
}

// logsChromeLines — сколько строк экрана занимает всё, кроме списка: заголовок,
// три оси, подсказка выделения, полоса плотности и две строки футера.
const logsChromeLines = 8

func (l *logsScreen) resize(width, height int) {
	l.ensure()
	bodyHeight := max(1, height-logsChromeLines)
	if !l.ready {
		l.viewport = viewport.New(max(1, width), bodyHeight)
		l.ready = true
	} else {
		l.viewport.Width = max(1, width)
		l.viewport.Height = bodyHeight
	}
	l.filterInput.Width = max(1, width-4)
	l.refresh()
}

func (l *logsScreen) refresh() {
	if !l.ready {
		return
	}
	lines := l.bodyLines()
	if l.cursor >= len(lines) {
		l.cursor = len(lines) - 1
	}
	filter := l.filterInput.Value()
	rendered := make([]string, len(lines))
	for i, line := range lines {
		text := l.displayLine(line)
		if i == l.cursor {
			// Подсветку совпадений на выделенной строке не накладываем: её
			// внутренний «сброс цвета» оборвал бы фон на середине строки.
			rendered[i] = logSelectionStyle.Render(padCell("▍"+text, max(1, l.viewport.Width)))
			continue
		}
		rendered[i] = " " + highlightMatches(text, filter)
	}
	l.viewport.SetContent(strings.Join(rendered, "\n"))
	switch {
	case l.cursor >= 0:
		l.scrollToCursor()
	case !l.paused:
		l.viewport.GotoBottom()
	}
}

func (l *logsScreen) scrollToCursor() {
	height := max(1, l.viewport.Height)
	offset := l.viewport.YOffset
	if l.cursor < offset {
		offset = l.cursor
	}
	if l.cursor >= offset+height {
		offset = l.cursor - height + 1
	}
	l.viewport.SetYOffset(offset)
}

// displayLine убирает колонку времени, когда она выключена клавишей «t»: на
// узком терминале сообщение важнее отметки, а формат префикса у journalctl и
// docker разный, поэтому режем по найденной метке HH:MM:SS.
func (l logsScreen) displayLine(line string) string {
	if !l.hideTime {
		return line
	}
	if end, _, ok := logTimeAt(line); ok {
		return strings.TrimLeft(line[end:], " ")
	}
	return line
}

// logTimePrefixLimit — докуда ищем отметку времени. Дальше начала строки её не
// бывает, а искать по всей строке нельзя: «12:34:56» встречается и в сообщении.
const logTimePrefixLimit = 40

// logTimeAt ищет в начале строки время суток HH:MM:SS. Префиксы разные
// (journalctl «Aug 01 19:41:02 host …», docker — ISO-8601), но время суток в
// обоих стоит в первых полях: этого хватает и полосе плотности, и клавише «t».
// Возвращает индекс за меткой и время в секундах от полуночи.
func logTimeAt(line string) (int, int, bool) {
	limit := min(len(line), logTimePrefixLimit)
	for i := 0; i+8 <= limit; i++ {
		if line[i+2] != ':' || line[i+5] != ':' {
			continue
		}
		if i > 0 && line[i-1] >= '0' && line[i-1] <= '9' {
			continue // хвост более длинного числа, а не часы
		}
		hours, okH := twoDigits(line[i:])
		minutes, okM := twoDigits(line[i+3:])
		seconds, okS := twoDigits(line[i+6:])
		if !okH || !okM || !okS || hours > 23 || minutes > 59 || seconds > 59 {
			continue
		}
		return i + 8, hours*3600 + minutes*60 + seconds, true
	}
	return 0, 0, false
}

func twoDigits(value string) (int, bool) {
	if len(value) < 2 || value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' {
		return 0, false
	}
	return int(value[0]-'0')*10 + int(value[1]-'0'), true
}

// logDensity — распределение строк по времени для полосы плотности.
type logDensity struct {
	counts     []*float64
	spike      string
	spikeLevel logLevel
	span       string
}

// newLogDensity раскладывает строки по buckets вёдрам между первой и последней
// отметкой времени. Строки без времени просто не попадают в полосу: пустая
// полоса честнее выдуманного распределения.
func newLogDensity(lines []string, buckets int) logDensity {
	if buckets < 1 {
		return logDensity{}
	}
	type entry struct {
		at    int
		level logLevel
	}
	entries := make([]entry, 0, len(lines))
	first, last := 0, 0
	for _, line := range lines {
		_, at, ok := logTimeAt(line)
		if !ok {
			continue
		}
		if len(entries) == 0 || at < first {
			first = at
		}
		if len(entries) == 0 || at > last {
			last = at
		}
		entries = append(entries, entry{at: at, level: logLineLevel(line)})
	}
	if len(entries) == 0 {
		return logDensity{}
	}
	span := last - first
	counts := make([]float64, buckets)
	alerts := make([]int, buckets)
	worst := make([]logLevel, buckets)
	for _, item := range entries {
		bucket := 0
		if span > 0 {
			bucket = (item.at - first) * (buckets - 1) / span
		}
		counts[bucket]++
		if item.level >= logLevelWarn {
			alerts[bucket]++
			if item.level > worst[bucket] {
				worst[bucket] = item.level
			}
		}
	}
	density := logDensity{counts: make([]*float64, buckets), span: "-" + shortSpan(span) + " — сейчас"}
	for i := range counts {
		density.counts[i] = &counts[i]
	}
	peak := -1
	for i, count := range alerts {
		if count > 0 && (peak < 0 || count > alerts[peak]) {
			peak = i
		}
	}
	if peak >= 0 {
		at := first
		if buckets > 1 {
			at = first + peak*span/(buckets-1)
		}
		density.spikeLevel = worst[peak]
		density.spike = fmt.Sprintf("⚠ %02d:%02d всплеск %s", at/3600, at/60%60, logLevelNames[worst[peak]])
	}
	return density
}

func shortSpan(seconds int) string {
	switch {
	case seconds >= 3600:
		return fmt.Sprintf("%dч", seconds/3600)
	case seconds >= 60:
		return fmt.Sprintf("%dм", seconds/60)
	default:
		return fmt.Sprintf("%dс", seconds)
	}
}

func (m Model) logsState() string {
	return logsStateText(m.logs.err, m.logs.paused)
}

// logsStateText — общая формулировка состояния хвоста для полноэкранных логов,
// ящика логов флота и плитки логов на экране сервера: один и тот же поток не
// должен называться на трёх экранах по-разному. Формулировки короткие (макет
// 3d: «postgres · warn+ · хвост вкл»), потому что в ящик логов строка состояния
// помещается вместе с именем источника, уровнем и счётчиком.
func logsStateText(err error, paused bool) string {
	switch {
	case err != nil:
		return "ошибка: " + err.Error()
	case paused:
		return "хвост на паузе"
	default:
		return "хвост вкл"
	}
}

func logSourceLabel(source collect.LogSource) string {
	label := string(source.Kind)
	if source.Name != "" {
		label += "/" + source.Name
	}
	return label
}

// logsAxisLabel — подпись источника в оси полноэкранных логов: системный журнал
// «systemd», юнит — коротким именем, контейнер — «docker/имя» (как в макете).
// Отдельно от logSourceLabel: ту подпись показывает ящик логов на списке хостов.
func logsAxisLabel(source collect.LogSource) string {
	switch source.Kind {
	case collect.LogContainer:
		return "docker/" + source.Name
	case collect.LogJournal:
		return strings.TrimSuffix(source.Name, ".service")
	default:
		return "systemd"
	}
}

func (m Model) logsSourceAxis(width int) string {
	sources := m.logs.sources
	prefix := "источник  "
	right := dimStyle.Render(fmt.Sprintf("%d/%d · ← →", min(m.logs.source+1, max(1, len(sources))), max(1, len(sources))))
	if len(sources) == 0 {
		return spread(dimStyle.Render(prefix), right, width)
	}
	active := max(0, min(len(sources)-1, m.logs.source))
	labels := make([]string, len(sources))
	for i, source := range sources {
		labels[i] = logsAxisLabel(source)
	}
	// Источников бывает десятки (юниты плюс контейнеры), в строку они не влезут.
	// Показываем окно вокруг активного: обрезка с конца прятала бы как раз его.
	budget := max(1, width-lipgloss.Width(prefix)-lipgloss.Width(right)-2)
	low, high := active, active
	used := lipgloss.Width(labels[active]) + 2 // скобки вокруг активного
	for {
		grew := false
		if high+1 < len(labels) && used+1+lipgloss.Width(labels[high+1]) <= budget {
			high++
			used += 1 + lipgloss.Width(labels[high])
			grew = true
		}
		if low > 0 && used+1+lipgloss.Width(labels[low-1]) <= budget {
			low--
			used += 1 + lipgloss.Width(labels[low])
			grew = true
		}
		if !grew {
			break
		}
	}
	parts := make([]string, 0, high-low+1)
	for i := low; i <= high; i++ {
		if i == active {
			parts = append(parts, titleStyle.Render("["+labels[i]+"]"))
			continue
		}
		parts = append(parts, dimStyle.Render(labels[i]))
	}
	return spread(dimStyle.Render(prefix)+strings.Join(parts, " "), right, width)
}

func (m Model) logsLevelAxis(width int) string {
	labels := make([]string, 0, len(logLevelNames))
	for i, name := range logLevelNames {
		if logLevel(i) == m.logs.level {
			labels = append(labels, titleStyle.Render("["+name+"]"))
			continue
		}
		labels = append(labels, dimStyle.Render(name))
	}
	left := dimStyle.Render("уровень   ") + strings.Join(labels, " ")
	return spread(left, dimStyle.Render("w только warn+"), width)
}

func (m Model) logsFilterAxis(width int) string {
	left := dimStyle.Render("фильтр    ")
	switch {
	case m.logs.filtering:
		left += m.logs.filterInput.View()
	case m.logs.filterInput.Value() != "":
		left += "> " + m.logs.filterInput.Value()
	default:
		left += dimStyle.Render("> все строки")
	}
	return spread(left, dimStyle.Render(m.logsCountHint()), width)
}

func (m Model) logsCountHint() string {
	return fmt.Sprintf("%s из %s строк", groupDigits(len(m.logs.visibleLines())), groupDigits(m.logs.buffer.Total()))
}

// groupDigits разбивает число на группы по три: в макете счётчик «214 из 8 412».
func groupDigits(value int) string {
	digits := fmt.Sprint(value)
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(digit)
	}
	return b.String()
}

func (m Model) logsDensityLine(width int) string {
	cells := max(8, min(30, width/3))
	density := newLogDensity(m.logs.bodyLines(), cells)
	left := dimStyle.Render("плотность ") + historySparkline(density.counts, cells)
	if density.spike != "" {
		style := warnStyle
		if density.spikeLevel == logLevelError {
			style = criticalStyle
		}
		left += "   " + style.Render(density.spike)
	}
	return spread(left, dimStyle.Render(density.span), width)
}

// logsSelectionHint — подсказка под списком строк. В режиме контекста она
// говорит, чем этот режим отличается и как из него выйти.
func (m Model) logsSelectionHint() string {
	if m.logs.contextLines != nil {
		return fmt.Sprintf("контекст ±%d строк без фильтра · c или esc назад", logsContextRadius)
	}
	return "↑ выделенная строка · y скопировать · c контекст ±5"
}

// logsFooter собирает футер макета. На узком терминале клавиши не заменяются
// другими: список тот же, просто хвост не влезших отбрасывается — кроме «esc»,
// без которого с экрана не выйти.
func logsFooter(width int) []string {
	const separator = " · "
	const closeHint = "esc закрыть"
	items := []string{
		"/ фильтр", "n/N след/пред", "w только warn+", "space пауза хвоста",
		"t время", "c контекст ±5", "y копировать",
	}
	rows := []string{""}
	for _, item := range items {
		row := rows[len(rows)-1]
		switch {
		case row == "":
			rows[len(rows)-1] = item
		case lipgloss.Width(row+separator+item) <= width:
			rows[len(rows)-1] = row + separator + item
		case len(rows) < 2:
			rows = append(rows, item)
		}
	}
	last := len(rows) - 1
	for lipgloss.Width(rows[last]+separator+closeHint) > width && strings.Contains(rows[last], separator) {
		rows[last] = rows[last][:strings.LastIndex(rows[last], separator)]
	}
	if rows[last] == "" {
		rows[last] = closeHint
	} else {
		rows[last] += separator + closeHint
	}
	for i, row := range rows {
		rows[i] = dimStyle.Render(fitLine(row, width))
	}
	return rows
}

func (m Model) renderLogs() string {
	m.logs.ensure()
	width := m.layout.width
	state := m.logsState()
	if m.logs.notice != "" {
		state = m.logs.notice
	}
	lines := []string{
		spread(titleStyle.Render("ЛОГИ · "+m.selectedName()), dimStyle.Render(state), width),
		m.logsSourceAxis(width),
		m.logsLevelAxis(width),
		m.logsFilterAxis(width),
		m.logs.viewport.View(),
		spread("", dimStyle.Render(m.logsSelectionHint()), width),
		m.logsDensityLine(width),
	}
	return strings.Join(append(lines, logsFooter(width)...), "\n")
}
