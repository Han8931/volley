package vimtext

import "testing"

func TestDeleteBeforeCursor(t *testing.T) {
	b := drive("hello", 3, "X")
	if b.Text() != "helo" {
		t.Errorf("X: %q, want %q", b.Text(), "helo")
	}
	if _, c := b.Cursor(); c != 2 {
		t.Errorf("col after X = %d, want 2", c)
	}
	// X at column 0 is a no-op on the text.
	b = drive("hello", 0, "X")
	if b.Text() != "hello" {
		t.Errorf("X at col 0 changed text: %q", b.Text())
	}
}

func TestSubstituteChar(t *testing.T) {
	b := drive("hello", 0, "s") // delete 'h', enter Insert
	if b.Mode() != Insert {
		t.Fatalf("s should enter Insert mode")
	}
	b.Feed("H")
	b.Feed("esc")
	if b.Text() != "Hello" {
		t.Errorf("s then H: %q, want %q", b.Text(), "Hello")
	}
}

func TestSubstituteLine(t *testing.T) {
	b := drive("hello", 2, "S")
	if b.Mode() != Insert {
		t.Fatalf("S should enter Insert mode")
	}
	b.Feed("h")
	b.Feed("i")
	b.Feed("esc")
	if b.Text() != "hi" {
		t.Errorf("S then hi: %q, want %q", b.Text(), "hi")
	}
}

func TestToggleCase(t *testing.T) {
	b := drive("aBc", 0, "3", "~")
	if b.Text() != "AbC" {
		t.Errorf("3~: %q, want %q", b.Text(), "AbC")
	}
}

func TestAppendEntersInsertAfterCursor(t *testing.T) {
	b := drive("hi", 0, "a")
	b.Feed("X")
	b.Feed("esc")
	if b.Text() != "hXi" {
		t.Errorf("a then X: %q, want %q", b.Text(), "hXi")
	}
}

func TestInsertAtFirstNonBlankAndLineEnd(t *testing.T) {
	b := drive("  hi", 0, "I")
	b.Feed("X")
	b.Feed("esc")
	if b.Text() != "  Xhi" {
		t.Errorf("I then X: %q, want %q", b.Text(), "  Xhi")
	}
	b = drive("hi", 0, "A")
	b.Feed("X")
	b.Feed("esc")
	if b.Text() != "hiX" {
		t.Errorf("A then X: %q, want %q", b.Text(), "hiX")
	}
}

func TestPasteBeforeCharwise(t *testing.T) {
	b := drive("abc", 0, "y", "l") // yank "a" charwise
	b.Feed("$")
	b.Feed("P") // paste before the cursor
	if b.Text() != "abac" {
		t.Errorf("yl $ P: %q, want %q", b.Text(), "abac")
	}
}

func TestReplaceCharIgnoresNamedKey(t *testing.T) {
	b := drive("cat", 0, "r", "z")
	if b.Text() != "zat" {
		t.Errorf("rz: %q, want %q", b.Text(), "zat")
	}
	// A named/multi-rune replacement target is ignored, leaving text intact.
	b = drive("cat", 0, "r", "enter")
	if b.Text() != "cat" {
		t.Errorf("r<enter> should be a no-op, got %q", b.Text())
	}
}

func TestCountPrefixWithTrailingZero(t *testing.T) {
	b := drive("abcdefghijk", 0, "1", "0", "x") // 10x
	if b.Text() != "k" {
		t.Errorf("10x: %q, want %q", b.Text(), "k")
	}
}

func TestOperatorBackwardMotion(t *testing.T) {
	b := drive("hello world", 6, "d", "b") // delete back to start of "hello "
	if b.Text() != "world" {
		t.Errorf("db: %q, want %q", b.Text(), "world")
	}
}

func TestDeleteToLineEndOperator(t *testing.T) {
	b := drive("hello", 2, "d", "$")
	if b.Text() != "he" {
		t.Errorf("d$: %q, want %q", b.Text(), "he")
	}
}

func TestDeleteLinesWithMotion(t *testing.T) {
	b := New("l1\nl2\nl3", false)
	b.SetMode(Normal)
	b.Feed("d")
	b.Feed("j") // linewise over rows 0..1
	if b.Text() != "l3" {
		t.Errorf("dj: %q, want %q", b.Text(), "l3")
	}
}

func TestChangeLineDoubledOperator(t *testing.T) {
	b := New("foo\nbar", false)
	b.SetMode(Normal)
	b.Feed("c")
	b.Feed("c") // cc: change the current line
	if b.Mode() != Insert {
		t.Fatalf("cc should enter Insert mode")
	}
	b.Feed("X")
	b.Feed("esc")
	if b.Text() != "X\nbar" {
		t.Errorf("cc then X: %q, want %q", b.Text(), "X\nbar")
	}
}

func TestYankLineThenPaste(t *testing.T) {
	b := New("a\nb", false)
	b.SetMode(Normal)
	b.Feed("y")
	b.Feed("y") // yank line "a" linewise
	b.Feed("p") // paste below
	if b.Text() != "a\na\nb" {
		t.Errorf("yy p: %q, want %q", b.Text(), "a\na\nb")
	}
}

func TestSetCursorColClamps(t *testing.T) {
	b := New("hello", true)
	b.SetMode(Normal)
	b.SetCursorCol(3)
	if _, c := b.Cursor(); c != 3 {
		t.Errorf("SetCursorCol(3): %d", c)
	}
	b.SetCursorCol(99)
	if _, c := b.Cursor(); c != 4 { // clamped to len-1 in Normal
		t.Errorf("SetCursorCol(99): %d, want 4", c)
	}
	b.SetCursorCol(-5)
	if _, c := b.Cursor(); c != 0 {
		t.Errorf("SetCursorCol(-5): %d, want 0", c)
	}
}

func TestSetModeToNormalClampsCursor(t *testing.T) {
	b := New("hello", false) // Insert, cursor at end (col 5)
	b.SetMode(Normal)
	if _, c := b.Cursor(); c != 4 {
		t.Errorf("Normal clamp: col %d, want 4", c)
	}
}
