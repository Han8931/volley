// Package tui implements Volley's terminal UI on top of Bubble Tea.
package tui

import (
	"net/url"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/model"
	"github.com/tabularasa/volley/internal/vars"
)

// focus identifies which pane currently receives navigation/edit keys.
type focus int

const (
	focusURL focus = iota
	focusRequest
	focusResponse
)

// Model is the root Bubble Tea model. It owns pane focus and the in-progress
// request/response; modal state is derived from whether a child is editing.
type Model struct {
	width, height int

	focus focus

	req       model.Request
	url       textinput.Model
	methodIdx int

	reqPane requestPane

	vars    vars.Store
	timeout time.Duration

	// response state
	vp          viewport.Model
	spin        spinner.Model
	sending     bool
	resp        model.Response
	hasResp     bool
	respTab     int    // 0 = body, 1 = headers
	respText    string // rendered body, kept for search
	respHeaders string // rendered headers
	pendingG    bool   // for the "gg" motion in the response viewport

	pendingWindow bool // for the "ctrl+w <hjkl>" window-navigation chord

	// command line (":" commands and "/" search)
	cmd       textinput.Model
	cmdActive bool
	cmdKind   rune // ':' or '/'

	// search state
	searchQuery string
	searchHits  []int // line offsets containing a match
	searchIdx   int

	showHelp  bool
	statusMsg string // ephemeral feedback shown in the status bar
}

// New builds the root model with a blank request ready to edit.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "https://api.example.com/v1/ping"
	ti.Prompt = ""

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	vp := viewport.New(0, 0)
	vp.KeyMap = vimViewportKeys()

	cmd := textinput.New()
	cmd.Prompt = ""

	return Model{
		req:     model.NewRequest(),
		url:     ti,
		spin:    sp,
		vp:      vp,
		cmd:     cmd,
		reqPane: newRequestPane(),
		vars:    vars.New(),
	}
}

// vimViewportKeys maps the response scroll viewport onto Vim motions.
func vimViewportKeys() viewport.KeyMap {
	return viewport.KeyMap{
		Up:           key.NewBinding(key.WithKeys("k")),
		Down:         key.NewBinding(key.WithKeys("j")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		PageUp:       key.NewBinding(key.WithKeys("ctrl+b")),
		PageDown:     key.NewBinding(key.WithKeys("ctrl+f")),
	}
}

// editing reports whether a child field is capturing keys (insert OR a
// field-level Vim normal mode). This gates pane navigation.
func (m Model) editing() bool {
	if m.url.Focused() {
		return true
	}
	return m.focus == focusRequest && m.reqPane.editing()
}

// inInsert reports whether the active field is actually inserting text, for
// the INSERT/NORMAL status tag (a field can be captured but in Vim-normal).
func (m Model) inInsert() bool {
	if m.url.Focused() {
		return true
	}
	return m.focus == focusRequest && m.reqPane.inInsert()
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		l := m.computeLayout()
		m.vp.Width = l.respInnerW
		m.vp.Height = l.respViewportH
		m.reqPane.setSize(l.reqInnerW, l.bodyInnerH)
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.showHelp {
			m.showHelp = false // any key dismisses help
			return m, nil
		}
		if m.cmdActive {
			return m.updateCommandLine(msg)
		}
		if m.editing() {
			return m.routeEditing(msg)
		}
		return m.updateNormal(msg)

	case responseMsg:
		m.sending = false
		m.resp = msg.resp
		m.hasResp = true
		m.respTab = 0
		m.respText = renderResponseBody(msg.resp, m.vp.Width)
		m.respHeaders = renderResponseHeaders(msg.resp)
		m.searchQuery, m.searchHits, m.searchIdx = "", nil, 0
		m.vp.SetContent(m.currentResponseText())
		m.vp.GotoTop()
		return m, nil

	case spinner.TickMsg:
		if !m.sending {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.url, cmd = m.url.Update(msg)
	return m, cmd
}

// routeEditing forwards keys to the active text field.
func (m Model) routeEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.url.Focused() {
		if msg.Type == tea.KeyEsc {
			m.url.Blur()
			m.req.URL = m.url.Value()
			return m, nil
		}
		var cmd tea.Cmd
		m.url, cmd = m.url.Update(msg)
		m.req.URL = m.url.Value()
		return m, cmd
	}
	cmd := m.reqPane.updateEditing(msg)
	return m, cmd
}

// updateNormal handles NORMAL-mode keys: window navigation and per-pane verbs.
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = "" // clear transient feedback on the next keystroke

	// Vim window-navigation chord: "ctrl+w" then h/j/k/l (or w to cycle).
	if m.pendingWindow {
		m.pendingWindow = false
		switch msg.String() {
		case "h":
			return m.setFocus(m.focusLeft()), nil
		case "j":
			return m.setFocus(m.focusDown()), nil
		case "k":
			return m.setFocus(m.focusUp()), nil
		case "l":
			return m.setFocus(m.focusRight()), nil
		case "w":
			return m.setFocus((m.focus + 1) % 3), nil
		}
		return m, nil // anything else cancels the chord
	}

	switch msg.String() {
	case "?":
		m.showHelp = true
		return m, nil
	case "[", "]":
		// Likewise, bracket tab navigation from the URL bar should operate on the
		// request tabs (Response keeps its own [/] handling when focused there).
		if m.focus == focusURL {
			m = m.setFocus(focusRequest)
			if msg.String() == "]" {
				m.reqPane.selectTab(m.reqPane.tab + 1)
			} else {
				m.reqPane.selectTab(m.reqPane.tab - 1)
			}
			return m, nil
		}
	case ":":
		return m.openCommandLine(':'), nil
	case "ctrl+w":
		m.pendingWindow = true
		return m, nil
	// Arrow keys also move focus directionally — reliable everywhere, and a
	// fallback for the Vim-style ctrl+h/j/k/l which collide with terminal
	// control codes (ctrl+h = Backspace, ctrl+j = Enter).
	case "left":
		return m.setFocus(m.focusLeft()), nil
	case "right":
		return m.setFocus(m.focusRight()), nil
	case "down":
		return m.setFocus(m.focusDown()), nil
	case "up":
		return m.setFocus(m.focusUp()), nil
	case "tab":
		return m.setFocus((m.focus + 1) % 3), nil
	case "shift+tab":
		return m.setFocus((m.focus + 2) % 3), nil
	case "q":
		return m, tea.Quit
	case "enter":
		return m.send()
	}

	switch m.focus {
	case focusURL:
		return m.updateURLNormal(msg)
	case focusRequest:
		cmd := m.reqPane.updateNormal(msg)
		return m, cmd
	case focusResponse:
		return m.updateResponseNav(msg)
	}
	return m, nil
}

// updateURLNormal handles NORMAL keys while the URL bar is focused. Vim h/l
// cycle the HTTP method; j moves down into the request pane.
func (m Model) updateURLNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "i", "a":
		m.url.Focus()
		m.url.CursorEnd()
		return m, textinput.Blink
	case "h":
		return m.cycleMethod(-1), nil
	case "l", "m":
		return m.cycleMethod(1), nil
	case "j":
		return m.setFocus(focusRequest), nil
	case "L":
		return m.setFocus(focusResponse), nil
	}
	return m, nil
}

func (m Model) cycleMethod(delta int) Model {
	n := len(model.Methods)
	m.methodIdx = (m.methodIdx + delta + n) % n
	m.req.Method = model.Methods[m.methodIdx]
	return m
}

// updateResponseNav handles Vim scroll motions in the response pane.
func (m Model) updateResponseNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "g":
		if m.pendingG {
			m.vp.GotoTop()
			m.pendingG = false
		} else {
			m.pendingG = true
		}
		return m, nil
	case "G":
		m.vp.GotoBottom()
		m.pendingG = false
		return m, nil
	case "[", "]":
		if m.hasResp {
			m.respTab = 1 - m.respTab
			m.resetSearch()
			m.vp.GotoTop()
		}
		return m, nil
	case "/":
		if m.hasResp {
			return m.openCommandLine('/'), nil
		}
		return m, nil
	case "n":
		return m.jumpMatch(1), nil
	case "N":
		return m.jumpMatch(-1), nil
	case "y":
		return m.yankResponse()
	}
	m.pendingG = false
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// setFocus updates focus and keeps the request pane's highlight in sync.
func (m Model) setFocus(f focus) Model {
	m.focus = f
	m.reqPane.setFocused(f == focusRequest)
	return m
}

// focus movement helpers — the layout is URL on top, Request left, Response right.
func (m Model) focusLeft() focus {
	if m.focus == focusResponse {
		return focusRequest
	}
	return m.focus
}
func (m Model) focusRight() focus {
	if m.focus == focusRequest || m.focus == focusURL {
		return focusResponse
	}
	return m.focus
}
func (m Model) focusUp() focus {
	if m.focus == focusRequest || m.focus == focusResponse {
		return focusURL
	}
	return m.focus
}
func (m Model) focusDown() focus {
	if m.focus == focusURL {
		return focusRequest
	}
	return m.focus
}

// currentResponseText returns the text shown in the response viewport for
// the active response tab.
func (m Model) currentResponseText() string {
	if m.respTab == 1 {
		return m.respHeaders
	}
	return m.respText
}

// buildRequest merges the URL bar and request pane into one Request, then
// expands {{variables}} and folds query params into the URL.
func (m Model) buildRequest() model.Request {
	req := m.req
	req.URL = m.url.Value()
	req.Headers = m.reqPane.headersOut()
	req.Query = m.reqPane.queryOut()
	req.Body = m.reqPane.bodyOut()
	req.Timeout = m.timeout

	req = m.vars.Apply(req)
	req.URL = appendQuery(req.URL, req.Query)
	return req
}

// send fires the current request (merging headers/body/query) if idle.
func (m Model) send() (tea.Model, tea.Cmd) {
	if m.sending || m.url.Value() == "" {
		return m, nil
	}
	m.sending = true
	return m, tea.Batch(m.spin.Tick, sendCmd(m.buildRequest()))
}

// appendQuery merges enabled query rows into base's query string.
func appendQuery(base string, kvs []model.KV) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for _, kv := range kvs {
		if kv.Enabled && kv.Key != "" {
			q.Add(kv.Key, kv.Value)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
