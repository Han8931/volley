// Command volley is a Vim-centric TUI API client and load tester.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/tui"
)

func main() {
	// Alternate-scroll mode (DECSET ?1007) makes many terminals translate the
	// mouse wheel into arrow keys on the alternate screen; the TUI disables it
	// during Init so the wheel only scrolls the response instead of also nudging
	// the focused pane. Restore it here on exit to hand the shell back its normal
	// wheel behavior.
	_, err := tea.NewProgram(tui.Program(), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()

	fmt.Print("\x1b[?1007h") // restore alternate scroll for the shell
	if err != nil {
		fmt.Fprintln(os.Stderr, "volley:", err)
		os.Exit(1)
	}
}
