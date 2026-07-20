package tui

import (
	"context"
	"errors"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

const dashboardLogLines = 50

type dashboardSource interface {
	Containers(context.Context, string) ([]collect.Container, error)
	SystemdUnits(context.Context, string, []string) ([]collect.SystemdUnit, error)
	LogSnapshot(context.Context, collect.LogRequest, int) ([]string, error)
}

type dashboardContainersState struct {
	items      []collect.Container
	status     diagnosticsStatus
	err        error
	generation uint64
	cancel     context.CancelFunc
}

type dashboardUnitsState struct {
	items      []collect.SystemdUnit
	status     diagnosticsStatus
	err        error
	generation uint64
	cancel     context.CancelFunc
}

type dashboardLogState struct {
	lines      []string
	source     collect.LogSource
	status     diagnosticsStatus
	err        error
	generation uint64
	cancel     context.CancelFunc
}

type dashboardWorkspace struct {
	containers dashboardContainersState
	units      dashboardUnitsState
	logs       dashboardLogState
	unitUI     dashboardUnitUI
}

type dashboardContainersResultMsg struct {
	generation uint64
	items      []collect.Container
	err        error
}

type dashboardUnitsResultMsg struct {
	generation uint64
	items      []collect.SystemdUnit
	err        error
}

type dashboardLogResultMsg struct {
	generation uint64
	lines      []string
	source     collect.LogSource
	err        error
}

func (m *Model) startDashboardWorkspace() tea.Cmd {
	m.cancelDashboardWorkspace()
	server := m.selectedName()
	configured := []string(nil)
	if m.config != nil {
		configured = slices.Clone(m.config.Dashboard.SystemdUnits)
	}

	containerCtx, containerCancel := context.WithCancel(context.Background())
	m.request++
	containerGeneration := m.request
	m.dashboard.containers = dashboardContainersState{status: diagnosticsLoading, generation: containerGeneration, cancel: containerCancel}

	unitCtx, unitCancel := context.WithCancel(context.Background())
	m.request++
	unitGeneration := m.request
	m.dashboard.units = dashboardUnitsState{status: diagnosticsLoading, generation: unitGeneration, cancel: unitCancel}

	logCtx, logCancel := context.WithCancel(context.Background())
	m.request++
	logGeneration := m.request
	source := collect.LogSource{Kind: collect.LogSystem}
	m.dashboard.logs = dashboardLogState{source: source, status: diagnosticsLoading, generation: logGeneration, cancel: logCancel}

	return tea.Batch(
		m.loadDashboardContainers(containerCtx, containerGeneration, server),
		m.loadDashboardUnits(unitCtx, unitGeneration, server, configured),
		m.loadDashboardLog(logCtx, logGeneration, collect.NewLogRequest(server, source)),
	)
}

func (m *Model) startDashboardLog(source collect.LogSource) tea.Cmd {
	if m.dashboard.logs.cancel != nil {
		m.dashboard.logs.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.request++
	generation := m.request
	m.dashboard.logs = dashboardLogState{source: source, status: diagnosticsLoading, generation: generation, cancel: cancel}
	return m.loadDashboardLog(ctx, generation, collect.NewLogRequest(m.selectedName(), source))
}

func (m *Model) cancelDashboardWorkspace() {
	for _, cancel := range []context.CancelFunc{m.dashboard.containers.cancel, m.dashboard.units.cancel, m.dashboard.logs.cancel} {
		if cancel != nil {
			cancel()
		}
	}
	m.dashboard.containers.cancel = nil
	m.dashboard.units.cancel = nil
	m.dashboard.logs.cancel = nil
}

func (m Model) loadDashboardContainers(ctx context.Context, generation uint64, server string) tea.Cmd {
	return func() tea.Msg {
		if err := dashboardRequestError(ctx, m.dashboardSource); err != nil {
			return dashboardContainersResultMsg{generation: generation, err: err}
		}
		items, err := m.dashboardSource.Containers(ctx, server)
		return dashboardContainersResultMsg{generation: generation, items: items, err: err}
	}
}

func (m Model) loadDashboardUnits(ctx context.Context, generation uint64, server string, configured []string) tea.Cmd {
	return func() tea.Msg {
		if err := dashboardRequestError(ctx, m.dashboardSource); err != nil {
			return dashboardUnitsResultMsg{generation: generation, err: err}
		}
		items, err := m.dashboardSource.SystemdUnits(ctx, server, configured)
		return dashboardUnitsResultMsg{generation: generation, items: items, err: err}
	}
}

func (m Model) loadDashboardLog(ctx context.Context, generation uint64, request collect.LogRequest) tea.Cmd {
	return func() tea.Msg {
		if err := dashboardRequestError(ctx, m.dashboardSource); err != nil {
			return dashboardLogResultMsg{generation: generation, source: request.Source, err: err}
		}
		lines, err := m.dashboardSource.LogSnapshot(ctx, request, dashboardLogLines)
		return dashboardLogResultMsg{generation: generation, lines: lines, source: request.Source, err: err}
	}
}

func dashboardRequestError(ctx context.Context, source dashboardSource) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if source == nil {
		return errors.New("коллектор недоступен")
	}
	return nil
}

func (m *Model) applyDashboardResult(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case dashboardContainersResultMsg:
		if msg.generation == m.dashboard.containers.generation {
			if msg.err == nil {
				m.dashboard.containers.items = slices.Clone(msg.items)
			}
			m.dashboard.containers.err = msg.err
			m.dashboard.containers.status = diagnosticsResultStatus(msg.err, len(m.dashboard.containers.items) > 0)
		}
		return true
	case dashboardUnitsResultMsg:
		if msg.generation == m.dashboard.units.generation {
			if msg.err == nil {
				m.dashboard.units.items = slices.Clone(msg.items)
			}
			m.dashboard.units.err = msg.err
			m.dashboard.units.status = diagnosticsResultStatus(msg.err, len(m.dashboard.units.items) > 0)
		}
		return true
	case dashboardLogResultMsg:
		if msg.generation == m.dashboard.logs.generation {
			if msg.err == nil {
				m.dashboard.logs.lines = slices.Clone(msg.lines)
			}
			m.dashboard.logs.source = msg.source
			m.dashboard.logs.err = msg.err
			m.dashboard.logs.status = diagnosticsResultStatus(msg.err, len(m.dashboard.logs.lines) > 0)
		}
		return true
	default:
		return false
	}
}
