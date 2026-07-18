package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/llm"
)

type Model struct {
	collector *collect.Collector
	llm       *llm.Client
	config    *config.Config

	screen   screenKind
	overlay  overlayKind
	selected int
	snapshot collect.Snapshot
	layout   layoutState
	request  uint64
	fleet    fleetModel

	events      <-chan collect.Event
	unsubscribe func()
}

func New(collector *collect.Collector, client *llm.Client, cfg *config.Config) Model {
	m := Model{collector: collector, llm: client, config: cfg, screen: screenFleet, fleet: newFleetModel()}
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
		return m, nil
	case collectorEventMsg:
		m.snapshot = msg.event.Snapshot
		m.clampSelection()
		return m, waitEvent(m.events)
	case ageTickMsg:
		return m, ageTick()
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
		return m.renderDeepPlaceholder("Процессы")
	case screenPorts:
		return m.renderDeepPlaceholder("Порты")
	case screenHistory:
		return m.renderDeepPlaceholder("История")
	case screenLogs:
		return m.renderDeepPlaceholder("Логи")
	case screenContainers:
		return m.renderDeepPlaceholder("Контейнеры")
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
