package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Left     key.Binding
	Right    key.Binding
	Up       key.Binding
	Down     key.Binding
	Create   key.Binding
	FetchPR  key.Binding
	Delete   key.Binding
	Pull     key.Binding
	Repair   key.Binding
	Editor   key.Binding
	Mouse    key.Binding
	Enter    key.Binding
	Quit     key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "prev"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "next"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Create: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "create"),
		),
		FetchPR: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "fetch PR"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Pull: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pull"),
		),
		Repair: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "repair"),
		),
		Editor: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "editor"),
		),
		Mouse: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mouse"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("↵", "open shell"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
