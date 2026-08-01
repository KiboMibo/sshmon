package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/sshx"
)

type logStreamer interface {
	StreamLogs(context.Context, collect.LogRequest) (sshx.Stream, error)
}

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
	stream      sshx.Stream
	viewport    viewport.Model
	ready       bool
	lastLineAt  time.Time
}

type logsOpenedMsg struct {
	generation uint64
	stream     sshx.Stream
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
	}
}

func (l *logsScreen) ensure() {
	if l.buffer != nil && len(l.sources) > 0 {
		return
	}
	initialized := newLogsScreen()
	if l.buffer == nil {
		l.buffer = initialized.buffer
	}
	if len(l.sources) == 0 {
		l.sources = initialized.sources
	}
	if l.filterInput.Placeholder == "" {
		l.filterInput = initialized.filterInput
	}
}

func (m *Model) startLogsStream() tea.Cmd {
	m.logs.ensure()
	m.cancelLogsStream()
	// Новый поток — новый хост или источник: строки прежнего под новым заголовком
	// выглядят как логи выбранного сервера, хотя пришли с другого.
	m.logs.buffer.Reset()
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
		m.logs.stream = sshx.Stream{}
	}
}

func waitLogEvent(generation uint64, stream sshx.Stream) tea.Cmd {
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
		m.logs.level = (m.logs.level + 1) % logLevel(len(logLevelNames))
		m.logs.refresh()
		return nil, true
	case "s", "x", "right":
		return m.cycleLogSource(1), true
	case "left":
		return m.cycleLogSource(-1), true
	case "r":
		return m.startLogsStream(), true
	case "up", "down", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.logs.viewport, cmd = m.logs.viewport.Update(key)
		return cmd, true
	}
	return nil, false
}

func (m *Model) cycleLogSource(delta int) tea.Cmd {
	count := len(m.logs.sources)
	if count == 0 {
		return nil
	}
	m.logs.source = ((m.logs.source+delta)%count + count) % count
	return m.startLogsStream()
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

func (l *logsScreen) resize(width, height int) {
	l.ensure()
	bodyHeight := max(1, height-6)
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
	lines := l.visibleLines()
	rendered := make([]string, len(lines))
	for i, line := range lines {
		rendered[i] = highlightMatches(line, l.filterInput.Value())
	}
	l.viewport.SetContent(strings.Join(rendered, "\n"))
	if !l.paused {
		l.viewport.GotoBottom()
	}
}

func (m Model) logsState() string {
	if m.logs.err != nil {
		return "ошибка: " + m.logs.err.Error()
	}
	if m.logs.paused {
		return "хвост на паузе"
	}
	return "хвост включён"
}

func logSourceLabel(source collect.LogSource) string {
	label := string(source.Kind)
	if source.Name != "" {
		label += "/" + source.Name
	}
	return label
}

func (m Model) logsSourceAxis(width int) string {
	labels := make([]string, 0, len(m.logs.sources))
	for i, source := range m.logs.sources {
		label := logSourceLabel(source)
		if i == m.logs.source {
			labels = append(labels, titleStyle.Render("["+label+"]"))
			continue
		}
		labels = append(labels, dimStyle.Render(label))
	}
	left := dimStyle.Render("источник  ") + strings.Join(labels, " ")
	right := dimStyle.Render(fmt.Sprintf("%d/%d · ← →", m.logs.source+1, max(1, len(m.logs.sources))))
	return spread(left, right, width)
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
	return spread(left, dimStyle.Render("w уровень"), width)
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
	return fmt.Sprintf("%d из %d строк", len(m.logs.visibleLines()), m.logs.buffer.Total())
}

func (m Model) renderLogs() string {
	m.logs.ensure()
	width := m.layout.width
	lines := []string{
		spread(titleStyle.Render("ЛОГИ · "+m.selectedName()), dimStyle.Render(m.logsState()), width),
		m.logsSourceAxis(width),
		m.logsLevelAxis(width),
		m.logsFilterAxis(width),
		m.logs.viewport.View(),
		dimStyle.Render("/ фильтр · w уровень · space пауза хвоста · s ← → источник"),
		dimStyle.Render("r обновить · ctrl+r переподключить · esc назад"),
	}
	return strings.Join(lines, "\n")
}
