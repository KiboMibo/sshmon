package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	overlayStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	goodStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	criticalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)
