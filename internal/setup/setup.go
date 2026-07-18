package setup

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/config"
)

var (
	styTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	styDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m model) Init() tea.Cmd { return nil }

// Run показывает пикер и возвращает выбранные серверы; nil, nil при отмене.
func Run(hosts []config.SSHHost) ([]config.Server, error) {
	p := tea.NewProgram(newModel(hosts), tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, err
	}
	finalModel, ok := result.(model)
	if !ok {
		return nil, fmt.Errorf("unexpected setup model %T", result)
	}
	if finalModel.abort {
		return nil, nil
	}
	return config.HostsToServers(finalModel.selectedHosts()), nil
}
