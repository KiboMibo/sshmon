package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	overlayStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
)
