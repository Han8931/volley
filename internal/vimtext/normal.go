package vimtext

import "unicode/utf8"

// feedNormal handles a key in Normal mode; returns release=true on Esc.
func (b *Buffer) feedNormal(key string) (release bool) {
	// Awaiting the target char of "r".
	if b.pendingR {
		b.pendingR = false
		if utf8.RuneCountInString(key) == 1 {
			b.pushUndo()
			line := b.cur()
			if b.col < len(line) {
				line[b.col] = []rune(key)[0]
			}
		}
		return false
	}

	// Awaiting the second 'g' of "gg".
	if b.pendingG {
		b.pendingG = false
		if key == "g" {
			n := b.takeCount()
			b.row = n - 1
			b.col = 0
			b.clampNormal()
		}
		b.count = 0
		return false
	}

	// Numeric count prefix.
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		b.count = b.count*10 + int(key[0]-'0')
		return false
	}
	if key == "0" && b.count > 0 {
		b.count *= 10
		return false
	}

	// Operator pending: this key is its motion (or a doubled operator).
	if b.pendingOp != 0 {
		return b.applyPendingOperator(key)
	}

	switch key {
	case "esc":
		b.count = 0
		return true // release focus to pane navigation

	// --- operators (await a motion) ---
	case "d", "c", "y":
		b.pendingOp = rune(key[0])
		b.opCount = b.count
		b.count = 0
		return false

	// --- motions ---
	case "h", "l", "0", "^", "$", "w", "b", "e", "j", "k", "G":
		tr, tc, _, _ := b.motion(key, b.takeCount())
		b.row, b.col = tr, tc
		b.clampNormal()
	case "g":
		b.pendingG = true
		return false // keep any count prefix alive for the second 'g' (Ngg)

	// --- single commands ---
	case "x":
		b.deleteChars(b.takeCount(), false)
	case "X":
		b.deleteChars(b.takeCount(), true)
	case "D":
		b.deleteToLineEnd()
		b.clampNormal()
	case "C":
		b.deleteToLineEnd()
		b.mode = Insert
	case "s":
		b.pushUndo()
		b.deleteCharsNoUndo(b.takeCount())
		b.mode = Insert
	case "S":
		b.changeLines(b.takeCount())
	case "r":
		b.pendingR = true
	case "~":
		b.toggleCase(b.takeCount())

	// --- enter insert ---
	case "i":
		b.pushUndo()
		b.mode = Insert
	case "a":
		b.pushUndo()
		if b.col < len(b.cur()) {
			b.col++
		}
		b.mode = Insert
	case "I":
		b.pushUndo()
		b.col = firstNonBlank(b.cur())
		b.mode = Insert
	case "A":
		b.pushUndo()
		b.col = len(b.cur())
		b.mode = Insert
	case "o":
		b.pushUndo()
		b.openLine(b.row + 1)
		b.mode = Insert
	case "O":
		b.pushUndo()
		b.openLine(b.row)
		b.mode = Insert

	// --- paste & history ---
	case "p":
		b.paste(true)
	case "P":
		b.paste(false)
	case "u":
		b.undoOnce()
	case "ctrl+r":
		b.redoOnce()
	}

	b.count = 0
	return false
}

// takeCount returns the pending count (min 1) without clearing accumulation
// that later code clears via the trailing b.count = 0.
func (b *Buffer) takeCount() int {
	if b.count < 1 {
		return 1
	}
	return b.count
}

// motion resolves a motion key to a target position and kind.
func (b *Buffer) motion(key string, cnt int) (int, int, motionKind, bool) {
	line := b.cur()
	switch key {
	case "h":
		return b.row, maxInt(0, b.col-cnt), charExclusive, true
	case "l":
		return b.row, b.col + cnt, charExclusive, true
	case "0":
		return b.row, 0, charExclusive, true
	case "^":
		return b.row, firstNonBlank(line), charExclusive, true
	case "$":
		return b.row, maxInt(0, len(line)-1), charInclusive, true
	case "w":
		r, c := b.row, b.col
		for i := 0; i < cnt; i++ {
			r, c = b.nextWordStart(r, c)
		}
		return r, c, charExclusive, true
	case "e":
		r, c := b.row, b.col
		for i := 0; i < cnt; i++ {
			r, c = b.wordEndForward(r, c)
		}
		return r, c, charInclusive, true
	case "b":
		r, c := b.row, b.col
		for i := 0; i < cnt; i++ {
			r, c = b.prevWordStart(r, c)
		}
		return r, c, charExclusive, true
	case "j":
		return minInt(b.row+cnt, len(b.lines)-1), b.col, linewise, true
	case "k":
		return maxInt(b.row-cnt, 0), b.col, linewise, true
	case "G":
		if b.count > 0 {
			return minInt(cnt-1, len(b.lines)-1), 0, linewise, true
		}
		return len(b.lines) - 1, 0, linewise, true
	}
	return b.row, b.col, charExclusive, false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
