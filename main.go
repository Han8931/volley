// Command volley is a Vim-centric TUI API client and load tester.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/tui"
)

func main() {
	p := tea.NewProgram(tui.Program(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "volley:", err)
		os.Exit(1)
	}
}
