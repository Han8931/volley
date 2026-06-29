// Package tui implements Volley's terminal UI on top of Bubble Tea.
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/model"
	"github.com/tabularasa/volley/internal/tui/keys"
)

// focus identifies which pane currently receives navigation/edit keys.
type focus int

const (
	focusURL focus = iota
	focusRequest
	focusResponse
)

// Model is the root Bubble Tea model. It owns the editor mode, pane focus,
// and the in-progress request/response.
type Model struct {
	width, height int

	mode  keys.Mode
	focus focus

	req       model.Request
	url       textinput.Model
	methodIdx int

	// response state
	vp      viewport.Model
	spin    spinner.Model
	sending bool
	resp    model.Response
	hasResp bool

	pendingG bool // for the "gg" motion in the response viewport
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

	return Model{
		mode: keys.Normal,
		req:  model.NewRequest(),
		url:  ti,
		spin: sp,
		vp:   vp,
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
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.mode == keys.Insert {
			return m.updateInsert(msg)
		}
		return m.updateNormal(msg)

	case responseMsg:
		m.sending = false
		m.resp = msg.resp
		m.hasResp = true
		m.vp.SetContent(renderResponseBody(msg.resp, m.vp.Width))
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

// updateNormal handles keys while in NORMAL mode: navigation and commands.
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While the response pane is focused, most keys drive the scroll viewport.
	if m.focus == focusResponse {
		return m.updateResponseNav(msg)
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "enter":
		return m.send()
	case "l":
		if m.focus == focusRequest {
			m.focus = focusResponse
		}
	case "j":
		if m.focus == focusURL {
			m.focus = focusRequest
		}
	case "k":
		if m.focus == focusRequest {
			m.focus = focusURL
		}
	case "tab":
		m.focus = (m.focus + 1) % 3
	case "i", "a":
		if m.focus == focusURL {
			m.mode = keys.Insert
			m.url.Focus()
			return m, textinput.Blink
		}
	case "m":
		if m.focus == focusURL {
			m.methodIdx = (m.methodIdx + 1) % len(model.Methods)
			m.req.Method = model.Methods[m.methodIdx]
		}
	}
	return m, nil
}

// updateResponseNav handles keys while the response pane is focused: Vim
// scroll motions plus a few keys that leave the pane or fire a request.
func (m Model) updateResponseNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "enter":
		return m.send()
	case "h":
		m.focus = focusRequest
		m.pendingG = false
		return m, nil
	case "tab":
		m.focus = (m.focus + 1) % 3
		m.pendingG = false
		return m, nil
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
	}
	m.pendingG = false
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// send fires the current request if one isn't already in flight.
func (m Model) send() (tea.Model, tea.Cmd) {
	if m.sending || m.url.Value() == "" {
		return m, nil
	}
	m.req.URL = m.url.Value()
	m.sending = true
	return m, tea.Batch(m.spin.Tick, sendCmd(m.req))
}

// updateInsert handles keys while editing a text field.
func (m Model) updateInsert(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.mode = keys.Normal
		m.url.Blur()
		m.req.URL = m.url.Value()
		return m, nil
	}
	var cmd tea.Cmd
	m.url, cmd = m.url.Update(msg)
	m.req.URL = m.url.Value()
	return m, cmd
}
