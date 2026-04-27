package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dylanewe/lira/tui"
)

func main() {
	p := tea.NewProgram(
		tui.New(),
		tea.WithAltScreen(),       // full-screen TUI, restores terminal on exit
		tea.WithMouseCellMotion(), // enables mouse support for future use
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lira: %v\n", err)
		os.Exit(1)
	}
}
