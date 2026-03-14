package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).MarginBottom(1)
	categoryStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	enabledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	disabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	stagedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dimStyle      = lipgloss.NewStyle().Faint(true)
	helpStyle     = lipgloss.NewStyle().Faint(true).MarginTop(1)
	searchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)
