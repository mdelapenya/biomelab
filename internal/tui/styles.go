package tui

import "github.com/charmbracelet/lipgloss"

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			Width(40)

	selectedCardStyle = cardStyle.
				BorderForeground(lipgloss.Color("39")) // cyan

	mainCardStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			Padding(1, 2).
			Width(40)

	selectedMainCardStyle = mainCardStyle.
				BorderForeground(lipgloss.Color("39")) // cyan

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("245")).
			MarginTop(1)

	cleanStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // green

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().Faint(true).MarginTop(1)

	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red

	inputPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	confirmStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)
