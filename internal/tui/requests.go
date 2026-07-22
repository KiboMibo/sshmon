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

type diagnostics struct {
	status     diagnosticsStatus
	err        error
	generation uint64
	cancel     func()
}

func (d *diagnostics) finish(err error, hasData bool) {
	d.err = err
	d.status = diagnosticsResultStatus(err, hasData)
}

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

func diagnosticsCadence(kind screenKind) time.Duration {
	switch kind {
	case screenProcesses:
		return 2 * time.Second
	case screenPorts, screenContainers:
		return 5 * time.Second
	default:
		return 0
	}
}

func (m *Model) diagnosticsFor(kind screenKind) *diagnostics {
	switch kind {
	case screenProcesses:
		return &m.processes.diagnostics
	case screenPorts:
		return &m.ports.diagnostics
	case screenContainers:
		return &m.containers.diagnostics
	default:
		return nil
	}
}

func (m Model) diagnosticsGeneration(kind screenKind) uint64 {
	if d := m.diagnosticsFor(kind); d != nil {
		return d.generation
	}
	return 0
}

func (m *Model) startDiagnostics() tea.Cmd {
	m.cancelDiagnostics()
	m.request++
	generation := m.request
	d := m.diagnosticsFor(m.screen)
	if d == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.generation, d.cancel, d.status = generation, cancel, diagnosticsLoading
	return runDiagnostics(ctx, generation, m.screen, m.selectedName(), m.collector)
}

func (m *Model) cancelDiagnostics() {
	d := m.diagnosticsFor(m.screen)
	if d == nil || d.cancel == nil {
		return
	}
	d.cancel()
	d.cancel = nil
}

func runDiagnostics(ctx context.Context, generation uint64, kind screenKind, server string, collector *collect.Collector) tea.Cmd {
	return func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return diagnosticsResult(kind, generation, err)
		}
		if collector == nil {
			return diagnosticsResult(kind, generation, errors.New("коллектор недоступен"))
		}
		switch kind {
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
			return diagnosticsResult(kind, generation, collect.ErrUnsupported)
		}
	}
}

func diagnosticsResult(kind screenKind, generation uint64, err error) tea.Msg {
	switch kind {
	case screenProcesses:
		return processesResultMsg{generation: generation, err: err}
	case screenPorts:
		return portsResultMsg{generation: generation, err: err}
	default:
		return containersResultMsg{generation: generation, err: err}
	}
}

func scheduleDiagnostics(kind screenKind, generation uint64) tea.Cmd {
	delay := diagnosticsCadence(kind)
	if delay == 0 {
		return nil
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return diagnosticsTickMsg{screen: kind, generation: generation}
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
