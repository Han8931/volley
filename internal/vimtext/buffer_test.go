package vimtext

import "testing"

// drive feeds a sequence of keys (one per slice element) into a buffer that
// starts in Normal mode with the given text and cursor column on row 0.
func drive(text string, col int, keys ...string) *Buffer {
	b := New(text, false)
	b.SetMode(Normal)
	b.row, b.col = 0, col
	for _, k := range keys {
		b.Feed(k)
	}
	return b
}

func TestInsertAndEscape(t *testing.T) {
	b := New("", false) // starts in Insert
	for _, k := range []string{"h", "i"} {
		b.Feed(k)
	}
	if b.Text() != "hi" {
		t.Fatalf("text = %q", b.Text())
	}
	b.Feed("esc")
	if b.Mode() != Normal {
		t.Error("esc should switch to Normal")
	}
	if _, c := b.Cursor(); c != 1 { // moved one left off the end
		t.Errorf("col after esc = %d, want 1", c)
	}
}

func TestEscReleasesFromNormal(t *testing.T) {
	b := drive("abc", 0)
	if rel := b.Feed("esc"); !rel {
		t.Error("esc in Normal mode should signal release")
	}
}

func TestX(t *testing.T) {
	b := drive("hello", 0, "x")
	if b.Text() != "ello" {
		t.Errorf("x: %q", b.Text())
	}
	b = drive("hello", 0, "3", "x")
	if b.Text() != "lo" {
		t.Errorf("3x: %q", b.Text())
	}
}

func TestDeleteWord(t *testing.T) {
	b := drive("foo bar baz", 0, "d", "w")
	if b.Text() != "bar baz" {
		t.Errorf("dw: %q", b.Text())
	}
	b = drive("foo bar baz", 0, "2", "d", "w")
	if b.Text() != "baz" {
		t.Errorf("2dw: %q", b.Text())
	}
}

func TestChangeWord(t *testing.T) {
	b := drive("foo bar", 0, "c", "w")
	if b.Mode() != Insert {
		t.Error("cw should enter Insert")
	}
	if b.Text() != " bar" { // cw == ce: removes 'foo', keeps the space
		t.Errorf("cw text: %q", b.Text())
	}
	for _, k := range []string{"X", "Y", "Z"} {
		b.Feed(k)
	}
	if b.Text() != "XYZ bar" {
		t.Errorf("after typing: %q", b.Text())
	}
}

func TestDD(t *testing.T) {
	b := New("one\ntwo\nthree", false)
	b.SetMode(Normal)
	b.row, b.col = 1, 0
	b.Feed("d")
	b.Feed("d")
	if b.Text() != "one\nthree" {
		t.Errorf("dd: %q", b.Text())
	}
}

func TestDCDollarAndCapitalC(t *testing.T) {
	b := drive("hello world", 6, "D")
	if b.Text() != "hello " {
		t.Errorf("D: %q", b.Text())
	}
	b = drive("hello world", 6, "C")
	if b.Mode() != Insert || b.Text() != "hello " {
		t.Errorf("C: mode=%v text=%q", b.Mode(), b.Text())
	}
	b.Feed("y")
	if b.Text() != "hello y" {
		t.Errorf("C then type: %q", b.Text())
	}
}

func TestWordMotions(t *testing.T) {
	b := drive("foo bar baz", 0, "w")
	if _, c := b.Cursor(); c != 4 {
		t.Errorf("w -> col %d, want 4", c)
	}
	b.Feed("e")
	if _, c := b.Cursor(); c != 6 {
		t.Errorf("e -> col %d, want 6", c)
	}
	b.Feed("b")
	if _, c := b.Cursor(); c != 4 {
		t.Errorf("b -> col %d, want 4", c)
	}
	b.Feed("$")
	if _, c := b.Cursor(); c != 10 {
		t.Errorf("$ -> col %d, want 10", c)
	}
	b.Feed("0")
	if _, c := b.Cursor(); c != 0 {
		t.Errorf("0 -> col %d, want 0", c)
	}
}

func TestUndoRedo(t *testing.T) {
	b := drive("hello", 0, "x", "x") // -> "llo"
	if b.Text() != "llo" {
		t.Fatalf("setup: %q", b.Text())
	}
	b.Feed("u")
	if b.Text() != "ello" {
		t.Errorf("after u: %q", b.Text())
	}
	b.Feed("u")
	if b.Text() != "hello" {
		t.Errorf("after uu: %q", b.Text())
	}
	b.Feed("ctrl+r")
	if b.Text() != "ello" {
		t.Errorf("after redo: %q", b.Text())
	}
}

func TestInsertIsOneUndo(t *testing.T) {
	b := drive("", 0, "i")
	for _, k := range []string{"a", "b", "c"} {
		b.Feed(k)
	}
	b.Feed("esc")
	if b.Text() != "abc" {
		t.Fatalf("typed: %q", b.Text())
	}
	b.Feed("u")
	if b.Text() != "" {
		t.Errorf("undo should remove the whole insert, got %q", b.Text())
	}
}

func TestYankPaste(t *testing.T) {
	b := drive("hello", 0, "y", "w") // yank "hello" (charwise, no trailing word)
	b.Feed("$")
	b.Feed("p") // paste after last char
	if b.Text() != "hellohello" {
		t.Errorf("yw then p: %q", b.Text())
	}
}

func TestLinewisePaste(t *testing.T) {
	b := New("one\ntwo", false)
	b.SetMode(Normal)
	b.row, b.col = 0, 0
	b.Feed("y")
	b.Feed("y") // yank line "one"
	b.Feed("p")
	if b.Text() != "one\none\ntwo" {
		t.Errorf("yy p: %q", b.Text())
	}
}

func TestReplaceChar(t *testing.T) {
	b := drive("cat", 0, "r", "b")
	if b.Text() != "bat" {
		t.Errorf("r: %q", b.Text())
	}
	if b.Mode() != Normal {
		t.Error("r should stay in Normal")
	}
}

func TestOpenLine(t *testing.T) {
	b := drive("top", 0, "o")
	if b.Mode() != Insert {
		t.Error("o enters Insert")
	}
	for _, k := range []string{"n", "e", "w"} {
		b.Feed(k)
	}
	if b.Text() != "top\nnew" {
		t.Errorf("o: %q", b.Text())
	}
	b.Feed("esc")
	b.Feed("O")
	for _, k := range []string{"u", "p"} {
		b.Feed(k)
	}
	if b.Text() != "top\nup\nnew" {
		t.Errorf("O: %q", b.Text())
	}
}

func TestSingleLineNoNewline(t *testing.T) {
	b := New("abc", true)
	b.Feed("enter") // ignored in single-line insert mode
	if b.Text() != "abc" {
		t.Errorf("single-line enter should be inert, got %q", b.Text())
	}
}
