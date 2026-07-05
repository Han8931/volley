// Package vimtext is a small, UI-agnostic modal text-editing engine with
// Vim semantics. It operates on a []rune line buffer and is driven entirely
// by key-name strings (the same names Bubble Tea's KeyMsg.String() produces),
// which makes it exhaustively unit-testable without a terminal.
package vimtext

import "strings"

// Mode is the editing sub-mode of a single field.
type Mode int

const (
	// Normal is field-level command mode (motions, operators, x/dd/C…).
	Normal Mode = iota
	// Insert is text-entry mode.
	Insert
)

// Buffer is a modal text buffer for one field.
type Buffer struct {
	lines      [][]rune
	row, col   int
	mode       Mode
	singleLine bool

	// register holds the last delete/yank for p/P.
	register    []string
	regLinewise bool

	// pending multi-key state
	pendingOp rune // 0, 'd', 'c', or 'y'
	opCount   int  // count captured with the operator
	pendingG  bool // saw 'g', waiting for the second
	pendingR  bool // saw 'r', waiting for the replacement char
	count     int  // numeric count prefix being accumulated

	undo []snapshot
	redo []snapshot
}

type snapshot struct {
	lines [][]rune
	row   int
	col   int
}

// New returns a buffer seeded with text. When singleLine is true, newlines
// are not introduced by editing and Text joins without separators.
func New(text string, singleLine bool) *Buffer {
	b := &Buffer{mode: Insert, singleLine: singleLine}
	b.setText(text)
	b.col = len(b.lines[b.row]) // start at end, like appending
	return b
}

// SetText replaces the buffer contents, placing the cursor at the end of the
// (first) line. The mode is left unchanged; the owner manages it. Programmatic
// replacement starts a fresh edit history so undo/redo and pending Vim commands
// from a previous request cannot leak into the new contents.
func (b *Buffer) SetText(text string) {
	b.setText(text)
	b.col = len(b.lines[b.row])
	b.clampForMode()
	b.resetTransient()
	b.undo = nil
	b.redo = nil
}

// CursorEnd moves the cursor to the end of the current line without changing
// text or history. Owners use this when focusing an already-loaded single-line
// field, where SetText would be too destructive.
func (b *Buffer) CursorEnd() {
	b.col = len(b.cur())
	b.clampForMode()
}

// SetCursorCol places the cursor at col on the first line, clamped to the
// current mode's bounds. Intended for single-line fields positioned by a mouse
// click; multi-line callers should use motions instead.
func (b *Buffer) SetCursorCol(col int) {
	b.row = 0
	b.col = col
	b.clampForMode()
}

func (b *Buffer) setText(text string) {
	raw := strings.Split(text, "\n")
	b.lines = make([][]rune, len(raw))
	for i, l := range raw {
		b.lines[i] = []rune(l)
	}
	if len(b.lines) == 0 {
		b.lines = [][]rune{{}}
	}
	b.row, b.col = 0, 0
}

// Text returns the buffer contents.
func (b *Buffer) Text() string {
	parts := make([]string, len(b.lines))
	for i, l := range b.lines {
		parts[i] = string(l)
	}
	if b.singleLine {
		return strings.Join(parts, "")
	}
	return strings.Join(parts, "\n")
}

// Mode reports the current sub-mode.
func (b *Buffer) Mode() Mode { return b.mode }

// Cursor returns the cursor row and column (0-based).
func (b *Buffer) Cursor() (int, int) { return b.row, b.col }

// Lines returns the buffer's lines as strings, for rendering.
func (b *Buffer) Lines() []string {
	out := make([]string, len(b.lines))
	for i, l := range b.lines {
		out[i] = string(l)
	}
	return out
}

// SetMode forces a mode (used by the owner when entering a field).
func (b *Buffer) SetMode(m Mode) {
	if b.mode != m {
		b.resetTransient()
	}
	b.mode = m
	if m == Normal {
		b.clampNormal()
	}
}

// Feed processes one key. It returns release=true when the user pressed Esc
// in Normal mode, signalling the owner to return focus to pane navigation.
func (b *Buffer) Feed(key string) (release bool) {
	if b.mode == Insert {
		b.feedInsert(key)
		return false
	}
	return b.feedNormal(key)
}

// --- helpers ---

func (b *Buffer) cur() []rune { return b.lines[b.row] }

func (b *Buffer) clampForMode() {
	if b.mode == Normal {
		b.clampNormal()
		return
	}
	b.clampInsert()
}

func (b *Buffer) resetTransient() {
	b.pendingOp = 0
	b.opCount = 0
	b.pendingG = false
	b.pendingR = false
	b.count = 0
}

func (b *Buffer) clampNormal() {
	if b.row < 0 {
		b.row = 0
	}
	if b.row > len(b.lines)-1 {
		b.row = len(b.lines) - 1
	}
	max := len(b.cur()) - 1
	if max < 0 {
		max = 0
	}
	if b.col > max {
		b.col = max
	}
	if b.col < 0 {
		b.col = 0
	}
}

func (b *Buffer) clampInsert() {
	if b.col > len(b.cur()) {
		b.col = len(b.cur())
	}
	if b.col < 0 {
		b.col = 0
	}
}

func (b *Buffer) snapshot() snapshot {
	cp := make([][]rune, len(b.lines))
	for i, l := range b.lines {
		cp[i] = append([]rune(nil), l...)
	}
	return snapshot{lines: cp, row: b.row, col: b.col}
}

// pushUndo records the current state for a subsequent mutation.
func (b *Buffer) pushUndo() {
	b.undo = append(b.undo, b.snapshot())
	b.redo = nil
}

func (b *Buffer) restore(s snapshot) {
	b.lines = make([][]rune, len(s.lines))
	for i, l := range s.lines {
		b.lines[i] = append([]rune(nil), l...)
	}
	b.row, b.col = s.row, s.col
}
