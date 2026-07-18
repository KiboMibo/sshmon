package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

type diagnosticsStatus uint8

const (
	diagnosticsIdle diagnosticsStatus = iota
	diagnosticsLoading
	diagnosticsReady
	diagnosticsStale
	diagnosticsUnsupported
	diagnosticsError
)

type processesResultMsg struct {
	generation uint64
	items      []collect.Process
	err        error
}

type portsResultMsg struct {
	generation uint64
	items      []collect.Port
	err        error
}

type containersResultMsg struct {
	generation uint64
	items      []collect.Container
	err        error
}

type diagnosticsTickMsg struct {
	screen     screenKind
	generation uint64
}

func diagnosticsCadence(screen screenKind) time.Duration {
	switch screen {
	case screenProcesses:
		return 2 * time.Second
	case screenPorts, screenContainers:
		return 5 * time.Second
	default:
		return 0
	}
}

func (m *Model) startDiagnostics() tea.Cmd {
	m.cancelDiagnostics()
	m.request++
	generation := m.request
	ctx, cancel := context.WithCancel(context.Background())
	server := m.selectedName()
	switch m.screen {
	case screenProcesses:
		m.processes.generation, m.processes.cancel, m.processes.status = generation, cancel, diagnosticsLoading
	case screenPorts:
		m.ports.generation, m.ports.cancel, m.ports.status = generation, cancel, diagnosticsLoading
	case screenContainers:
		m.containers.generation, m.containers.cancel, m.containers.status = generation, cancel, diagnosticsLoading
	default:
		cancel()
		return nil
	}
	return runDiagnostics(ctx, generation, m.screen, server, m.collector)
}

func (m *Model) cancelDiagnostics() {
	switch m.screen {
	case screenProcesses:
		if m.processes.cancel != nil {
			m.processes.cancel()
			m.processes.cancel = nil
		}
	case screenPorts:
		if m.ports.cancel != nil {
			m.ports.cancel()
			m.ports.cancel = nil
		}
	case screenContainers:
		if m.containers.cancel != nil {
			m.containers.cancel()
			m.containers.cancel = nil
		}
	}
}

func runDiagnostics(ctx context.Context, generation uint64, screen screenKind, server string, collector *collect.Collector) tea.Cmd {
	return func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return diagnosticsResult(screen, generation, err)
		}
		if collector == nil {
			return diagnosticsResult(screen, generation, errors.New("коллектор недоступен"))
		}
		switch screen {
		case screenProcesses:
			items, err := collector.Processes(ctx, server)
			return processesResultMsg{generation: generation, items: items, err: err}
		case screenPorts:
			items, err := collector.Ports(ctx, server)
			return portsResultMsg{generation: generation, items: items, err: err}
		case screenContainers:
			items, err := collector.Containers(ctx, server)
			return containersResultMsg{generation: generation, items: items, err: err}
		default:
			return diagnosticsResult(screen, generation, collect.ErrUnsupported)
		}
	}
}

func diagnosticsResult(screen screenKind, generation uint64, err error) tea.Msg {
	switch screen {
	case screenProcesses:
		return processesResultMsg{generation: generation, err: err}
	case screenPorts:
		return portsResultMsg{generation: generation, err: err}
	default:
		return containersResultMsg{generation: generation, err: err}
	}
}

func scheduleDiagnostics(screen screenKind, generation uint64) tea.Cmd {
	delay := diagnosticsCadence(screen)
	if delay == 0 {
		return nil
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return diagnosticsTickMsg{screen: screen, generation: generation}
	})
}

func diagnosticsResultStatus(err error, hasData bool) diagnosticsStatus {
	if err == nil {
		return diagnosticsReady
	}
	if errors.Is(err, collect.ErrUnsupported) {
		return diagnosticsUnsupported
	}
	if hasData {
		return diagnosticsStale
	}
	return diagnosticsError
}

func diagnosticsFooter(status diagnosticsStatus, err error) string {
	switch status {
	case diagnosticsLoading:
		return "загрузка… · esc назад"
	case diagnosticsUnsupported:
		return "не поддерживается на сервере · esc назад"
	case diagnosticsStale:
		return "устаревшие данные: " + err.Error() + " · esc назад"
	case diagnosticsError:
		return "ошибка: " + err.Error() + " · esc назад"
	default:
		return "только чтение · esc назад"
	}
}
