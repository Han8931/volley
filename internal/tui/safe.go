package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// safeModel wraps a tea.Model so that a panic in any single message handler is
// logged and swallowed rather than tearing down the whole session. Bubble Tea
// already restores the terminal on an uncaught panic; this goes one better by
// keeping the app running — one bad keypress drops that action instead of
// ending the session — and by writing a durable stack trace for diagnosis.
type safeModel struct{ m tea.Model }

// Program returns the root model wrapped with panic protection. main uses this
// instead of New(); tests keep using New() directly for the bare Model.
func Program() tea.Model { return safeModel{m: New()} }

// Init implements tea.Model.
func (s safeModel) Init() tea.Cmd { return s.m.Init() }

// Update delegates to the inner model, recovering from any panic. Because the
// inner Update runs on a value copy, a panic leaves s.m at its pre-update
// state, giving us a clean rollback: the offending action is simply ignored.
func (s safeModel) Update(msg tea.Msg) (out tea.Model, cmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			logCrash("update", r)
			inner := s.m
			// Surface a hint in the status bar when the inner model supports it.
			if mm, ok := inner.(Model); ok {
				mm.statusMsg = "internal error (ignored; logged to " + crashLogPath() + ")"
				inner = mm
			}
			out, cmd = safeModel{m: inner}, nil
		}
	}()

	next, c := s.m.Update(msg)
	return safeModel{m: next}, c
}

// disableAltScroll is DECRST ?1007 — it turns off "alternate scroll mode", in
// which many terminals (iTerm2, etc.) translate the mouse wheel into arrow
// keys while the alternate screen is active. Left on, one wheel notch would
// both scroll the response (the mouse event we handle) AND move the focused
// pane via the injected arrows — visible as "the other panes move," especially
// once the response is scrolled to its end and can't absorb the wheel.
//
// It must be emitted from within a rendered frame: Bubble Tea enters the
// alternate screen before the first frame, so writing it any earlier (e.g. in
// main before Run) lands on the normal buffer and never takes effect here.
const disableAltScroll = "\x1b[?1007l"

// View delegates to the inner model, falling back to a readable message rather
// than a corrupt frame if rendering panics. It prefixes the frame with the
// alternate-scroll reset so the wheel arrives only as mouse events.
func (s safeModel) View() (v string) {
	defer func() {
		if r := recover(); r != nil {
			logCrash("view", r)
			v = "volley: internal render error (logged to " + crashLogPath() + ")\n" +
				"press a key to continue, or q to quit"
		}
	}()
	return disableAltScroll + s.m.View()
}

// crashLogPath is where panics are appended. It sits alongside the collections
// directory so it is easy to find when reporting a bug.
func crashLogPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "volley", "crash.log")
}

// logCrash appends a timestamped panic value and stack trace to the crash log.
// It is best-effort: any I/O error is intentionally ignored, since we are
// already handling a panic and must not introduce a second one.
func logCrash(where string, r interface{}) {
	path := crashLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	// Best-effort crash log: if this write fails there is nowhere sensible left
	// to report it, so the error is deliberately dropped.
	_, _ = fmt.Fprintf(f, "\n=== panic in %s at %s ===\n%v\n\n%s\n",
		where, time.Now().Format(time.RFC3339), r, debug.Stack())
}
