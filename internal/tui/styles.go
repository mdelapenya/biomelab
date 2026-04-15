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

	mainCardHelpActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	mainCardHelpFaintStyle  = lipgloss.NewStyle().Faint(true)

	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	sandboxStoppedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // yellow/orange — matches card sandbox stopped color

	inputPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	refreshTimestampStyle = lipgloss.NewStyle().Faint(true)

	selectedRepoStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")) // cyan

	unselectedRepoStyle = lipgloss.NewStyle().Faint(true)

	repoPanelHelpStyle = lipgloss.NewStyle().Faint(true)

	scrollTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	scrollThumbStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	popupStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("214")).
			BorderBackground(lipgloss.Color("234")).
			Padding(1, 2).
			Foreground(lipgloss.Color("214")).
			Background(lipgloss.Color("234"))
)
