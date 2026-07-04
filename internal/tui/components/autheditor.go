package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/model"
)

// AuthEditor is a small Vim-navigable form for a request's auth helper: a type
// selector (None / Bearer / Basic / API Key) plus the fields that type needs.
// It mirrors KVEditor's modal contract — navigating (j/k between fields) or
// editing (an inline textinput captures one field) — so the owning pane routes
// keys the same way for both.
type AuthEditor struct {
	auth    model.Auth
	cursor  int // index into the currently-visible fields
	editing bool
	input   textinput.Model
	focused bool
	width   int
}

// authFieldID identifies one row of the form. Which rows are visible depends on
// the selected auth type (see visibleFields).
type authFieldID int

const (
	afType authFieldID = iota
	afToken
	afUsername
	afPassword
	afKey
	afValue
	afLocation
)

// authTypes is the cycle order of the type selector.
var authTypes = []string{model.AuthNone, model.AuthBearer, model.AuthBasic, model.AuthAPIKey}

// NewAuthEditor builds an empty editor (Type: None).
func NewAuthEditor() AuthEditor {
	ti := textinput.New()
	ti.Prompt = ""
	return AuthEditor{input: ti}
}

// Editing reports whether an inline field edit is in progress.
func (e AuthEditor) Editing() bool { return e.editing }

// SetFocused toggles the focused highlight.
func (e *AuthEditor) SetFocused(f bool) { e.focused = f }

// SetWidth sets the render width.
func (e *AuthEditor) SetWidth(w int) { e.width = w }

// SetAuth replaces the edited auth and resets the cursor.
func (e *AuthEditor) SetAuth(a model.Auth) {
	e.auth = a
	e.cursor = 0
	e.editing = false
	e.input.Blur()
}

// Auth returns the current auth (used to build the outgoing request).
func (e AuthEditor) Auth() model.Auth { return e.auth }

// visibleFields lists the rows shown for the current type, in order.
func (e AuthEditor) visibleFields() []authFieldID {
	switch e.auth.Type {
	case model.AuthBearer:
		return []authFieldID{afType, afToken}
	case model.AuthBasic:
		return []authFieldID{afType, afUsername, afPassword}
	case model.AuthAPIKey:
		return []authFieldID{afType, afKey, afValue, afLocation}
	default:
		return []authFieldID{afType}
	}
}

func (e AuthEditor) currentField() authFieldID {
	fields := e.visibleFields()
	if e.cursor < 0 || e.cursor >= len(fields) {
		return afType
	}
	return fields[e.cursor]
}

// UpdateNormal handles navigation keys. Returns true if the key was consumed.
func (e *AuthEditor) UpdateNormal(msg tea.KeyMsg) bool {
	fields := e.visibleFields()
	switch msg.String() {
	case "j":
		if e.cursor < len(fields)-1 {
			e.cursor++
		}
		return true
	case "k":
		if e.cursor > 0 {
			e.cursor--
		}
		return true
	case "g":
		e.cursor = 0
		return true
	case "G":
		e.cursor = len(fields) - 1
		return true
	}

	switch e.currentField() {
	case afType:
		switch msg.String() {
		case " ", "l", "enter":
			e.cycleType(1)
			return true
		case "h":
			e.cycleType(-1)
			return true
		}
	case afLocation:
		switch msg.String() {
		case " ":
			e.auth.InQuery = !e.auth.InQuery
			return true
		case "h":
			e.auth.InQuery = false
			return true
		case "l":
			e.auth.InQuery = true
			return true
		}
	default: // a text field
		switch msg.String() {
		case "i", "a", "c", "enter", "I", "A":
			e.beginEdit()
			return true
		}
	}
	return false
}

// cycleType advances the auth type by dir (+1/-1) and clamps the cursor to the
// new field set.
func (e *AuthEditor) cycleType(dir int) {
	idx := 0
	for i, t := range authTypes {
		if t == e.auth.Type {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(authTypes)) % len(authTypes)
	e.auth.Type = authTypes[idx]
	if n := len(e.visibleFields()); e.cursor >= n {
		e.cursor = n - 1
	}
}

// beginEdit focuses the inline input on the current text field.
func (e *AuthEditor) beginEdit() {
	e.editing = true
	e.input.SetValue(e.textValue(e.currentField()))
	e.input.CursorEnd()
	e.input.Focus()
}

// UpdateEditing handles keys during an inline field edit.
func (e *AuthEditor) UpdateEditing(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter:
		e.commit()
		return nil
	}
	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	return cmd
}

func (e *AuthEditor) commit() {
	if e.editing {
		e.setText(e.currentField(), e.input.Value())
	}
	e.editing = false
	e.input.Blur()
}

func (e AuthEditor) textValue(id authFieldID) string {
	switch id {
	case afToken:
		return e.auth.Token
	case afUsername:
		return e.auth.Username
	case afPassword:
		return e.auth.Password
	case afKey:
		return e.auth.Key
	case afValue:
		return e.auth.Value
	}
	return ""
}

func (e *AuthEditor) setText(id authFieldID, v string) {
	switch id {
	case afToken:
		e.auth.Token = v
	case afUsername:
		e.auth.Username = v
	case afPassword:
		e.auth.Password = v
	case afKey:
		e.auth.Key = v
	case afValue:
		e.auth.Value = v
	}
}

// --- rendering ---

var authTypeLabels = map[string]string{
	model.AuthNone:   "None",
	model.AuthBearer: "Bearer Token",
	model.AuthBasic:  "Basic",
	model.AuthAPIKey: "API Key",
}

func fieldLabel(id authFieldID) string {
	switch id {
	case afType:
		return "Type"
	case afToken:
		return "Token"
	case afUsername:
		return "Username"
	case afPassword:
		return "Password"
	case afKey:
		return "Key"
	case afValue:
		return "Value"
	case afLocation:
		return "Add to"
	}
	return ""
}

// View renders the form.
func (e AuthEditor) View() string {
	fields := e.visibleFields()
	lines := make([]string, 0, len(fields)+1)
	for i, id := range fields {
		lines = append(lines, e.renderRow(id, i))
	}
	if e.auth.Type == model.AuthNone {
		hint := "no auth — press " +
			lipgloss.NewStyle().Foreground(colAccent).Render("space") +
			" on Type to choose Bearer / Basic / API Key"
		lines = append(lines, "", lipgloss.NewStyle().Foreground(colDim).Render(hint))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (e AuthEditor) renderRow(id authFieldID, row int) string {
	label := lipgloss.NewStyle().Foreground(colDim).Width(10).Render(fieldLabel(id))
	active := e.focused && row == e.cursor

	var val string
	switch id {
	case afType:
		val = authTypeLabels[e.auth.Type]
		if active {
			val = "‹ " + val + " ›"
		}
	case afLocation:
		val = "Header"
		if e.auth.InQuery {
			val = "Query param"
		}
		if active {
			val = "‹ " + val + " ›"
		}
	default:
		val = e.fieldDisplay(id, active)
	}

	st := lipgloss.NewStyle()
	switch {
	case active && e.editing:
		// input.View already shows the live cursor
	case active:
		st = st.Foreground(lipgloss.Color("#FFFFFF")).Background(colSel)
	default:
		st = st.Foreground(lipgloss.Color("#E5E5E5"))
	}
	return label + " " + st.Render(val)
}

// fieldDisplay renders a text field's value (masking passwords), the live input
// while editing, or a dim placeholder when empty.
func (e AuthEditor) fieldDisplay(id authFieldID, active bool) string {
	if active && e.editing {
		return e.input.View()
	}
	v := e.textValue(id)
	if v == "" {
		return lipgloss.NewStyle().Foreground(colOff).Render("(empty)")
	}
	if id == afPassword {
		return strings.Repeat("•", len([]rune(v)))
	}
	return v
}
