package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	historypkg "github.com/kibomibo/sshmon/internal/history"
	"github.com/kibomibo/sshmon/internal/llm"
)

type Model struct {
	collector *collect.Collector
	llm       *llm.Client
	config    *config.Config

	screen     screenKind
	overlay    overlayKind
	selected   int
	snapshot   collect.Snapshot
	layout     layoutState
	request    uint64
	fleet      fleetModel
	processes  processScreen
	ports      portScreen
	containers containerScreen
	history    historyScreen
	historyDB  *historypkg.Service
	logs       logsScreen
	logSource  logStreamer

	events      <-chan collect.Event
	unsubscribe func()
}

func New(collector *collect.Collector, client *llm.Client, cfg *config.Config) Model {
	m := Model{collector: collector, llm: client, config: cfg, screen: screenFleet, fleet: newFleetModel(), logs: newLogsScreen(), logSource: collector}
	if collector != nil {
		m.snapshot = collector.Snapshot()
		m.events, m.unsubscribe = collector.Subscribe(1)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(waitEvent(m.events), ageTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout = newLayout(msg.Width, msg.Height)
		m.logs.resize(msg.Width, msg.Height)
		return m, nil
	case collectorEventMsg:
		previousMinute := m.snapshot.Time.Truncate(time.Minute)
		m.snapshot = msg.event.Snapshot
		m.clampSelection()
		if m.screen == screenHistory && !m.snapshot.Time.Truncate(time.Minute).Equal(previousMinute) {
			return m, tea.Batch(waitEvent(m.events), m.startHistoryQuery())
		}
		return m, waitEvent(m.events)
	case ageTickMsg:
		return m, ageTick()
	case processesResultMsg:
		if msg.generation == m.processes.generation {
			m.processes.apply(msg.items, msg.err)
			return m, scheduleDiagnostics(screenProcesses, msg.generation)
		}
		return m, nil
	case portsResultMsg:
		if msg.generation == m.ports.generation {
			m.ports.apply(msg.items, msg.err)
			return m, scheduleDiagnostics(screenPorts, msg.generation)
		}
		return m, nil
	case containersResultMsg:
		if msg.generation == m.containers.generation {
			m.containers.apply(msg.items, msg.err)
			return m, scheduleDiagnostics(screenContainers, msg.generation)
		}
		return m, nil
	case diagnosticsTickMsg:
		if msg.screen == m.screen && msg.generation == m.diagnosticsGeneration(msg.screen) {
			return m, m.startDiagnostics()
		}
		return m, nil
	case historyResultMsg:
		if msg.generation == m.history.generation {
			m.history.apply(msg.points, msg.err)
		}
		return m, nil
	case logsOpenedMsg:
		if msg.generation != m.logs.generation {
			if msg.stream.Close != nil {
				_ = msg.stream.Close()
			}
			return m, nil
		}
		if msg.err != nil {
			m.logs.status = diagnosticsError
			m.logs.err = msg.err
			return m, nil
		}
		m.logs.stream = msg.stream
		m.logs.status = diagnosticsReady
		return m, waitLogEvent(msg.generation, msg.stream)
	case logLineMsg:
		if msg.generation != m.logs.generation {
			return m, nil
		}
		m.logs.buffer.Append(msg.line)
		m.logs.lastLineAt = time.Now()
		m.logs.status = diagnosticsReady
		m.logs.refresh()
		return m, waitLogEvent(msg.generation, m.logs.stream)
	case logErrorMsg:
		if msg.generation == m.logs.generation {
			m.logs.err = msg.err
			if len(m.logs.buffer.Visible()) > 0 {
				m.logs.status = diagnosticsStale
			} else {
				m.logs.status = diagnosticsError
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) View() string {
	if m.layout.tooSmall {
		return "sshmon\n\nувеличьте терминал минимум до 60×16"
	}
	body := m.renderScreen()
	if m.overlay != overlayNone {
		body += "\n\n" + overlayStyle.Render(overlayTitle(m.overlay)+" · esc закрыть")
	}
	return body
}

func (m Model) renderScreen() string {
	switch m.screen {
	case screenFleet:
		return m.renderFleet()
	case screenDashboard:
		return m.renderDashboard()
	case screenProcesses:
		return m.renderProcesses()
	case screenPorts:
		return m.renderPorts()
	case screenHistory:
		return m.renderHistory()
	case screenLogs:
		return m.renderLogs()
	case screenContainers:
		return m.renderContainers()
	default:
		return "sshmon"
	}
}

func (m Model) renderDeepPlaceholder(title string) string {
	return titleStyle.Render("sshmon · "+m.selectedName()+" · "+title) + "\n\n" +
		dimStyle.Render("данные загружаются · esc назад")
}

func (m *Model) clampSelection() {
	if len(m.snapshot.Servers) == 0 {
		m.selected = 0
		return
	}
	if m.selected >= len(m.snapshot.Servers) {
		m.selected = len(m.snapshot.Servers) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m Model) selectedName() string {
	if m.selected >= 0 && m.selected < len(m.snapshot.Servers) {
		return m.snapshot.Servers[m.selected].Name
	}
	return "сервер не выбран"
}

func (m *Model) closeSubscription() {
	if m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

var _ tea.Model = Model{}

func ageTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return ageTickMsg{} })
}
