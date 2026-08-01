package tui

import (
	"context"
	"os/exec"
	"slices"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/config"
)

func (m *Model) startFleetCardUnits() tea.Cmd {
	if m.dashboard.units.cancel != nil {
		m.dashboard.units.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	generation := max(m.request, m.dashboard.units.generation) + 1
	m.request = generation
	configured := []string(nil)
	if m.config != nil {
		configured = slices.Clone(m.config.Dashboard.SystemdUnits)
	}
	m.dashboard.units = dashboardUnitsState{status: diagnosticsLoading, generation: generation, cancel: cancel}
	return m.loadDashboardUnits(ctx, generation, m.selectedName(), configured)
}

func (m *Model) moveFleetBy(delta int) tea.Cmd {
	m.ensureFleet()
	previous := m.selectedName()
	m.moveFleet(delta)
	if m.fleet.expanded {
		return m.startFleetCardUnits()
	}
	// Запрос строго по факту смены хоста: удержанная стрелка иначе слала бы по
	// SSH-команде на каждое движение курсора, а упёршийся в край список — на
	// каждое нажатие.
	if m.selectedName() == previous {
		return nil
	}
	return m.startFleetTopProcesses()
}

func (m Model) openFromFleet(kind screenKind) (tea.Model, tea.Cmd) {
	if len(m.snapshot.Servers) == 0 {
		return m, nil
	}
	workspace := m.startDashboardWorkspace()
	m.screen = kind
	switch kind {
	case screenProcesses, screenPorts, screenContainers:
		return m, tea.Batch(workspace, m.startDiagnostics())
	case screenLogs:
		return m, tea.Batch(workspace, m.startLogsStream())
	default:
		return m, workspace
	}
}

func (m Model) startSSHSession() tea.Cmd {
	server, ok := m.selectedConfigServer()
	if !ok {
		return nil
	}
	return tea.ExecProcess(exec.Command("ssh", sshArgs(server)...), func(error) tea.Msg { return nil })
}

func (m Model) selectedConfigServer() (config.Server, bool) {
	name := m.selectedName()
	for _, server := range m.configServers() {
		if server.Name == name {
			return server, true
		}
	}
	return config.Server{}, false
}

func sshArgs(server config.Server) []string {
	args := make([]string, 0, 5)
	if server.Port != 0 && server.Port != 22 {
		args = append(args, "-p", strconv.Itoa(server.Port))
	}
	if server.Key != "" {
		args = append(args, "-i", server.Key)
	}
	target := server.Host
	if server.User != "" {
		target = server.User + "@" + server.Host
	}
	return append(args, target)
}
