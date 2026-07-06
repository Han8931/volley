package vimtext

import "testing"

func TestMotionCaretFirstNonBlank(t *testing.T) {
	b := drive("   hi", 4, "^")
	if _, c := b.Cursor(); c != 3 {
		t.Errorf("^ -> col %d, want 3", c)
	}
}

func TestWordMotionAcrossPunctuation(t *testing.T) {
	// charClass splits letters (1) from punctuation (2), so "foo" "." "bar"
	// are three separate words.
	b := drive("foo.bar", 0, "w")
	if _, c := b.Cursor(); c != 3 {
		t.Errorf("w onto '.': col %d, want 3", c)
	}
	b.Feed("w")
	if _, c := b.Cursor(); c != 4 {
		t.Errorf("w onto 'bar': col %d, want 4", c)
	}
	b = drive("foo.bar", 0, "e")
	if _, c := b.Cursor(); c != 2 {
		t.Errorf("e end-of-foo: col %d, want 2", c)
	}
}

func TestWordMotionCrossesLines(t *testing.T) {
	b := drive("ab\ncd", 0, "w")
	if r, c := b.Cursor(); r != 1 || c != 0 {
		t.Errorf("w across newline = %d,%d, want 1,0", r, c)
	}
	// b from the start of the second line steps back onto the first word.
	b.Feed("b")
	if r, c := b.Cursor(); r != 0 || c != 0 {
		t.Errorf("b across newline = %d,%d, want 0,0", r, c)
	}
}

func TestVerticalMotionClampsColumn(t *testing.T) {
	b := New("hello\nhi", false)
	b.SetMode(Normal)
	b.row, b.col = 0, 4
	b.Feed("j")
	if r, c := b.Cursor(); r != 1 || c != 1 {
		t.Errorf("j onto short line = %d,%d, want 1,1 (clamped)", r, c)
	}
	b.Feed("k")
	if r := func() int { r, _ := b.Cursor(); return r }(); r != 0 {
		t.Errorf("k back to row %d, want 0", r)
	}
}

func TestGotoLastAndFirstLine(t *testing.T) {
	b := New("l1\nl2\nl3\nl4", false)
	b.SetMode(Normal)
	b.Feed("G")
	if r := func() int { r, _ := b.Cursor(); return r }(); r != 3 {
		t.Errorf("G -> row %d, want 3", r)
	}
	b.Feed("g")
	b.Feed("g")
	if r := func() int { r, _ := b.Cursor(); return r }(); r != 0 {
		t.Errorf("gg -> row %d, want 0", r)
	}
}

func TestGotoLineWithCount(t *testing.T) {
	b := New("l1\nl2\nl3\nl4", false)
	b.SetMode(Normal)
	b.Feed("2")
	b.Feed("G")
	if r := func() int { r, _ := b.Cursor(); return r }(); r != 1 {
		t.Errorf("2G -> row %d, want 1", r)
	}
	b2 := New("l1\nl2\nl3\nl4", false)
	b2.SetMode(Normal)
	b2.Feed("3")
	b2.Feed("g")
	b2.Feed("g")
	if r := func() int { r, _ := b2.Cursor(); return r }(); r != 2 {
		t.Errorf("3gg -> row %d, want 2", r)
	}
}

func TestPendingGCancelsOnNonG(t *testing.T) {
	b := drive("hello", 0, "g", "x") // g arms gg; x cancels and is swallowed
	if b.Text() != "hello" {
		t.Errorf("a swallowed key after g must not edit, got %q", b.Text())
	}
	if _, c := b.Cursor(); c != 0 {
		t.Errorf("cursor moved after cancelled gg: col %d, want 0", c)
	}
}
