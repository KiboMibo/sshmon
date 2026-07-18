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
	styOn    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func (m model) Init() tea.Cmd { return nil }

// View: временный плоский рендер; иерархический рендер с вьюпортом добавляется в Задаче 3 (view.go).
func (m model) View() string {
	s := styTitle.Render("sshmon: выберите серверы для мониторинга") + "\n"
	for vi, row := range m.visible {
		cur := "  "
		if vi == m.cursor {
			cur = "> "
		}
		switch row.kind {
		case rowSource:
			src := m.sources[row.source]
			s += cur + fmt.Sprintf("[%s] %s\n", src.group, src.path)
		case rowHost:
			h := m.sources[row.source].hosts[row.host]
			mark := "[ ]"
			if h.selected {
				mark = styOn.Render("[x]")
			}
			s += cur + "  " + mark + " " + h.host.Alias + "\n"
		}
	}
	s += styDim.Render(fmt.Sprintf("выбрано: %d", m.selectedCount()))
	return s
}

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
