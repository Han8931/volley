package vimtext

import (
	"strings"
	"unicode"
)

// mul combines an operator count and a motion count (both default to 1).
func mul(a, b int) int {
	if a < 1 {
		a = 1
	}
	if b < 1 {
		b = 1
	}
	return a * b
}

// applyPendingOperator consumes the motion (or doubled operator) following d/c/y.
func (b *Buffer) applyPendingOperator(key string) (release bool) {
	op := b.pendingOp
	cnt := mul(b.opCount, b.count)
	b.pendingOp, b.opCount, b.count = 0, 0, 0

	// Doubled operator → linewise over cnt lines (dd, cc, yy).
	if key == string(op) {
		b.operateLines(op, b.row, minInt(b.row+cnt-1, len(b.lines)-1))
		return false
	}

	// cw acts like ce (Vim's special case).
	mkey := key
	if op == 'c' && key == "w" {
		mkey = "e"
	}

	tr, tc, kind, ok := b.motion(mkey, cnt)
	if !ok {
		return false
	}
	if kind == linewise {
		r1, r2 := b.row, tr
		if r1 > r2 {
			r1, r2 = r2, r1
		}
		b.operateLines(op, r1, r2)
		return false
	}
	b.operateChars(op, b.row, b.col, tr, tc, kind)
	return false
}

// operateChars applies d/c/y over a charwise range on the current line.
func (b *Buffer) operateChars(op rune, sr, sc, tr, tc int, kind motionKind) {
	forward := tr > sr || (tr == sr && tc > sc)
	line := b.lines[sr]
	start, end := sc, tc
	if tr != sr {
		// Clamp cross-line charwise ranges to the current line.
		if forward {
			end = len(line)
		} else {
			end = 0
		}
	}
	if forward {
		if kind == charInclusive {
			end++
		}
	} else {
		start, end = end, sc // backward range [target, cursor)
	}
	if start < 0 {
		start = 0
	}
	if end > len(line) {
		end = len(line)
	}
	if start >= end {
		return
	}

	b.register = []string{string(line[start:end])}
	b.regLinewise = false
	if op == 'y' {
		b.col = start
		b.clampNormal()
		return
	}
	b.pushUndo()
	b.lines[sr] = append(append([]rune(nil), line[:start]...), line[end:]...)
	b.col = start
	if op == 'c' {
		b.mode = Insert
	} else {
		b.clampNormal()
	}
}

// operateLines applies d/c/y over whole lines [r1,r2].
func (b *Buffer) operateLines(op rune, r1, r2 int) {
	if r1 < 0 {
		r1 = 0
	}
	if r2 > len(b.lines)-1 {
		r2 = len(b.lines) - 1
	}
	b.register = make([]string, 0, r2-r1+1)
	for i := r1; i <= r2; i++ {
		b.register = append(b.register, string(b.lines[i]))
	}
	b.regLinewise = true
	if op == 'y' {
		b.row, b.col = r1, 0
		b.clampNormal()
		return
	}

	b.pushUndo()
	rest := append(b.lines[:r1:r1], b.lines[r2+1:]...)
	b.lines = rest
	if op == 'c' {
		// keep one empty line to type into
		b.lines = insertLine(b.lines, r1, nil)
		b.row, b.col = r1, 0
		b.mode = Insert
		return
	}
	if len(b.lines) == 0 {
		b.lines = [][]rune{{}}
	}
	b.row = minInt(r1, len(b.lines)-1)
	b.col = firstNonBlank(b.cur())
}

// delForward removes n runes at the cursor, recording them in the register.
func (b *Buffer) delForward(n int) {
	line := b.cur()
	if b.col >= len(line) {
		return
	}
	end := minInt(b.col+n, len(line))
	b.register = []string{string(line[b.col:end])}
	b.regLinewise = false
	b.lines[b.row] = append(append([]rune(nil), line[:b.col]...), line[end:]...)
}

func (b *Buffer) deleteChars(n int, before bool) {
	b.pushUndo()
	if before {
		line := b.cur()
		start := maxInt(0, b.col-n)
		if start == b.col {
			return
		}
		b.register = []string{string(line[start:b.col])}
		b.regLinewise = false
		b.lines[b.row] = append(append([]rune(nil), line[:start]...), line[b.col:]...)
		b.col = start
		return
	}
	b.delForward(n)
	b.clampNormal()
}

// deleteCharsNoUndo is used by 's' (undo already pushed by the caller path).
func (b *Buffer) deleteCharsNoUndo(n int) {
	b.delForward(n)
}

func (b *Buffer) deleteToLineEnd() {
	b.pushUndo()
	line := b.cur()
	if b.col < len(line) {
		b.register = []string{string(line[b.col:])}
		b.regLinewise = false
		b.lines[b.row] = append([]rune(nil), line[:b.col]...)
	}
	b.col = len(b.cur())
}

func (b *Buffer) changeLines(n int) {
	b.pushUndo()
	r2 := minInt(b.row+n-1, len(b.lines)-1)
	b.register = make([]string, 0, r2-b.row+1)
	for i := b.row; i <= r2; i++ {
		b.register = append(b.register, string(b.lines[i]))
	}
	b.regLinewise = true
	b.lines = append(b.lines[:b.row:b.row], b.lines[r2+1:]...)
	b.lines = insertLine(b.lines, b.row, nil)
	b.col = 0
	b.mode = Insert
}

func (b *Buffer) toggleCase(n int) {
	b.pushUndo()
	line := b.cur()
	for i := 0; i < n && b.col < len(line); i++ {
		r := line[b.col]
		if unicode.IsUpper(r) {
			line[b.col] = unicode.ToLower(r)
		} else {
			line[b.col] = unicode.ToUpper(r)
		}
		b.col++
	}
	b.clampNormal()
}

func (b *Buffer) openLine(at int) {
	if b.singleLine {
		// A single-line buffer never grows rows: o appends at the end of the
		// line, O inserts at the start — both then enter Insert via the caller.
		if at > b.row {
			b.col = len(b.cur())
		} else {
			b.col = 0
		}
		return
	}
	if at < 0 {
		at = 0
	}
	if at > len(b.lines) {
		at = len(b.lines)
	}
	b.lines = insertLine(b.lines, at, nil)
	b.row, b.col = at, 0
}

func (b *Buffer) paste(after bool) {
	if len(b.register) == 0 {
		return
	}
	b.pushUndo()
	if b.regLinewise && !b.singleLine {
		at := b.row + 1
		if !after {
			at = b.row
		}
		for i, s := range b.register {
			b.lines = insertLine(b.lines, at+i, []rune(s))
		}
		b.row = at
		b.col = firstNonBlank(b.cur())
		return
	}
	// Charwise paste (and any paste in a single-line buffer: a linewise
	// register is flattened inline so no new rows appear).
	text := []rune(strings.Join(b.register, ""))
	line := b.cur()
	pos := b.col
	if after && len(line) > 0 {
		pos = b.col + 1
	}
	nl := append([]rune(nil), line[:pos]...)
	nl = append(nl, text...)
	nl = append(nl, line[pos:]...)
	b.lines[b.row] = nl
	b.col = maxInt(0, pos+len(text)-1)
}

func (b *Buffer) undoOnce() {
	if len(b.undo) == 0 {
		return
	}
	b.redo = append(b.redo, b.snapshot())
	s := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	b.restore(s)
	b.clampNormal()
}

func (b *Buffer) redoOnce() {
	if len(b.redo) == 0 {
		return
	}
	b.undo = append(b.undo, b.snapshot())
	s := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	b.restore(s)
	b.clampNormal()
}

// insertLine inserts line at index at, returning the grown slice.
func insertLine(lines [][]rune, at int, line []rune) [][]rune {
	lines = append(lines, nil)
	copy(lines[at+1:], lines[at:])
	lines[at] = line
	return lines
}
