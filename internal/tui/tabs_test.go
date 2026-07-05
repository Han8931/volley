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
// requests, all opened as tabs with the first active.
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
	m.openTabs = append([]string(nil), names...)
	m.activeTab = 0
	m = m.loadSavedRequest(names[0]) // clean baseline, as openCollectionTabs leaves it
	return m
}

// twoTabModel returns a model at 120x40 with two open request tabs, active on 0.
func twoTabModel(t *testing.T) Model {
	t.Helper()
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.openTabs = []string{"alpha", "beta"}
	m.activeTab = 0
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

// H/L walk the open tabs everywhere except the request pane, where they must
// stay bound to the pane's own sub-tab switching.
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

	w0 := lipgloss.Width(openTabLabel("alpha"))
	if idx, onClose, ok := openTabHit(rightX+2, rightX, m.openTabs); !ok || idx != 0 || onClose {
		t.Fatalf("click on tab 0 name: idx=%d close=%v ok=%v, want 0/false/true", idx, onClose, ok)
	}
	if _, onClose, ok := openTabHit(rightX+w0-1, rightX, m.openTabs); !ok || !onClose {
		t.Fatalf("click on tab 0 ✕ should flag close, ok=%v close=%v", ok, onClose)
	}
	start1 := rightX + w0 + openTabGap
	w1 := lipgloss.Width(openTabLabel("beta"))
	if idx, _, ok := openTabHit(start1+2, rightX, m.openTabs); !ok || idx != 1 {
		t.Fatalf("click on tab 1 name: idx=%d ok=%v, want 1/true", idx, ok)
	}
	if _, _, ok := openTabHit(start1+w1-1, rightX, m.openTabs); !ok {
		t.Fatalf("click on tab 1 ✕ should hit")
	}
	if openTabGap > 0 {
		if _, _, ok := openTabHit(rightX+w0, rightX, m.openTabs); ok {
			t.Fatal("click in the inter-tab gap should miss")
		}
	}
}

// Clicking a tab's ✕ closes that tab. Closing a background tab keeps the active
// editor untouched.
func TestClickCloseButton(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	m.activeTab = 2
	m = m.loadSavedRequest("gamma") // gamma active and loaded
	l := m.computeLayout()
	rightX := l.collectionInnerW + borderOverhead + l.gap
	// ✕ of tab 0 (alpha): last cell of its label.
	closeX := rightX + lipgloss.Width(openTabLabel("alpha")) - 1
	next, _ := m.clickOpenTab(closeX, rightX)
	m = next.(Model)
	if strings.Join(m.openTabs, ",") != "beta,gamma" {
		t.Fatalf("clicking alpha's ✕ should close it, got %v", m.openTabs)
	}
	if m.activeTab != 1 || m.currentName != "gamma" {
		t.Fatalf("closing a background tab should keep gamma active; active=%d current=%q", m.activeTab, m.currentName)
	}
}

// Pressing T repeatedly (marking one request at a time) appends tabs rather than
// replacing them, and re-opening one already open doesn't duplicate it.
func TestTOpenAppendsAndDedupes(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	m.openTabs = nil // start with no tabs, like a fresh session
	m.activeTab = 0
	m = m.setFocus(focusCollection)

	openCurrent := func(name string) {
		m.collectionPane.marked = map[string]bool{name: true}
		next, _ := m.openCollectionTabs()
		m = next.(Model)
	}

	openCurrent("alpha")
	openCurrent("beta")
	if strings.Join(m.openTabs, ",") != "alpha,beta" {
		t.Fatalf("repeated T should accumulate tabs, got %v", m.openTabs)
	}
	if m.activeTab != 1 || m.currentName != "beta" {
		t.Fatalf("opening beta should focus it; active=%d current=%q", m.activeTab, m.currentName)
	}
	openCurrent("alpha") // already open — switch, don't duplicate
	if strings.Join(m.openTabs, ",") != "alpha,beta" {
		t.Fatalf("re-opening alpha should not duplicate, got %v", m.openTabs)
	}
	if m.activeTab != 0 {
		t.Fatalf("re-opening alpha should switch to it, active=%d", m.activeTab)
	}
}

// Opening tabs is not blocked by unsaved edits — the editor just loads the tab.
func TestTOpenIgnoresDirtyEditor(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.openTabs = nil
	m = m.setFocus(focusCollection)
	m.url.SetText("https://scratch.test") // unsaved edits in the editor
	m.collectionPane.marked = map[string]bool{"alpha": true}
	next, _ := m.openCollectionTabs()
	m = next.(Model)
	if len(m.openTabs) != 1 || m.openTabs[0] != "alpha" {
		t.Fatalf("open should proceed despite dirty editor, got %v", m.openTabs)
	}
}

func TestTabCloseLoadsNeighbour(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	m.activeTab = 1 // beta active
	next, _ := m.closeActiveTab()
	m = next.(Model)
	if strings.Join(m.openTabs, ",") != "alpha,gamma" {
		t.Fatalf("openTabs = %v, want [alpha gamma]", m.openTabs)
	}
	if m.activeTab != 1 || m.currentName != "gamma" {
		t.Fatalf("after closing beta, want gamma active; active=%d current=%q", m.activeTab, m.currentName)
	}
}

func TestTabCloseLastTabClears(t *testing.T) {
	m := storedTabModel(t, "alpha")
	next, _ := m.closeActiveTab()
	m = next.(Model)
	if len(m.openTabs) != 0 {
		t.Fatalf("closing the only tab should empty the open set; openTabs=%v", m.openTabs)
	}
	if m.tabBarH() != 1 { // strip stays reserved, now showing the empty hint
		t.Fatalf("tabline stays reserved after the last tab closes, got %d", m.tabBarH())
	}
}

func TestTabCloseAsksWhenDirty(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta")
	m.url.SetText("https://edited.test") // make it dirty
	next, _ := m.closeActiveTab()
	m = next.(Model)
	if !m.confirmCloseTab || len(m.openTabs) != 2 {
		t.Fatalf("dirty editor should ask before closing, confirm=%v openTabs=%v", m.confirmCloseTab, m.openTabs)
	}
	cancelled, _ := m.resolveTabCloseConfirm(runes("n"))
	m = cancelled.(Model)
	if m.confirmCloseTab || len(m.openTabs) != 2 {
		t.Fatalf("n should cancel tab close, confirm=%v openTabs=%v", m.confirmCloseTab, m.openTabs)
	}

	next, _ = m.closeActiveTab()
	m = next.(Model)
	closed, _ := m.resolveTabCloseConfirm(runes("y"))
	m = closed.(Model)
	if strings.Join(m.openTabs, ",") != "beta" || m.currentName != "beta" {
		t.Fatalf("y should close dirty alpha and load beta, openTabs=%v current=%q", m.openTabs, m.currentName)
	}
}

func TestTabOnlyKeepsActive(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	m.activeTab = 2
	next, _ := m.closeOtherTabs()
	m = next.(Model)
	if strings.Join(m.openTabs, ",") != "gamma" || m.activeTab != 0 {
		t.Fatalf("tabonly should keep only gamma; openTabs=%v active=%d", m.openTabs, m.activeTab)
	}
}

func TestTabNewOpensAndDedupes(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta") // both saved; open only alpha as a tab
	m.openTabs = []string{"alpha"}
	m.activeTab = 0
	m = m.loadSavedRequest("alpha")
	next, _ := m.openTabByName("beta")
	m = next.(Model)
	if strings.Join(m.openTabs, ",") != "alpha,beta" || m.activeTab != 1 {
		t.Fatalf("tabnew beta should append+focus; openTabs=%v active=%d", m.openTabs, m.activeTab)
	}
	// Re-opening an already-open tab just switches to it, no duplicate.
	next, _ = m.openTabByName("alpha")
	m = next.(Model)
	if strings.Join(m.openTabs, ",") != "alpha,beta" || m.activeTab != 0 {
		t.Fatalf("reopening alpha should switch, not duplicate; openTabs=%v active=%d", m.openTabs, m.activeTab)
	}
}

// ctrl+w q closes the active tab, Vim-window style.
func TestCtrlWQClosesActiveTab(t *testing.T) {
	m := storedTabModel(t, "alpha", "beta", "gamma")
	m.activeTab = 1 // beta
	m = step(step(m, keyCtrlW), runes("q"))
	if strings.Join(m.openTabs, ",") != "alpha,gamma" {
		t.Fatalf("ctrl+w q should close beta, got %v", m.openTabs)
	}
	if m.currentName != "gamma" {
		t.Fatalf("after closing beta, gamma should load, got %q", m.currentName)
	}
}

// The tree gets a blank leading line matching the tabline height, so its
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
