package tui

import "github.com/charmbracelet/bubbles/key"

// GlobalKeys are available from any view.
type GlobalKeys struct {
	GoalsBoard  key.Binding
	SprintStats key.Binding
	Monthly     key.Binding
	Back        key.Binding
	Help        key.Binding
}

// BoardKeys handle navigation and ticket actions on the dashboard and goals board.
type BoardKeys struct {
	// Directional (arrow + vim)
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// Ticket actions
	Select key.Binding
	Create key.Binding
	Delete key.Binding
	Open   key.Binding
}

// StatsKeys handle navigation within the sprint stats overlay.
type StatsKeys struct {
	PrevSprint key.Binding
	NextSprint key.Binding
	Close      key.Binding
}

// FormKeys handle the ticket creation form.
type FormKeys struct {
	Next     key.Binding
	Prev     key.Binding
	Confirm  key.Binding
	Cancel   key.Binding
	Toggle   key.Binding // toggle boolean fields (e.g. repeatable)
}

// Global is the singleton key map shared across all views.
var Global = GlobalKeys{
	GoalsBoard: key.NewBinding(
		key.WithKeys("g", "G"),
		key.WithHelp("g", "goals board"),
	),
	SprintStats: key.NewBinding(
		key.WithKeys("y", "Y"),
		key.WithHelp("y", "sprint stats"),
	),
	Monthly: key.NewBinding(
		key.WithKeys("m", "M"),
		key.WithHelp("m", "monthly analysis"),
	),
	Back: key.NewBinding(
		key.WithKeys("q", "esc"),
		key.WithHelp("q", "back / quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
}

// Board is the singleton key map for kanban boards.
var Board = BoardKeys{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "move left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "move right"),
	),
	Select: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "select / move"),
	),
	Create: key.NewBinding(
		key.WithKeys("+"),
		key.WithHelp("+", "create ticket"),
	),
	Delete: key.NewBinding(
		key.WithKeys("-"),
		key.WithHelp("-", "delete ticket"),
	),
	Open: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open ticket"),
	),
}

// Stats is the key map for the sprint stats overlay.
var Stats = StatsKeys{
	PrevSprint: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "previous sprint"),
	),
	NextSprint: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "next sprint"),
	),
	Close: key.NewBinding(
		key.WithKeys("y", "Y", "esc", "q"),
		key.WithHelp("y/esc", "close"),
	),
}

// Form is the key map for ticket creation forms.
var Form = FormKeys{
	Next: key.NewBinding(
		key.WithKeys("tab", "down"),
		key.WithHelp("tab", "next field"),
	),
	Prev: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
	),
}
