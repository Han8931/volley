package vimtext

import (
	"unicode"
	"unicode/utf8"
)

// feedInsert handles a key while in Insert mode.
func (b *Buffer) feedInsert(key string) {
	switch key {
	case "esc":
		b.mode = Normal
		// Vim moves one left when leaving insert.
		if b.col > 0 {
			b.col--
		}
		b.clampNormal()
	case "backspace":
		b.backspace()
	case "enter":
		if !b.singleLine {
			b.splitLine()
		}
	case "left":
		if b.col > 0 {
			b.col--
		}
	case "right":
		if b.col < len(b.cur()) {
			b.col++
		}
	case "up":
		b.moveVertical(-1, true)
	case "down":
		b.moveVertical(1, true)
	case "home":
		b.col = 0
	case "end":
		b.col = len(b.cur())
	case "ctrl+w":
		b.deleteWordBack()
	case "tab":
		b.insertRunes([]rune("  ")) // soft tab
	case "space":
		b.insertRunes([]rune{' '})
	default:
		// Insert printable text. Single runes and pasted runs land here;
		// control/named keys (containing "+") are ignored.
		if isInsertable(key) {
			b.insertRunes([]rune(key))
		}
	}
}

// isInsertable reports whether a key string is literal text to insert.
func isInsertable(key string) bool {
	if key == "" {
		return false
	}
	if utf8.RuneCountInString(key) == 1 {
		return true
	}
	// Multi-rune: treat as pasted text unless it looks like a chord/named key.
	for _, r := range key {
		if r == '+' {
			return false
		}
	}
	return !namedKeys[key]
}

var namedKeys = map[string]bool{
	"esc": true, "enter": true, "backspace": true, "tab": true, "space": true,
	"up": true, "down": true, "left": true, "right": true,
	"home": true, "end": true, "delete": true, "pgup": true, "pgdown": true,
}

func (b *Buffer) insertRunes(rs []rune) {
	line := b.cur()
	nl := make([]rune, 0, len(line)+len(rs))
	nl = append(nl, line[:b.col]...)
	nl = append(nl, rs...)
	nl = append(nl, line[b.col:]...)
	b.lines[b.row] = nl
	b.col += len(rs)
}

func (b *Buffer) backspace() {
	if b.col > 0 {
		line := b.cur()
		b.lines[b.row] = append(line[:b.col-1], line[b.col:]...)
		b.col--
		return
	}
	// Join with previous line.
	if b.row > 0 && !b.singleLine {
		prev := b.lines[b.row-1]
		b.col = len(prev)
		b.lines[b.row-1] = append(prev, b.cur()...)
		b.lines = append(b.lines[:b.row], b.lines[b.row+1:]...)
		b.row--
	}
}

func (b *Buffer) splitLine() {
	line := b.cur()
	head := append([]rune(nil), line[:b.col]...)
	tail := append([]rune(nil), line[b.col:]...)
	b.lines[b.row] = head
	// insert tail as a new line after row
	b.lines = append(b.lines, nil)
	copy(b.lines[b.row+2:], b.lines[b.row+1:])
	b.lines[b.row+1] = tail
	b.row++
	b.col = 0
}

func (b *Buffer) deleteWordBack() {
	if b.col == 0 {
		b.backspace()
		return
	}
	line := b.cur()
	i := b.col
	// skip trailing spaces
	for i > 0 && unicode.IsSpace(line[i-1]) {
		i--
	}
	// skip the word
	for i > 0 && !unicode.IsSpace(line[i-1]) {
		i--
	}
	b.lines[b.row] = append(line[:i], line[b.col:]...)
	b.col = i
}
