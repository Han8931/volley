// Package keys defines Volley's editor modes. The root model holds the
// current Mode and routes every key event through it — this is the heart
// of Volley's Vim-style modal editing.
package keys

// Mode is the current editor mode, à la Vim.
type Mode int

const (
	// Normal is for navigation and commands (h/j/k/l, gt, :, etc.).
	Normal Mode = iota
	// Insert is for typing into the focused text field.
	Insert
	// Command is the ":" command-line (added in a later phase).
	Command
)

// String returns the short uppercase tag shown in the status bar.
func (m Mode) String() string {
	switch m {
	case Insert:
		return "INSERT"
	case Command:
		return "COMMAND"
	default:
		return "NORMAL"
	}
}
