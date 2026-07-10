package tui

import (
	"net/url"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/model"
)

// This file owns the edited request's lifecycle: assembling the panes into a
// model.Request, detecting unsaved changes, and the y/n/esc guard that protects
// those changes before a discarding transition (open / new / quit / import).

// rawRequest assembles the current edits into a Request WITHOUT expanding
// {{variables}} or folding query params into the URL. This is the canonical
// editable form used both for saving to disk and for unsaved-changes detection.
func (m Model) rawRequest() model.Request {
	req := m.req
	req.URL = m.url.Text()
	req.Headers = m.reqPane.headersOut()
	req.Query = m.reqPane.queryOut()
	req.Body = m.reqPane.bodyOut()
	req.Auth = m.reqPane.authOut()
	req.Timeout = m.timeout
	return req
}

// buildRequest merges the URL bar and request pane into one Request, then
// expands {{variables}} and folds query params into the URL.
func (m Model) buildRequest() model.Request {
	req := m.vars.Apply(m.rawRequest())
	req = req.ApplyAuth() // turn the auth helper into a header/query param
	req.URL = appendQuery(req.URL, req.Query)
	return req
}

// dirty reports whether the current edits diverge from the last saved or loaded
// state — i.e. there are unsaved changes worth guarding before a discard.
func (m Model) dirty() bool {
	return !requestsEqual(m.rawRequest(), m.baseline)
}

// requestsEqual compares two requests field by field. A nil and an empty slice
// are treated as equal so a freshly-loaded request never reads as dirty.
func requestsEqual(a, b model.Request) bool {
	if a.Method != b.Method || a.URL != b.URL || a.Body != b.Body || a.Timeout != b.Timeout {
		return false
	}
	if a.Auth != b.Auth {
		return false
	}
	if len(a.Headers) != len(b.Headers) || len(a.Query) != len(b.Query) {
		return false
	}
	for i := range a.Headers {
		if a.Headers[i] != b.Headers[i] {
			return false
		}
	}
	for i := range a.Query {
		if a.Query[i] != b.Query[i] {
			return false
		}
	}
	return true
}

// The guarded* helpers perform a transition that would discard the current
// request, but first pop an unsaved-changes prompt when there are edits.

func (m Model) guardedOpen(name string) (tea.Model, tea.Cmd) {
	// Already open in another tab: switch to it — its buffer keeps its own
	// edits, so no unsaved-changes guard is needed and no duplicate tab appears.
	for i, t := range m.tabs {
		if t.name == name && i != m.activeTab {
			return m.switchOpenTabTo(i)
		}
	}
	if m.dirty() {
		return m.armSavePrompt(pendingOpenRequest, name), nil
	}
	return m.loadSavedRequest(name), nil
}

func (m Model) guardedNewBlank() (tea.Model, tea.Cmd) {
	if m.dirty() {
		return m.armSavePrompt(pendingNewBlank, ""), nil
	}
	return m.newBlankRequest(), nil
}

func (m Model) guardedNewSaved(name string) (tea.Model, tea.Cmd) {
	if m.dirty() {
		return m.armSavePrompt(pendingNewNamed, name), nil
	}
	return m.newSavedRequest(name), nil
}

func (m Model) guardedQuit() (tea.Model, tea.Cmd) {
	// Quitting discards every open buffer, so dirty background tabs guard it too.
	if m.anyDirty() {
		return m.armSavePrompt(pendingQuit, ""), nil
	}
	return m, tea.Quit
}

// armSavePrompt records the deferred transition and shows the y/n/esc prompt.
func (m Model) armSavePrompt(action pendingKind, arg string) Model {
	m.pendingAction = action
	m.pendingArg = arg
	verb := "continuing"
	switch action {
	case pendingOpenRequest:
		verb = "opening another request"
	case pendingNewBlank, pendingNewNamed:
		verb = "starting a new request"
	case pendingImportCurl:
		verb = "importing a curl command"
	case pendingQuit:
		verb = "quitting"
	}
	// Quitting discards every open buffer, so when background tabs also carry
	// unsaved edits the prompt lists all dirty buffers and y saves them all.
	if action == pendingQuit && m.backgroundTabsDirty() {
		m.statusMsg = "unsaved changes in " + strings.Join(m.dirtyBufferNames(), ", ") +
			" — save all before quitting? (y)es (n)o (esc)"
		return m
	}
	if m.currentName != "" {
		m.statusMsg = "unsaved changes in " + m.currentName +
			" — save before " + verb + "? (y)es (n)o (esc)"
	} else {
		m.statusMsg = "unsaved changes — (n) discard and continue, (esc) cancel · :w <name> to save"
	}
	return m
}

func (m *Model) clearSavePrompt() {
	m.pendingAction = pendingNone
	m.pendingArg = ""
}

// resolveSaveConfirm handles the key pressed while the save prompt is armed.
func (m Model) resolveSaveConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		// Quitting discards every buffer, so y saves them all; the other pending
		// transitions replace only the active editor, so y saves just that.
		if m.pendingAction == pendingQuit {
			m, ok := m.saveAllDirty()
			if !ok {
				return m, nil // failure is in statusMsg; keep the prompt armed
			}
			return m.performPending()
		}
		if m.currentName == "" {
			// Nothing to save to yet; keep the prompt so n/esc still work.
			m.statusMsg = "no name yet — use :w <name> to save, or (n) to discard"
			return m, nil
		}
		m, ok := m.saveCurrentRequest(m.currentName)
		if !ok {
			// Save failed — keep the prompt armed so the pending transition
			// doesn't discard the user's edits; the failure is in statusMsg.
			return m, nil
		}
		return m.performPending()
	case "n":
		return m.performPending()
	default:
		m.clearSavePrompt()
		m.statusMsg = "cancelled"
		return m, nil
	}
}

// performPending runs the transition that was deferred behind the save prompt.
func (m Model) performPending() (tea.Model, tea.Cmd) {
	action, arg := m.pendingAction, m.pendingArg
	m.clearSavePrompt()
	switch action {
	case pendingOpenRequest:
		return m.loadSavedRequest(arg), nil
	case pendingNewBlank:
		return m.newBlankRequest(), nil
	case pendingNewNamed:
		return m.newSavedRequest(arg), nil
	case pendingImportCurl:
		return m.applyCurlImport(arg), nil
	case pendingQuit:
		return m, tea.Quit
	}
	return m, nil
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
