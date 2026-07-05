// Command volley is a Vim-centric TUI API client and load tester.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/tui"
)

func main() {
	// In alternate-screen mode many terminals (iTerm2, etc.) translate the mouse
	// wheel into arrow keys — "alternate scroll mode", DECSET ?1007. With mouse
	// reporting enabled, one wheel notch would then do two things at once: scroll
	// the response (the mouse event we handle) AND move the focused pane (the
	// injected arrow keys). Disable it so the wheel arrives only as mouse events,
	// and restore it on exit.
	fmt.Print("\x1b[?1007l")

	_, err := tea.NewProgram(tui.Program(), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()

	fmt.Print("\x1b[?1007h") // restore alternate scroll for the shell
	if err != nil {
		fmt.Fprintln(os.Stderr, "volley:", err)
		os.Exit(1)
	}
}
