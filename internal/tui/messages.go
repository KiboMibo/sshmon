package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

type collectorEventMsg struct {
	event collect.Event
}

type ageTickMsg struct{}

func waitEvent(events <-chan collect.Event) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return collectorEventMsg{event: event}
	}
}
