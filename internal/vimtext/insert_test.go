package vimtext

import "testing"

// insertAt returns a buffer in Insert mode with the cursor at col on row 0.
func insertAt(text string, col int) *Buffer {
	b := New(text, false)
	b.SetMode(Insert)
	b.row, b.col = 0, col
	b.clampInsert()
	return b
}

func TestInsertBackspaceMidLine(t *testing.T) {
	b := insertAt("abc", 2)
	b.Feed("backspace")
	if b.Text() != "ac" {
		t.Errorf("backspace: %q, want %q", b.Text(), "ac")
	}
	if _, c := b.Cursor(); c != 1 {
		t.Errorf("col after backspace = %d, want 1", c)
	}
}

func TestInsertBackspaceJoinsLines(t *testing.T) {
	b := New("ab\ncd", false)
	b.SetMode(Insert)
	b.row, b.col = 1, 0
	b.Feed("backspace")
	if b.Text() != "abcd" {
		t.Errorf("join: %q, want %q", b.Text(), "abcd")
	}
	if r, c := b.Cursor(); r != 0 || c != 2 {
		t.Errorf("cursor after join = %d,%d, want 0,2", r, c)
	}
}

func TestInsertEnterSplitsLine(t *testing.T) {
	b := insertAt("abcd", 2)
	b.Feed("enter")
	if b.Text() != "ab\ncd" {
		t.Errorf("enter split: %q, want %q", b.Text(), "ab\ncd")
	}
	if r, c := b.Cursor(); r != 1 || c != 0 {
		t.Errorf("cursor after split = %d,%d, want 1,0", r, c)
	}
}

func TestInsertArrowsHomeEnd(t *testing.T) {
	b := insertAt("hello", 5)
	b.Feed("left")
	if _, c := b.Cursor(); c != 4 {
		t.Errorf("left: col %d, want 4", c)
	}
	b.Feed("home")
	if _, c := b.Cursor(); c != 0 {
		t.Errorf("home: col %d, want 0", c)
	}
	b.Feed("right")
	if _, c := b.Cursor(); c != 1 {
		t.Errorf("right: col %d, want 1", c)
	}
	b.Feed("end")
	if _, c := b.Cursor(); c != 5 {
		t.Errorf("end: col %d, want 5", c)
	}
	// left never underflows, right never overruns the line length in Insert.
	b.Feed("home")
	b.Feed("left")
	if _, c := b.Cursor(); c != 0 {
		t.Errorf("left at col 0: %d, want 0", c)
	}
	b.Feed("end")
	b.Feed("right")
	if _, c := b.Cursor(); c != 5 {
		t.Errorf("right at line end: %d, want 5", c)
	}
}

func TestInsertUpDownClampsColumn(t *testing.T) {
	b := New("longline\nhi", false) // Insert, cursor at end of row 0 (col 8)
	b.Feed("down")
	if r, c := b.Cursor(); r != 1 || c != 2 {
		t.Errorf("down onto short line = %d,%d, want 1,2 (clamped)", r, c)
	}
	b.Feed("up")
	if r := func() int { r, _ := b.Cursor(); return r }(); r != 0 {
		t.Errorf("up back to row %d, want 0", r)
	}
}

func TestInsertCtrlWDeletesWord(t *testing.T) {
	b := insertAt("foo bar", 7)
	b.Feed("ctrl+w")
	if b.Text() != "foo " {
		t.Errorf("ctrl+w: %q, want %q", b.Text(), "foo ")
	}
	if _, c := b.Cursor(); c != 4 {
		t.Errorf("col after ctrl+w = %d, want 4", c)
	}
}

func TestInsertCtrlWAtLineStartJoins(t *testing.T) {
	b := New("ab\ncd", false)
	b.SetMode(Insert)
	b.row, b.col = 1, 0
	b.Feed("ctrl+w") // col 0 falls back to backspace, joining lines
	if b.Text() != "abcd" {
		t.Errorf("ctrl+w at col 0: %q, want %q", b.Text(), "abcd")
	}
}

func TestInsertTabAndSpace(t *testing.T) {
	b := insertAt("", 0)
	b.Feed("tab")
	b.Feed("space")
	if b.Text() != "   " { // two-space soft tab + one space
		t.Errorf("tab+space: %q, want three spaces", b.Text())
	}
}

func TestInsertFiltersNamedAndChordKeys(t *testing.T) {
	b := insertAt("", 0)
	for _, k := range []string{"delete", "pgup", "ctrl+x", "shift+enter"} {
		b.Feed(k)
	}
	if b.Text() != "" {
		t.Errorf("named/chord keys must not insert text, got %q", b.Text())
	}
	b.Feed("x")      // single rune inserts
	b.Feed("pasted") // multi-rune non-chord is treated as pasted text
	if b.Text() != "xpasted" {
		t.Errorf("literal keys: %q, want %q", b.Text(), "xpasted")
	}
}

func TestInsertSingleLineEnterDoesNotSplit(t *testing.T) {
	b := New("ab", true) // single-line
	b.Feed("enter")
	if b.Text() != "ab" {
		t.Errorf("single-line enter changed text: %q", b.Text())
	}
	if len(b.Lines()) != 1 {
		t.Errorf("single-line buffer grew to %d rows", len(b.Lines()))
	}
}
