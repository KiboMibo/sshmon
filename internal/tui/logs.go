package tui

import (
	"context"
	"errors"
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
	case "s":
		if len(m.logs.sources) > 0 {
			m.logs.source = (m.logs.source + 1) % len(m.logs.sources)
			return m.startLogsStream(), true
		}
		return nil, true
	case "r":
		return m.startLogsStream(), true
	case "up", "down", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.logs.viewport, cmd = m.logs.viewport.Update(key)
		return cmd, true
	}
	return nil, false
}

func (l *logsScreen) resize(width, height int) {
	l.ensure()
	bodyHeight := max(1, height-4)
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
	l.viewport.SetContent(strings.Join(l.buffer.Visible(), "\n"))
	if !l.paused {
		l.viewport.GotoBottom()
	}
}

func (m Model) renderLogs() string {
	m.logs.ensure()
	source := "system"
	if len(m.logs.sources) > 0 {
		selected := m.logs.sources[m.logs.source]
		source = string(selected.Kind)
		if selected.Name != "" {
			source += ":" + selected.Name
		}
	}
	state := "live"
	if m.logs.paused {
		state = "pause"
	}
	if m.logs.err != nil {
		state = "ошибка: " + m.logs.err.Error()
	}
	view := titleStyle.Render("sshmon · "+m.selectedName()+" · Логи") + "\n"
	view += dimStyle.Render(source+" · "+state) + "\n"
	view += m.logs.viewport.View() + "\n"
	if m.logs.filtering {
		return view + m.logs.filterInput.View()
	}
	return view + dimStyle.Render("space пауза · / фильтр · s источник · r переподключить · esc назад")
}
