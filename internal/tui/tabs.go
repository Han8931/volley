package tui

import "github.com/tabularasa/volley/internal/model"

// tabBuf is one open request tab: a live in-memory editor buffer. Unlike a mere
// reference to a saved request, it carries its own working edits and baseline,
// so switching tabs preserves each tab's unsaved changes instead of reloading
// from disk. The active tab's live editor panes are the source of truth; its
// buffer is refreshed from the panes by syncActiveTab before switching away.
type tabBuf struct {
	name     string        // saved-request name; "" for an unsaved scratch buffer
	req      model.Request // working edits (unexpanded), captured on switch-away
	baseline model.Request // last saved/loaded state, for the per-tab dirty marker
}

// tabNames returns the open tabs' saved names, for rendering and hit-testing.
func (m Model) tabNames() []string {
	names := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		names[i] = t.name
	}
	return names
}

// syncActiveTab writes the live editor's current edits back into the active tab
// so they survive a switch to another tab. It clones the slice so retained
// copies of an earlier Model aren't mutated underneath us.
func (m Model) syncActiveTab() Model {
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return m
	}
	m.tabs = cloneTabs(m.tabs)
	m.tabs[m.activeTab] = tabBuf{
		name:     m.currentName,
		req:      m.rawRequest(),
		baseline: m.baseline,
	}
	return m
}

// loadActiveTab rebuilds the live editor from the active tab's buffer, restoring
// its edits and baseline (and thus its dirty state) rather than reading disk.
func (m Model) loadActiveTab() Model {
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return m
	}
	t := m.tabs[m.activeTab]
	m = m.applyRequest(t.req) // rebuilds panes; sets a clean baseline we override next
	m.baseline = t.baseline   // restore the tab's real baseline so dirty is preserved
	m.currentName = t.name
	return m
}

// tabDirty reports whether tab i has unsaved edits. The active tab reflects the
// live editor; the others compare their stored buffer against its baseline.
func (m Model) tabDirty(i int) bool {
	if i < 0 || i >= len(m.tabs) {
		return false
	}
	if i == m.activeTab {
		return m.dirty()
	}
	return !requestsEqual(m.tabs[i].req, m.tabs[i].baseline)
}

// backgroundTabsDirty reports whether any non-active tab buffer has unsaved
// edits. (The active tab's dirty state lives in the editor; see Model.dirty.)
func (m Model) backgroundTabsDirty() bool {
	for i := range m.tabs {
		if i != m.activeTab && m.tabDirty(i) {
			return true
		}
	}
	return false
}

// anyDirty reports whether any open buffer — the live editor or a background
// tab — has unsaved edits worth guarding before a quit discards everything.
func (m Model) anyDirty() bool {
	return m.dirty() || m.backgroundTabsDirty()
}

// dirtyBufferNames lists the names of every buffer with unsaved edits, for the
// quit prompt. An unnamed buffer reads as [No Name].
func (m Model) dirtyBufferNames() []string {
	display := func(name string) string {
		if name == "" {
			return "[No Name]"
		}
		return name
	}
	if len(m.tabs) == 0 {
		if m.dirty() {
			return []string{display(m.currentName)}
		}
		return nil
	}
	var names []string
	for i, t := range m.tabs {
		if m.tabDirty(i) {
			names = append(names, display(t.name))
		}
	}
	return names
}

// saveAllDirty persists every buffer with unsaved edits: the active editor via
// saveCurrentRequest, background tabs straight to the store (updating their
// baselines so they read clean). It reports false — with the failure in
// statusMsg — when anything could not be saved, so callers keep the user's
// edits instead of quitting past them.
func (m Model) saveAllDirty() (Model, bool) {
	if m.dirty() {
		if m.currentName == "" {
			m.statusMsg = "no name yet — use :w <name> to save, or (n) to discard"
			return m, false
		}
		var ok bool
		m, ok = m.saveCurrentRequest(m.currentName)
		if !ok {
			return m, false
		}
	}
	for i := range m.tabs {
		if i == m.activeTab || !m.tabDirty(i) {
			continue
		}
		t := m.tabs[i]
		if t.name == "" {
			m.statusMsg = "a background tab has no name — switch to it (H/L) and :w <name>"
			return m, false
		}
		if err := m.collectionStore.Save(t.name, t.req); err != nil {
			m.statusMsg = "save failed: " + t.name + ": " + err.Error()
			return m, false
		}
		tabs := cloneTabs(m.tabs)
		tabs[i].baseline = t.req
		m.tabs = tabs
	}
	return m, true
}

// renameOpenBuffers repoints the live editor and any open tab from oldName to
// newName after a successful :rename, so the tabline stays truthful and a later
// save doesn't silently recreate the old file.
func (m Model) renameOpenBuffers(oldName, newName string) Model {
	if m.currentName == oldName {
		m.currentName = newName
	}
	for i, t := range m.tabs {
		if t.name == oldName {
			tabs := cloneTabs(m.tabs)
			tabs[i].name = newName
			m.tabs = tabs
		}
	}
	return m
}

// newTabFromSaved builds a fresh tab buffer for a request just loaded from disk:
// its edits and baseline start equal, so it opens clean.
func newTabFromSaved(name string, req model.Request) tabBuf {
	return tabBuf{name: name, req: req, baseline: req}
}

func cloneTabs(tabs []tabBuf) []tabBuf {
	return append([]tabBuf(nil), tabs...)
}
