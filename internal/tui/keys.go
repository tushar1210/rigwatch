package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit     key.Binding
	Next     key.Binding
	Prev     key.Binding
	Proc     key.Binding
	Help     key.Binding
	SortCPU  key.Binding
	SortMem  key.Binding
	SortGPU  key.Binding
	SortVRAM key.Binding
	Kill     key.Binding
	Back     key.Binding
}

func newKeys() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Next: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next host"),
		),
		Prev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev host"),
		),
		Proc: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "processes"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		SortCPU: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "sort cpu"),
		),
		SortMem: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "sort mem"),
		),
		SortGPU: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "sort gpu"),
		),
		SortVRAM: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "sort vram"),
		),
		Kill: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "kill"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.Next, k.Proc, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Quit, k.Back, k.Next, k.Prev},
		{k.Proc, k.SortCPU, k.SortMem, k.SortGPU, k.SortVRAM, k.Kill, k.Help},
	}
}

// procKeyMap is the short hint while the process list is open.
type procKeyMap struct{ keyMap }

func (k procKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.SortCPU, k.SortMem, k.SortGPU, k.SortVRAM, k.Kill, k.Back}
}

func (k procKeyMap) FullHelp() [][]key.Binding {
	return k.keyMap.FullHelp()
}
