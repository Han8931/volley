package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/model"
)

// storedTabModel returns a model backed by a temp store holding the named saved
// requests, all opened as real tab buffers with the first active and loaded.
func storedTabModel(t *testing.T, names ...string) Model {
	t.Helper()
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = collections.Store{Root: t.TempDir()}
	for _, n := range names {
		if err := m.collectionStore.Save(n, model.Request{Method: "GET", URL: "https://" + n + ".test"}); err != nil {
			t.Fatal(err)
		}
	}
	m.refreshCollections()
	for _, n := range names {
		req, err := m.collectionStore.Load(n)
		if err != nil {
			t.Fatal(err)
		}
		m.tabs = append(m.tabs, newTabFromSaved(n, req))
	}
	m.activeTab = 0
	m = m.loadActiveTab()
	return m
}

// twoTabModel returns a model at 120x40 with two open request tabs, active on 0.
func twoTabModel(t *testing.T) Model {
	t.Helper()
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tabs = []tabBuf{{name: "alpha"}, {name: "beta"}}
	m.activeTab = 0
	m = m.loadActiveTab() // sync currentName to the active tab
	return m
}

// The tabline is a fixed strip at the top of the right-hand column: always one
// row, with a dim hint when nothing is open, so the layout never shifts as tabs
// come and go.
func TestTablineAlwaysReserved(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.tabBarH() != 1 {
		t.Fatalf("tabBarH is fixed at 1, got %d", m.tabBarH())
	}
	hint := strings.Split(stripANSI(m.View()), "\n")[m.tablineY()]
	if !strings.Contains(hint, "no open tabs") {
		t.Fatalf("empty tabline should show the hint on row %d, got %q", m.tablineY(), hint)
	}
}

// H/L walk the open request tabs from any focused pane.
func TestHLSwitchesOpenTabs(t *testing.T) {
	m := twoTabModel(t).setFocus(focusURL)
	m = urlNormal(m) // leave URL typing mode so H/L are nav keys
	m = step(m, runes("L"))
	if m.activeTab != 1 {
		t.Fatalf("L should advance to tab 1, got %d", m.activeTab)
	}
	m = step(m, runes("H"))
	if m.activeTab != 0 {
		t.Fatalf("H should return to tab 0, got %d", m.activeTab)
	}
}

func TestRequestSubTabsUseBracketsOnly(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40}).setFocus(focusRequest)
	start := m.reqPane.tab
	m = step(m, runes("L"))
	if m.reqPane.tab != start {
		t.Fatalf("L must not switch request sub-tabs, moved %d->%d", start, m.reqPane.tab)
	}
	m = step(m, runes("]"))
	if m.reqPane.tab == start {
		t.Fatal("] should switch request sub-tabs")
	}
}

// With tabs open, H/L switch open tabs even from the request pane; its own
// sub-tabs stay on [ / ].
func TestHLSwitchesOpenTabsFromRequestPane(t *testing.T) {
	m := twoTabModel(t).setFocus(focusRequest)
	subTab := m.reqPane.tab
	m = step(m, runes("L"))
	if m.activeTab != 1 {
		t.Fatalf("L on the request pane should switch open tabs, got %d", m.activeTab)
	}
	if m.reqPane.tab != subTab {
		t.Fatalf("L should not touch the request sub-tab, moved %d->%d", subTab, m.reqPane.tab)
	}
}

// The tabline starts at the right-hand column's left edge (rightX); a click on
// a tab's name selects it, a click on its trailing ✕ flags a close, and the gap
// between tabs is a miss.
func TestOpenTabClickHitTest(t *testing.T) {
	m := twoTabModel(t)
	l := m.computeLayout()
	rightX := l.collectionInnerW + borderOverhead + l.gap
	labels := m.tabLabels()

	w0 := lipgloss.Width(labels[0])
	if idx, onClose, ok := openTabHit(rightX+2, rightX, labels); !ok || idx != 0 || onClose {
		t.Fatalf("click on tab 0 name: idx=%d close=%v ok=%v, want 0/false/true", idx, onClose, ok)
	}
	if _, onClose, ok := openTabHit(rightX+w0-1, rightX, labels); !ok || !onClose {
		t.Fatalf("click on tab 0 ✕ should flag close, ok=%v close=%v", ok, onClose)
	}
	start1 := rightX + w0 + openTabGap
	w1 := lipgloss.Width(labels[1])
	if idx, _, ok := openTabHit(start1+2, rightX, labels); !ok || idx != 1 {
		t.Fatalf("click on tab 1 name: idx=%d ok=%v, want 1/true", idx, ok)
	}
	if _, _, ok := openTabHit(start1+w1-1, rightX, labels); !ok {
		t.Fatalf("click on tab 1 ✕ should hit")
	}
	if openTabGap > 0 {
		if _, _, ok := openTabHit(rightX+w0, rightX, labels); ok {
			t.Fatal("click in the inter-tab gap should miss")
		}
	}
}

// Clicking a tab's ✕ closes that tab. Closing a background tab keeps the active
// editor untouched.
func TestClickCloseButton(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	next0, _ := m.switchOpenTabTo(2) // gamma active
	m = next0.(Model)
	l := m.computeLayout()
	rightX := l.collectionInnerW + borderOverhead + l.gap
	// ✕ of tab 0 (alpha): last cell of its label.
	closeX := rightX + lipgloss.Width(m.tabLabels()[0]) - 1
	next, _ := m.clickOpenTab(closeX, rightX)
	m = next.(Model)
	if strings.Join(m.tabNames(), ",") != "beta,gamma" {
		t.Fatalf("clicking alpha's ✕ should close it, got %v", m.tabNames())
	}
	if m.activeTab != 1 || m.currentName != "gamma" {
		t.Fatalf("closing a background tab should keep gamma active; active=%d current=%q", m.activeTab, m.currentName)
	}
}

// Pressing T repeatedly (marking one request at a time) appends tabs rather than
// replacing them, and re-opening one already open doesn't duplicate it.
func TestTOpenAppendsAndDedupes(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	m.tabs = nil // start with no tabs, like a fresh session
	m.activeTab = 0
	m = m.setFocus(focusCollection)

	openCurrent := func(name string) {
		m.collectionPane.marked = map[string]bool{name: true}
		next, _ := m.openCollectionTabs()
		m = next.(Model)
	}

	openCurrent("alpha")
	openCurrent("beta")
	if strings.Join(m.tabNames(), ",") != "alpha,beta" {
		t.Fatalf("repeated T should accumulate tabs, got %v", m.tabNames())
	}
	if m.activeTab != 1 || m.currentName != "beta" {
		t.Fatalf("opening beta should focus it; active=%d current=%q", m.activeTab, m.currentName)
	}
	openCurrent("alpha") // already open — switch, don't duplicate
	if strings.Join(m.tabNames(), ",") != "alpha,beta" {
		t.Fatalf("re-opening alpha should not duplicate, got %v", m.tabNames())
	}
	if m.activeTab != 0 {
		t.Fatalf("re-opening alpha should switch to it, active=%d", m.activeTab)
	}
}

// Opening tabs is not blocked by unsaved edits — the editor just loads the tab.
func TestTOpenIgnoresDirtyEditor(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.tabs = nil
	m = m.setFocus(focusCollection)
	m.url.SetText("https://scratch.test") // unsaved edits in the editor
	m.collectionPane.marked = map[string]bool{"alpha": true}
	next, _ := m.openCollectionTabs()
	m = next.(Model)
	if strings.Join(m.tabNames(), ",") != "alpha" {
		t.Fatalf("open should proceed despite dirty editor, got %v", m.tabNames())
	}
}

// Switching tabs preserves each tab's own unsaved edits instead of reloading
// from disk — the core of the per-tab-buffer model.
func TestTabSwitchPreservesEdits(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.url.SetText("https://alpha-edited.test") // dirty the active tab (alpha)
	if !m.dirty() {
		t.Fatal("editing the URL should dirty the active tab")
	}

	next, _ := m.switchOpenTabTo(1) // to beta — no save/discard block
	m = next.(Model)
	if m.currentName != "beta" || m.url.Text() != "https://beta.test" {
		t.Fatalf("switching to beta should show its saved state, current=%q url=%q", m.currentName, m.url.Text())
	}
	if m.dirty() {
		t.Fatal("beta was not edited, so it should be clean")
	}

	back, _ := m.switchOpenTabTo(0) // back to alpha
	m = back.(Model)
	if m.url.Text() != "https://alpha-edited.test" {
		t.Fatalf("alpha's unsaved edit should survive the round-trip, got url=%q", m.url.Text())
	}
	if !m.dirty() {
		t.Fatal("alpha should still be dirty after switching back")
	}
}

// Each tab shows its own [+]/dot dirty marker, driven by its buffer even when
// it is not the active tab.
func TestTabDirtyMarkerPerTab(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.url.SetText("https://alpha-edited.test") // dirty the active tab (alpha)

	labels := m.tabLabels()
	if !strings.Contains(labels[0], openTabDirtyGlyph) {
		t.Fatalf("active dirty tab should show the dirty glyph, got %q", labels[0])
	}
	if strings.Contains(labels[1], openTabDirtyGlyph) {
		t.Fatalf("clean background tab should not show a dirty glyph, got %q", labels[1])
	}

	next, _ := m.switchOpenTabTo(1) // beta active; alpha's dirt lives in its buffer
	m = next.(Model)
	labels = m.tabLabels()
	if !strings.Contains(labels[0], openTabDirtyGlyph) {
		t.Fatalf("alpha should stay dirty-marked from its buffer after switching away, got %q", labels[0])
	}
	if strings.Contains(labels[1], openTabDirtyGlyph) {
		t.Fatalf("beta (active, unedited) should be clean, got %q", labels[1])
	}
}

func TestTabCloseLoadsNeighbour(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	next0, _ := m.switchOpenTabTo(1) // beta active
	m = next0.(Model)
	next, _ := m.closeActiveTab()
	m = next.(Model)
	if strings.Join(m.tabNames(), ",") != "alpha,gamma" {
		t.Fatalf("tabNames = %v, want [alpha gamma]", m.tabNames())
	}
	if m.activeTab != 1 || m.currentName != "gamma" {
		t.Fatalf("after closing beta, want gamma active; active=%d current=%q", m.activeTab, m.currentName)
	}
}

func TestTabCloseLastTabClears(t *testing.T) {
	m := storedTabModel(t, "alpha")
	next, _ := m.closeActiveTab()
	m = next.(Model)
	if len(m.tabs) != 0 {
		t.Fatalf("closing the only tab should empty the open set; tabs=%v", m.tabNames())
	}
	if m.tabBarH() != 1 { // strip stays reserved, now showing the empty hint
		t.Fatalf("tabline stays reserved after the last tab closes, got %d", m.tabBarH())
	}
}

func TestTabCloseAsksWhenDirty(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.url.SetText("https://edited.test") // make the active tab dirty
	next, _ := m.closeActiveTab()
	m = next.(Model)
	if !m.confirmCloseTab || len(m.tabs) != 2 {
		t.Fatalf("dirty editor should ask before closing, confirm=%v tabs=%v", m.confirmCloseTab, m.tabNames())
	}
	cancelled, _ := m.resolveTabCloseConfirm(runes("n"))
	m = cancelled.(Model)
	if m.confirmCloseTab || len(m.tabs) != 2 {
		t.Fatalf("n should cancel tab close, confirm=%v tabs=%v", m.confirmCloseTab, m.tabNames())
	}

	next, _ = m.closeActiveTab()
	m = next.(Model)
	closed, _ := m.resolveTabCloseConfirm(runes("y"))
	m = closed.(Model)
	if strings.Join(m.tabNames(), ",") != "beta" || m.currentName != "beta" {
		t.Fatalf("y should close dirty alpha and load beta, tabs=%v current=%q", m.tabNames(), m.currentName)
	}
}

func TestTabOnlyKeepsActive(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	next0, _ := m.switchOpenTabTo(2)
	m = next0.(Model)
	next, _ := m.closeOtherTabs()
	m = next.(Model)
	if strings.Join(m.tabNames(), ",") != "gamma" || m.activeTab != 0 {
		t.Fatalf("tabonly should keep only gamma; tabs=%v active=%d", m.tabNames(), m.activeTab)
	}
}

func TestTabNewOpensAndDedupes(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta") // both saved; keep only alpha open
	m.tabs = m.tabs[:1]
	m.activeTab = 0
	m = m.loadActiveTab()
	next, _ := m.openTabByName("beta")
	m = next.(Model)
	if strings.Join(m.tabNames(), ",") != "alpha,beta" || m.activeTab != 1 {
		t.Fatalf("tabnew beta should append+focus; tabs=%v active=%d", m.tabNames(), m.activeTab)
	}
	// Re-opening an already-open tab just switches to it, no duplicate.
	next, _ = m.openTabByName("alpha")
	m = next.(Model)
	if strings.Join(m.tabNames(), ",") != "alpha,beta" || m.activeTab != 0 {
		t.Fatalf("reopening alpha should switch, not duplicate; tabs=%v active=%d", m.tabNames(), m.activeTab)
	}
}

// T selections whose files vanished behind the tree (deleted externally) must
// not panic or fabricate tabs: loadable selections still open, failed ones are
// named in the status message.
func TestTOpenReportsFailedLoads(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.tabs = nil
	m.activeTab = 0
	if err := m.collectionStore.Delete("alpha"); err != nil {
		t.Fatal(err)
	}
	// No refresh: the tree still lists alpha, as after an external deletion.
	m.collectionPane.marked = map[string]bool{"alpha": true}
	next, _ := m.openCollectionTabs()
	m = next.(Model)
	if len(m.tabs) != 0 {
		t.Fatalf("a failed load must not create a tab, got %v", m.tabNames())
	}
	if !strings.Contains(m.statusMsg, "open failed") || !strings.Contains(m.statusMsg, "alpha") {
		t.Fatalf("statusMsg should report the failed open, got %q", m.statusMsg)
	}

	m.collectionPane.marked = map[string]bool{"alpha": true, "beta": true}
	next, _ = m.openCollectionTabs()
	m = next.(Model)
	if strings.Join(m.tabNames(), ",") != "beta" || m.currentName != "beta" {
		t.Fatalf("the loadable selection should still open, tabs=%v current=%q", m.tabNames(), m.currentName)
	}
	if !strings.Contains(m.statusMsg, "failed to open: alpha") {
		t.Fatalf("a partial failure should still name the bad request, got %q", m.statusMsg)
	}
}

// Quitting guards unsaved edits in background tabs, not just the active editor:
// the prompt arms, and y saves every dirty buffer before quitting.
func TestQuitGuardsDirtyBackgroundTab(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.url.SetText("https://alpha-edited.test") // dirty alpha
	next, _ := m.switchOpenTabTo(1)            // beta active (clean); alpha dirty in background
	m = next.(Model)
	if m.dirty() {
		t.Fatal("beta was not edited, so the active editor should be clean")
	}

	q, cmd := m.guardedQuit()
	if isQuit(cmd) {
		t.Fatal("quit with a dirty background tab should prompt, not quit")
	}
	m = q.(Model)
	if m.pendingAction != pendingQuit {
		t.Fatal("quit should arm the quit prompt for the dirty background tab")
	}
	if !strings.Contains(m.statusMsg, "alpha") {
		t.Fatalf("the quit prompt should name the dirty tab, got %q", m.statusMsg)
	}

	saved, cmd := m.resolveSaveConfirm(runes("y"))
	if !isQuit(cmd) {
		t.Fatal("y should save all dirty buffers and quit")
	}
	m = saved.(Model)
	reloaded, err := m.collectionStore.Load("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.URL != "https://alpha-edited.test" {
		t.Fatalf("y should persist alpha's background edits, got %q", reloaded.URL)
	}
}

// :wqa (and the other write-quit aliases) persist dirty background tabs too,
// since quitting discards every buffer.
func TestWqSavesDirtyBackgroundTabs(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.url.SetText("https://alpha-edited.test") // dirty alpha
	next, _ := m.switchOpenTabTo(1)            // beta active (clean)
	m = next.(Model)

	_, cmd := m.executeCommand("wqa")
	if !isQuit(cmd) {
		t.Fatal(":wqa should save all and quit")
	}
	reloaded, err := m.collectionStore.Load("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.URL != "https://alpha-edited.test" {
		t.Fatalf(":wqa should persist alpha's background edits, got %q", reloaded.URL)
	}
}

// :open into the active tab keeps the tab slot in step — the tabline shows the
// newly opened name — and opening a request already open in another tab
// switches to it instead of duplicating.
func TestOpenIntoTabSyncsTabline(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	m.tabs = m.tabs[:2] // alpha, beta open; gamma saved but not open
	m.activeTab = 0
	m = m.loadActiveTab()

	next, _ := m.guardedOpen("gamma") // clean buffer: loads immediately
	m = next.(Model)
	if m.tabs[0].name != "gamma" || m.currentName != "gamma" {
		t.Fatalf("opening gamma into the active tab should rename its slot, tab=%q current=%q",
			m.tabs[0].name, m.currentName)
	}

	next, _ = m.guardedOpen("beta") // already open in tab 1: switch, don't duplicate
	m = next.(Model)
	if len(m.tabs) != 2 || m.activeTab != 1 || m.currentName != "beta" {
		t.Fatalf("opening beta should switch to its tab; tabs=%v active=%d current=%q",
			m.tabNames(), m.activeTab, m.currentName)
	}
}

// :rename repoints the live editor and any open tab at the new name, so a later
// save doesn't silently recreate the old file.
func TestRenameUpdatesOpenBuffers(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	next, _ := m.executeCommand("rename alpha zeta")
	m = next.(Model)
	if m.tabs[0].name != "zeta" {
		t.Fatalf("renaming alpha should repoint its open tab, got %q", m.tabs[0].name)
	}
	if m.currentName != "zeta" {
		t.Fatalf("renaming the active request should update currentName, got %q", m.currentName)
	}
}

// ctrl+w q closes the active tab, Vim-window style.
func TestCtrlWQClosesActiveTab(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	next0, _ := m.switchOpenTabTo(1) // beta
	m = next0.(Model)
	m = step(step(m, keyCtrlW), runes("q"))
	if strings.Join(m.tabNames(), ",") != "alpha,gamma" {
		t.Fatalf("ctrl+w q should close beta, got %v", m.tabNames())
	}
	if m.currentName != "gamma" {
		t.Fatalf("after closing beta, gamma should load, got %q", m.currentName)
	}
}

// The tabline drops one row below the tree's top border so it shares a row with
// the tree's COLLECTIONS title — the tree border stays the topmost element.
func TestTreeContentAlignsWithTabline(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	lines := strings.Split(stripANSI(m.View()), "\n")
	row := m.tablineY()
	if row < 1 {
		t.Fatalf("the tabline should sit below the tree's top border, got row %d", row)
	}
	if !strings.Contains(lines[row], "COLLECTIONS") {
		t.Fatalf("COLLECTIONS should share the tabline row %d, got %q", row, lines[row])
	}
	if !strings.Contains(lines[row], "alpha") {
		t.Fatalf("tabs should share the row with COLLECTIONS, got %q", lines[row])
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "╭") {
		t.Fatalf("row 0 should be the tree's top border, got %q", lines[0])
	}
}

// The tab strip scrolls so the active tab is always fully visible; the click
// hit-tester applies the same offset (see clickOpenTab).
func TestTabStripScrollsToActiveTab(t *testing.T) {
	labels := []string{}
	for i := 0; i < 8; i++ {
		labels = append(labels, openTabLabel("request-name-x", false)) // uniform width
	}
	w := lipgloss.Width(labels[0])

	// Everything fits: no scroll regardless of the active tab.
	if got := tabStripFirst(labels, 7, 8*(w+openTabGap)); got != 0 {
		t.Errorf("no scroll needed, first = %d, want 0", got)
	}
	// Strip holds ~two tabs: activating the last must scroll it into view.
	width := 2*w + openTabGap
	first := tabStripFirst(labels, 7, width)
	if first == 0 {
		t.Fatal("active tab 7 cannot be visible without scrolling")
	}
	// The active tab's right edge must fit within the strip.
	end := 0
	for i := first; i <= 7; i++ {
		if i > first {
			end += openTabGap
		}
		end += w
	}
	if end > width {
		t.Errorf("active tab overflows: end %d > width %d (first=%d)", end, width, first)
	}
	// The first tab stays pinned while it is the active one.
	if got := tabStripFirst(labels, 0, width); got != 0 {
		t.Errorf("active first tab must not scroll away, first = %d", got)
	}
}

// Enter (and o) on a tree request opens it as a tab — the same behavior as
// clicking the row, so keyboard and mouse never diverge.
func TestTreeEnterOpensTab(t *testing.T) {
	m := storedTabModel(t) // empty tab set, store-backed
	if err := m.collectionStore.Save("alpha", model.Request{Method: "GET", URL: "https://alpha.test"}); err != nil {
		t.Fatal(err)
	}
	m.refreshCollections()
	m = m.setFocus(focusCollection)
	// Row 0 is the root; row 1 is alpha.
	m.collectionPane.cursor = 1
	next, _ := m.updateCollectionNormal(keyEnter)
	m = next.(Model)
	if len(m.tabs) != 1 || m.tabs[0].name != "alpha" {
		t.Fatalf("enter on a tree request should open a tab, tabs = %+v", m.tabs)
	}
	if m.currentName != "alpha" {
		t.Errorf("currentName = %q, want alpha", m.currentName)
	}
}
