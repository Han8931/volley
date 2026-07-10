package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/model"
)

func TestTreeRowsShowMethod(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = collections.Store{Root: t.TempDir()}
	if err := m.collectionStore.Save("api_test_1", model.Request{Method: "POST", URL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	m.refreshCollections()

	var found bool
	for _, r := range m.collectionPane.rows() {
		if r.file && r.name == "api_test_1" {
			found = true
			if r.method != "POST" {
				t.Errorf("row method = %q, want POST", r.method)
			}
		}
	}
	if !found {
		t.Fatal("api_test_1 row not found in tree")
	}
	// The rendered tree shows the method as a Bruno-style prefix.
	if got := stripANSI(m.collectionPane.view()); !strings.Contains(got, "POST api_test_1") {
		t.Errorf("tree view should contain %q, got:\n%s", "POST api_test_1", got)
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = collections.Store{Root: t.TempDir()}
	if err := m.collectionStore.Save("a/b", model.NewRequest()); err != nil {
		t.Fatal(err)
	}
	m.refreshCollections()

	// Arming does not delete; the confirmation is pending.
	m = m.askDeleteConfirm("a/b", false)
	if m.confirmDelete != "a/b" {
		t.Fatal("delete should be pending confirmation")
	}

	// 'n' cancels and the request survives.
	cancelled := step(m, runes("n"))
	if cancelled.confirmDelete != "" {
		t.Error("n should clear the pending delete")
	}
	if _, err := cancelled.collectionStore.Load("a/b"); err != nil {
		t.Error("request must NOT be deleted after 'n'")
	}

	// 'y' confirms and the request is gone.
	confirmed := step(m, runes("y"))
	if _, err := confirmed.collectionStore.Load("a/b"); err == nil {
		t.Error("request should be deleted after 'y'")
	}
}

func TestTreeShowsEmptyAndNestedGroups(t *testing.T) {
	items := []collections.Item{
		{Name: "APISet1", IsDir: true},
		{Name: "APISet1/auth", IsDir: true},
		{Name: "APISet1/auth/login"},
		{Name: "APISet1/getUsers"},
		{Name: "APISet2", IsDir: true}, // empty group
	}
	p := newCollectionPane(items, "~/x")
	rows := p.rows()

	// Everything is expanded by default; verify structure & depths.
	want := []struct {
		name  string
		file  bool
		depth int
	}{
		{"", false, 0}, // root group for top-level actions
		{"APISet1", false, 1},
		{"APISet1/auth", false, 2},
		{"APISet1/auth/login", true, 3},
		{"APISet1/getUsers", true, 2},
		{"APISet2", false, 1}, // empty group still shown
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i].name != w.name || rows[i].file != w.file || rows[i].depth != w.depth {
			t.Errorf("row %d = {%q file=%v depth=%d}, want {%q file=%v depth=%d}",
				i, rows[i].name, rows[i].file, rows[i].depth, w.name, w.file, w.depth)
		}
	}
}

func TestTreeRecursiveCollapseAndParentJump(t *testing.T) {
	items := []collections.Item{
		{Name: "g/sub/a"}, {Name: "g/sub/b"}, {Name: "g/c"},
	}
	p := newCollectionPane(items, "")
	p.focused = true

	// Collapse "g" recursively -> root plus the top group row remain.
	p.setExpandedRecursive("g", false)
	if n := len(p.rows()); n != 2 {
		t.Fatalf("after recursive collapse rows = %d, want 2", n)
	}

	// Re-expand and jump from a deep file to its parent folder.
	p.setExpandedRecursive("g", true)
	for i, r := range p.rows() {
		if r.name == "g/sub/a" {
			p.cursor = i
		}
	}
	p.jumpToParent()
	if row, _ := p.current(); row.name != "g/sub" || row.file {
		t.Errorf("jumpToParent landed on %+v, want folder g/sub", row)
	}
}

func TestMkgroupCommandCreatesVisibleGroup(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = collections.Store{Root: t.TempDir()}
	m.refreshCollections()

	next, _ := m.executeCommand("mkgroup APISet1")
	m = next.(Model)

	found := false
	for _, r := range m.collectionPane.rows() {
		if r.name == "APISet1" && !r.file {
			found = true
		}
	}
	if !found {
		t.Errorf("mkgroup should create a visible group; rows = %+v", m.collectionPane.rows())
	}
}

func TestGroupDeleteConfirmation(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = collections.Store{Root: t.TempDir()}
	if err := m.collectionStore.Save("APISet1/login", model.NewRequest()); err != nil {
		t.Fatal(err)
	}
	m.refreshCollections()

	m = m.askDeleteConfirm("APISet1", true)
	if !m.confirmGroup {
		t.Fatal("group delete should set confirmGroup")
	}
	confirmed := step(m, runes("y"))
	if items, _ := confirmed.collectionStore.List(); len(items) != 0 {
		t.Errorf("group and its request should be gone, got %+v", items)
	}
}

func TestTreeATogglesWidePane(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40}).setFocus(focusCollection)
	normalW := m.computeLayout().collectionInnerW

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = next.(Model)
	wideW := m.computeLayout().collectionInnerW
	if !m.collectionWide || wideW <= normalW {
		t.Fatalf("A should widen the tree, normal=%d wide=%d collectionWide=%v", normalW, wideW, m.collectionWide)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = next.(Model)
	if m.collectionWide || m.computeLayout().collectionInnerW != normalW {
		t.Fatalf("second A should restore tree width, got width=%d collectionWide=%v", m.computeLayout().collectionInnerW, m.collectionWide)
	}
}

func TestSpaceMarksRequestAndMovesCursorDown(t *testing.T) {
	items := []collections.Item{{Name: "a", Method: "GET"}, {Name: "b", Method: "POST"}}
	p := newCollectionPane(items, "~/x")
	p.focused = true
	p.width = 24
	p.cursor = 1 // first file; row 0 is root

	p.updateNormal(tea.KeyMsg{Type: tea.KeySpace})
	if !p.marked["a"] {
		t.Fatal("space should mark the request under the cursor")
	}
	if p.cursor != 2 {
		t.Fatalf("space should move cursor down one row, cursor=%d", p.cursor)
	}
	if got := stripANSI(p.view()); strings.Contains(got, "☑") || strings.Contains(got, "☐") {
		t.Fatalf("marked request should not render checkbox markers, got:\n%s", got)
	} else if !strings.Contains(got, "GET a") {
		t.Fatalf("marked request should still render method and name, got:\n%s", got)
	}

	p.updateNormal(tea.KeyMsg{Type: tea.KeySpace})
	if !p.marked["b"] {
		t.Fatal("space on next request should mark it too")
	}
	if p.cursor != 2 {
		t.Fatalf("space at the bottom should stay on the last row, cursor=%d", p.cursor)
	}
}

func TestTOpensMarkedRequestsAsTabs(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40}).setFocus(focusCollection)
	m.collectionStore = collections.Store{Root: t.TempDir()}
	if err := m.collectionStore.Save("a", model.Request{Method: "GET", URL: "https://a.test"}); err != nil {
		t.Fatal(err)
	}
	if err := m.collectionStore.Save("b", model.Request{Method: "POST", URL: "https://b.test"}); err != nil {
		t.Fatal(err)
	}
	m.refreshCollections()
	m.collectionPane.marked["a"] = true
	m.collectionPane.marked["b"] = true

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = next.(Model)
	if len(m.tabs) != 2 || m.tabs[0].name != "a" || m.tabs[1].name != "b" {
		t.Fatalf("tabs = %#v, want [a b]", m.tabNames())
	}
	if m.currentName != "a" || m.url.Text() != "https://a.test" {
		t.Fatalf("T should load first opened tab, current=%q url=%q", m.currentName, m.url.Text())
	}
	if got := stripANSI(m.docNameSeg()); !strings.Contains(got, "[1/2]") {
		t.Fatalf("doc segment should show tab count, got %q", got)
	}
	tabLine := strings.Split(stripANSI(m.View()), "\n")[m.tablineY()]
	if !strings.Contains(tabLine, "a") || !strings.Contains(tabLine, "b") {
		t.Fatalf("request tabs should render on the tabline row, got %q", tabLine)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	m = next.(Model)
	if m.activeTab != 1 || m.currentName != "b" {
		t.Fatalf("L should switch to next tab, active=%d current=%q", m.activeTab, m.currentName)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	m = next.(Model)
	if m.activeTab != 0 || m.currentName != "a" {
		t.Fatalf("H should switch to previous tab, active=%d current=%q", m.activeTab, m.currentName)
	}
}

func TestCollectionCursorClampedAfterCollapse(t *testing.T) {
	items := []collections.Item{
		{Name: "a/one"}, {Name: "a/two"}, {Name: "b/three"},
	}
	p := newCollectionPane(items, "~/x")
	p.focused = true

	p.cursor = len(p.rows()) - 1 // bottom-most row
	p.expanded["a"] = false      // collapse a folder above the cursor
	p.clampCursor()

	if p.cursor >= len(p.rows()) {
		t.Fatalf("cursor %d out of range after collapse (rows=%d)", p.cursor, len(p.rows()))
	}
	if _, ok := p.selected(); !ok && len(p.rows()) > 0 {
		// selected() may point at a folder row; only assert it doesn't panic /
		// return an out-of-range row. The clamp above is the real guarantee.
		_ = ok
	}
}
