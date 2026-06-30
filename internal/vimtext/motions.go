package vimtext

import "unicode"

// charClass partitions runes for word motions: 0 = space, 1 = word
// (letters/digits/underscore), 2 = other (punctuation).
func charClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	default:
		return 2
	}
}

// motionKind classifies how an operator should treat a motion's range.
type motionKind int

const (
	charExclusive motionKind = iota
	charInclusive
	linewise
)

// moveVertical moves the cursor up/down n lines. insert chooses the column
// clamp (len vs len-1).
func (b *Buffer) moveVertical(n int, insert bool) {
	b.row += n
	if b.row < 0 {
		b.row = 0
	}
	if b.row > len(b.lines)-1 {
		b.row = len(b.lines) - 1
	}
	if insert {
		b.clampInsert()
	} else {
		b.clampNormal()
	}
}

// firstNonBlank returns the column of the first non-space rune on a line.
func firstNonBlank(line []rune) int {
	for i, r := range line {
		if !unicode.IsSpace(r) {
			return i
		}
	}
	return 0
}

// nextWordStart returns the position of the next word start from (row,col).
func (b *Buffer) nextWordStart(row, col int) (int, int) {
	line := b.lines[row]
	if col < len(line) {
		cls := charClass(line[col])
		if cls != 0 {
			for col < len(line) && charClass(line[col]) == cls {
				col++
			}
		}
	}
	for {
		if col >= len(line) {
			if row >= len(b.lines)-1 {
				return row, len(line)
			}
			row++
			line = b.lines[row]
			col = 0
			if len(line) == 0 || charClass(line[0]) != 0 {
				return row, 0
			}
			continue
		}
		if charClass(line[col]) != 0 {
			return row, col
		}
		col++
	}
}

// wordEndForward returns the (inclusive) position of the next word end.
func (b *Buffer) wordEndForward(row, col int) (int, int) {
	line := b.lines[row]
	col++ // e always advances at least one
	for {
		if col >= len(line) {
			if row >= len(b.lines)-1 {
				return row, maxInt(0, len(line)-1)
			}
			row++
			line = b.lines[row]
			col = 0
			continue
		}
		if charClass(line[col]) == 0 {
			col++
			continue
		}
		// on a non-space run: go to its last rune
		cls := charClass(line[col])
		for col+1 < len(line) && charClass(line[col+1]) == cls {
			col++
		}
		return row, col
	}
}

// prevWordStart returns the position of the previous word start.
func (b *Buffer) prevWordStart(row, col int) (int, int) {
	line := b.lines[row]
	col--
	for {
		if col < 0 {
			if row == 0 {
				return 0, 0
			}
			row--
			line = b.lines[row]
			col = len(line) - 1
			if len(line) == 0 {
				return row, 0
			}
			continue
		}
		if charClass(line[col]) == 0 {
			col--
			continue
		}
		cls := charClass(line[col])
		for col-1 >= 0 && charClass(line[col-1]) == cls {
			col--
		}
		return row, col
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
