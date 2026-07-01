package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/model"
	"github.com/tabularasa/volley/internal/tui/components"
	"github.com/tabularasa/volley/internal/vimtext"
)

// request pane tabs.
const (
	tabHeaders = iota
	tabBody
	tabQuery
)

var tabNames = []string{"Headers", "Body", "Query"}

// bodyEntryKeys are the Vim keys that, from pane-normal on the Body tab,
// activate the body editor (the engine then handles cursor placement/mode).
var bodyEntryKeys = map[string]bool{
	"i": true, "a": true, "I": true, "A": true, "o": true, "O": true,
}

// requestPane is the editable left-hand pane: tabbed Headers / Body / Query.
type requestPane struct {
	tab     int
	headers components.KVEditor
	query   components.KVEditor

	body       *vimtext.Buffer
	bodyActive bool
	bodyWidth  int
	bodyHeight int
	bodyScroll int

	pendingG bool // first 'g' of the "gt"/"gT" tab motion

	focused bool
	width   int
	height  int
}

func newRequestPane() requestPane {
	body := vimtext.New("", false)
	body.SetMode(vimtext.Normal)
	return requestPane{
		headers: components.NewKVEditor("headers"),
		query:   components.NewKVEditor("query params"),
		body:    body,
	}
}

func (p *requestPane) setSize(w, h int) {
	p.width, p.height = w, h
	p.headers.SetWidth(w)
	p.query.SetWidth(w)
	p.bodyWidth = w
	p.bodyHeight = h - 2 // tab bar + blank line
	if p.bodyHeight < 1 {
		p.bodyHeight = 1
	}
}

func (p *requestPane) setFocused(f bool) {
	p.focused = f
	p.headers.SetFocused(f && p.tab == tabHeaders)
	p.query.SetFocused(f && p.tab == tabQuery)
}

// editing reports whether a child editor is actively capturing keys.
func (p requestPane) editing() bool {
	switch p.tab {
	case tabBody:
		return p.bodyActive
	case tabQuery:
		return p.query.Editing()
	default:
		return p.headers.Editing()
	}
}

// inInsert reports whether the active child is in a text-insertion mode (for
// the INSERT/NORMAL status tag).
func (p requestPane) inInsert() bool {
	switch p.tab {
	case tabBody:
		return p.bodyActive && p.body.Mode() == vimtext.Insert
	case tabQuery:
		return p.query.Editing()
	default:
		return p.headers.Editing()
	}
}

func (p requestPane) headersOut() []model.Header { return p.headers.Headers() }
func (p requestPane) queryOut() []model.KV       { return p.query.Rows() }
func (p requestPane) bodyOut() string            { return p.body.Text() }

func (p *requestPane) setRequest(req model.Request) {
	headerRows := make([]model.KV, 0, len(req.Headers))
	for _, h := range req.Headers {
		headerRows = append(headerRows, model.KV{Key: h.Name, Value: h.Value, Enabled: h.Enabled})
	}
	p.headers.SetRows(headerRows)
	p.query.SetRows(req.Query)
	p.body = vimtext.New(req.Body, false)
	p.body.SetMode(vimtext.Normal)
	p.bodyActive = false
	p.bodyScroll = 0
	p.selectTab(tabHeaders)
}

func (p *requestPane) selectTab(t int) {
	p.tab = (t + len(tabNames)) % len(tabNames)
	p.setFocused(p.focused)
}

// updateNormal handles navigation while the pane is focused and not editing.
func (p *requestPane) updateNormal(msg tea.KeyMsg) tea.Cmd {
	// "gt" / "gT" Vim tab motions.
	if p.pendingG {
		p.pendingG = false
		switch msg.String() {
		case "t":
			p.headers.CancelPending()
			p.query.CancelPending()
			p.selectTab(p.tab + 1)
			return nil
		case "T":
			p.headers.CancelPending()
			p.query.CancelPending()
			p.selectTab(p.tab - 1)
			return nil
		case "g":
			// Let Header/Query tables use the Vim-standard gg top motion. The first
			// g was forwarded below; this second one completes the table motion.
			if p.tab == tabQuery {
				p.query.UpdateNormal(msg)
			} else if p.tab == tabHeaders {
				p.headers.UpdateNormal(msg)
			}
			return nil
		}
		p.headers.CancelPending()
		p.query.CancelPending()
		return nil
	}

	switch msg.String() {
	case "g":
		p.pendingG = true
		if p.tab == tabQuery {
			p.query.UpdateNormal(msg)
		} else if p.tab == tabHeaders {
			p.headers.UpdateNormal(msg)
		}
		return nil
	case "]", "L":
		p.selectTab(p.tab + 1)
		return nil
	case "[", "H":
		p.selectTab(p.tab - 1)
		return nil
	}

	switch p.tab {
	case tabBody:
		if key := msg.String(); bodyEntryKeys[key] {
			p.bodyActive = true
			p.body.SetMode(vimtext.Normal)
			p.body.Feed(key) // engine enters Insert with the right cursor
			p.adjustBodyScroll()
		}
	case tabQuery:
		p.query.UpdateNormal(msg)
	default:
		p.headers.UpdateNormal(msg)
	}
	return nil
}

// updateEditing handles keys while a child editor captures text.
func (p *requestPane) updateEditing(msg tea.KeyMsg) tea.Cmd {
	switch p.tab {
	case tabBody:
		if release := p.body.Feed(msg.String()); release {
			p.bodyActive = false
		}
		p.adjustBodyScroll()
		return nil
	case tabQuery:
		return p.query.UpdateEditing(msg)
	default:
		return p.headers.UpdateEditing(msg)
	}
}

// adjustBodyScroll keeps the body cursor inside the visible window.
func (p *requestPane) adjustBodyScroll() {
	row, _ := p.body.Cursor()
	h := p.bodyHeight
	if h < 1 {
		h = 1
	}
	if row < p.bodyScroll {
		p.bodyScroll = row
	}
	if row >= p.bodyScroll+h {
		p.bodyScroll = row - h + 1
	}
	if p.bodyScroll < 0 {
		p.bodyScroll = 0
	}
}

func (p requestPane) view() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		p.tabBar(),
		"",
		p.tabContent(),
	)
}

func (p requestPane) tabBar() string {
	cells := make([]string, len(tabNames))
	for i, name := range tabNames {
		label := " " + name + " "
		st := lipgloss.NewStyle()
		switch {
		case i == p.tab && p.focused:
			st = st.Foreground(lipgloss.Color("#FFFFFF")).Background(colAccent).Bold(true)
		case i == p.tab:
			st = st.Foreground(colAccent).Bold(true) // active, but pane not focused
		default:
			st = st.Foreground(colDim)
		}
		cells[i] = st.Render(label)
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Left, cells...)
	if !p.focused {
		bar += dim("   focus this pane (Tab) to switch")
	} else {
		bar += dim("   H/L or [ ]")
	}
	return bar
}

func (p requestPane) tabContent() string {
	switch p.tab {
	case tabBody:
		if !p.bodyActive && p.body.Text() == "" {
			if p.focused {
				return dim("empty body — press ") + keyHint("i") + dim(" to start editing (Vim)")
			}
			return dim("empty body — focus this pane (") + keyHint("Tab") +
				dim(" or ") + keyHint("ctrl+w j") + dim("), then press ") + keyHint("i")
		}
		return p.renderBody()
	case tabQuery:
		return p.query.View()
	default:
		return p.headers.View()
	}
}

var (
	bodyCursorStyle = lipgloss.NewStyle().Reverse(true)
	jsonKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#93C5FD"))
	jsonStringStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
	jsonNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	jsonBoolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#C084FC"))
	jsonNullStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171"))
	jsonPunctStyle  = lipgloss.NewStyle().Foreground(colDim)
)

// renderBody draws the visible window of the body buffer with a block cursor
// when the editor is active.
func (p requestPane) renderBody() string {
	lines := p.body.Lines()
	cr, cc := p.body.Cursor()
	showCursor := p.bodyActive

	rows := make([]string, 0, p.bodyHeight)
	highlightJSON := isLikelyJSONBody(p.body.Text())
	for i := 0; i < p.bodyHeight; i++ {
		li := p.bodyScroll + i
		if li >= len(lines) {
			rows = append(rows, "")
			continue
		}
		if showCursor && li == cr {
			rows = append(rows, renderCursorLine(lines[li], cc))
			continue
		}
		line := truncateRunes(lines[li], p.bodyWidth)
		if highlightJSON {
			line = highlightJSONLine(line)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

// renderCursorLine renders a line with a reverse-video cell at col.
func renderCursorLine(line string, col int) string {
	r := []rune(line)
	if col > len(r) {
		col = len(r)
	}
	before := string(r[:col])
	at := " "
	after := ""
	if col < len(r) {
		at = string(r[col])
		after = string(r[col+1:])
	}
	return before + bodyCursorStyle.Render(at) + after
}

// isLikelyJSONBody reports whether body should receive lightweight JSON
// highlighting. It intentionally keys off the first non-space character so
// partially typed JSON still gets colored while editing.
func isLikelyJSONBody(body string) bool {
	trimmed := strings.TrimSpace(body)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// highlightJSONLine applies a small JSON lexer to one already-truncated line.
// It is deliberately forgiving: invalid/incomplete JSON is still readable and
// the editor remains dependency-free.
func highlightJSONLine(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		switch c := line[i]; {
		case c == '"':
			end := scanJSONString(line, i)
			tok := line[i:end]
			if isJSONKey(line, end) {
				b.WriteString(jsonKeyStyle.Render(tok))
			} else {
				b.WriteString(jsonStringStyle.Render(tok))
			}
			i = end
		case strings.ContainsRune("{}[]:,", rune(c)):
			b.WriteString(jsonPunctStyle.Render(string(c)))
			i++
		case isJSONNumberStart(c):
			end := scanJSONNumber(line, i)
			b.WriteString(jsonNumberStyle.Render(line[i:end]))
			i = end
		case strings.HasPrefix(line[i:], "true"):
			b.WriteString(jsonBoolStyle.Render("true"))
			i += len("true")
		case strings.HasPrefix(line[i:], "false"):
			b.WriteString(jsonBoolStyle.Render("false"))
			i += len("false")
		case strings.HasPrefix(line[i:], "null"):
			b.WriteString(jsonNullStyle.Render("null"))
			i += len("null")
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func scanJSONString(line string, start int) int {
	for i := start + 1; i < len(line); i++ {
		if line[i] == '\\' {
			i++ // skip escaped byte; enough for UTF-8-safe display slices here
			continue
		}
		if line[i] == '"' {
			return i + 1
		}
	}
	return len(line)
}

func isJSONKey(line string, stringEnd int) bool {
	for i := stringEnd; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t', '\r', '\n':
			continue
		case ':':
			return true
		default:
			return false
		}
	}
	return false
}

func isJSONNumberStart(c byte) bool { return c == '-' || (c >= '0' && c <= '9') }

func scanJSONNumber(line string, start int) int {
	i := start
	for i < len(line) {
		c := line[i]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			i++
			continue
		}
		break
	}
	return i
}

// truncateRunes clips s to at most w runes (no horizontal scroll yet).
func truncateRunes(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w])
}
