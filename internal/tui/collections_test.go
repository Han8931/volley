package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/model"
)

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
		{"APISet1", false, 0},
		{"APISet1/auth", false, 1},
		{"APISet1/auth/login", true, 2},
		{"APISet1/getUsers", true, 1},
		{"APISet2", false, 0}, // empty group still shown
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

	// Collapse "g" recursively -> only the top group row remains.
	p.setExpandedRecursive("g", false)
	if n := len(p.rows()); n != 1 {
		t.Fatalf("after recursive collapse rows = %d, want 1", n)
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
