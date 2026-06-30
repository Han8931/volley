// Package components holds reusable TUI widgets composed by the root model.
package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/model"
)

// KVEditor is a Vim-navigable table of enabled-able key/value rows, used for
// headers and query parameters.
//
// Modal contract: the editor is either navigating (cursor moves between
// cells) or editing (an inline textinput captures a single cell). The owner
// asks Editing() to decide whether to route keys to UpdateEditing.
type KVEditor struct {
	rows    []model.KV
	cursor  int // active row
	col     int // 0 = key, 1 = value
	editing bool
	input   textinput.Model
	pendD   bool // pending first 'd' of a "dd" delete
	pendG   bool // pending first 'g' of a "gg" top motion
	focused bool

	title string
	width int
}

// NewKVEditor builds an empty editor labelled with title (e.g. "Headers").
func NewKVEditor(title string) KVEditor {
	ti := textinput.New()
	ti.Prompt = ""
	return KVEditor{title: title, input: ti}
}

// Editing reports whether an inline cell edit is in progress.
func (e KVEditor) Editing() bool { return e.editing }

// SetFocused toggles the focused highlight.
func (e *KVEditor) SetFocused(f bool) { e.focused = f }

// SetWidth sets the render width.
func (e *KVEditor) SetWidth(w int) { e.width = w }

// CancelPending clears unfinished multi-key normal-mode commands owned by the
// parent pane (for example when "gt" is used for tab switching instead of
// "gg" as a table motion).
func (e *KVEditor) CancelPending() {
	e.pendD = false
	e.pendG = false
}

// SetRows replaces the editor rows.
func (e *KVEditor) SetRows(rows []model.KV) {
	e.rows = append([]model.KV(nil), rows...)
	e.cursor, e.col = 0, 0
	e.editing = false
	e.input.Blur()
}

// Rows returns the current rows (used to build the outgoing request).
func (e KVEditor) Rows() []model.KV { return e.rows }

// Headers adapts the rows to model.Header values.
func (e KVEditor) Headers() []model.Header {
	out := make([]model.Header, 0, len(e.rows))
	for _, r := range e.rows {
		out = append(out, model.Header{Name: r.Key, Value: r.Value, Enabled: r.Enabled})
	}
	return out
}

// UpdateNormal handles navigation keys. Returns true if the key was consumed
// (so the owner won't treat it as a focus-movement key).
func (e *KVEditor) UpdateNormal(msg tea.KeyMsg) bool {
	pendD := e.pendD
	e.pendD = false
	pendG := e.pendG
	e.pendG = false

	if pendG {
		if msg.String() == "g" {
			e.cursor = 0
			return true
		}
		// Any other key cancels the pending g and falls through normally.
	}

	switch msg.String() {
	case "g":
		e.pendG = true
		return true
	case "G":
		if len(e.rows) > 0 {
			e.cursor = len(e.rows) - 1
		}
		return true
	case "j":
		if pendD && len(e.rows) > 0 {
			e.deleteRow()
			return true
		}
		if e.cursor < len(e.rows)-1 {
			e.cursor++
		}
		return true
	case "k":
		if e.cursor > 0 {
			e.cursor--
		}
		return true
	case "h", "0", "^", "b":
		e.col = 0
		return true
	case "l", "$", "w", "e":
		e.col = 1
		return true
	case "o":
		e.rows = append(e.rows, model.KV{Enabled: true})
		e.cursor = len(e.rows) - 1
		e.col = 0
		e.beginEdit()
		return true
	case "O":
		at := e.cursor
		if len(e.rows) == 0 {
			at = 0
		}
		e.rows = append(e.rows, model.KV{})
		copy(e.rows[at+1:], e.rows[at:])
		e.rows[at] = model.KV{Enabled: true}
		e.cursor = at
		e.col = 0
		e.beginEdit()
		return true
	case "i", "a", "c", "enter":
		e.ensureEditableRow()
		e.beginEdit()
		return true
	case "I":
		e.ensureEditableRow()
		e.col = 0
		e.beginEdit()
		return true
	case "A":
		e.ensureEditableRow()
		e.col = 1
		e.beginEdit()
		return true
	case " ":
		if len(e.rows) > 0 {
			e.rows[e.cursor].Enabled = !e.rows[e.cursor].Enabled
		}
		return true
	case "d":
		if pendD && len(e.rows) > 0 {
			e.deleteRow()
		} else {
			e.pendD = true
		}
		return true
	}
	return false
}

func (e *KVEditor) ensureEditableRow() {
	if len(e.rows) == 0 {
		// Create the first row so editing is immediately possible.
		e.rows = append(e.rows, model.KV{Enabled: true})
		e.cursor, e.col = 0, 0
	}
}

func (e *KVEditor) deleteRow() {
	e.rows = append(e.rows[:e.cursor], e.rows[e.cursor+1:]...)
	if e.cursor >= len(e.rows) && e.cursor > 0 {
		e.cursor--
	}
}

// beginEdit focuses the inline input on the current cell.
func (e *KVEditor) beginEdit() {
	e.editing = true
	e.input.SetValue(e.cellValue())
	e.input.CursorEnd()
	e.input.Focus()
}

// UpdateEditing handles keys during an inline cell edit.
func (e *KVEditor) UpdateEditing(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		e.commit()
		return nil
	case tea.KeyEnter:
		e.commit()
		return nil
	case tea.KeyTab:
		// Commit and hop to the value cell to keep typing.
		e.commit()
		if e.col == 0 {
			e.col = 1
			e.beginEdit()
		}
		return nil
	}
	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	return cmd
}

func (e *KVEditor) commit() {
	if e.editing && len(e.rows) > 0 {
		v := e.input.Value()
		if e.col == 0 {
			e.rows[e.cursor].Key = v
		} else {
			e.rows[e.cursor].Value = v
		}
	}
	e.editing = false
	e.input.Blur()
}

func (e KVEditor) cellValue() string {
	if len(e.rows) == 0 {
		return ""
	}
	if e.col == 0 {
		return e.rows[e.cursor].Key
	}
	return e.rows[e.cursor].Value
}

// --- styles ---

var (
	colAccent = lipgloss.Color("#7D56F4")
	colDim    = lipgloss.Color("#6C6C6C")
	colOff    = lipgloss.Color("#4B4B4B")
	colSel    = lipgloss.Color("#2A2440")
)

// View renders the table.
func (e KVEditor) View() string {
	if len(e.rows) == 0 {
		hint := "no " + e.title + " — press " +
			lipgloss.NewStyle().Foreground(colAccent).Render("o") + " to add one"
		return lipgloss.NewStyle().Foreground(colDim).Render(hint)
	}

	colW := (e.width - 6) / 2
	if colW < 6 {
		colW = 6
	}

	lines := make([]string, 0, len(e.rows))
	for i, r := range e.rows {
		mark := "[x] "
		markCol := colDim
		if !r.Enabled {
			mark, markCol = "[ ] ", colOff
		}
		line := lipgloss.NewStyle().Foreground(markCol).Render(mark) +
			e.renderCell(r.Key, "key", colW, i, 0) + " " +
			e.renderCell(r.Value, "value", colW, i, 1)
		lines = append(lines, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderCell renders one key/value cell, highlighting the active one and
// showing the live input when editing.
func (e KVEditor) renderCell(text, placeholder string, w, row, col int) string {
	active := e.focused && row == e.cursor && col == e.col
	if active && e.editing {
		text = e.input.View()
	} else if text == "" {
		text = placeholder
	}
	st := lipgloss.NewStyle().Width(w).MaxWidth(w)
	switch {
	case active:
		st = st.Foreground(lipgloss.Color("#FFFFFF")).Background(colSel)
	case text == placeholder:
		st = st.Foreground(colOff)
	default:
		st = st.Foreground(lipgloss.Color("#E5E5E5"))
	}
	return st.Render(text)
}
