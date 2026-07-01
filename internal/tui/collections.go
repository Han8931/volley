package tui

import (
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/collections"
)

type treeRow struct {
	label string
	name  string
	depth int
	file  bool
	open  bool
}

type collectionPane struct {
	items    []collections.Item
	cursor   int
	focused  bool
	width    int
	root     string // display label for the root directory
	pendD    bool
	pendG    bool
	expanded map[string]bool
}

func newCollectionPane(items []collections.Item, root string) collectionPane {
	p := collectionPane{expanded: map[string]bool{"": true}, root: root}
	p.SetItems(items)
	return p
}

func (p *collectionPane) SetItems(items []collections.Item) {
	p.items = items

	// Collect every folder that currently exists (from group items and from
	// each request's ancestors), then rebuild the expanded map so stale folders
	// are pruned while surviving folders keep their collapse state. New folders
	// default to expanded.
	valid := map[string]bool{"": true}
	addAncestors := func(name string, includeSelf bool) {
		parts := strings.Split(filepath.ToSlash(name), "/")
		last := len(parts)
		if !includeSelf {
			last--
		}
		path := ""
		for i := 0; i < last; i++ {
			if path == "" {
				path = parts[i]
			} else {
				path += "/" + parts[i]
			}
			valid[path] = true
		}
	}
	for _, it := range items {
		addAncestors(it.Name, it.IsDir)
	}
	next := make(map[string]bool, len(valid))
	for path := range valid {
		if prev, ok := p.expanded[path]; ok {
			next[path] = prev
		} else {
			next[path] = true
		}
	}
	p.expanded = next
	p.clampCursor()
}

// clampCursor keeps the cursor within the current (possibly collapsed) rows.
func (p *collectionPane) clampCursor() {
	n := len(p.rows())
	switch {
	case n == 0:
		p.cursor = 0
	case p.cursor >= n:
		p.cursor = n - 1
	case p.cursor < 0:
		p.cursor = 0
	}
}

// current returns the row under the cursor.
func (p *collectionPane) current() (treeRow, bool) {
	rows := p.rows()
	if len(rows) == 0 || p.cursor < 0 || p.cursor >= len(rows) {
		return treeRow{}, false
	}
	return rows[p.cursor], true
}

// selected returns the saved request under the cursor, if the cursor is on a file.
func (p *collectionPane) selected() (collections.Item, bool) {
	row, ok := p.current()
	if !ok || !row.file {
		return collections.Item{}, false
	}
	for _, it := range p.items {
		if it.Name == row.name {
			return it, true
		}
	}
	return collections.Item{}, false
}

func (p *collectionPane) updateNormal(msg tea.KeyMsg) (action string) {
	pendD := p.pendD
	p.pendD = false
	pendG := p.pendG
	p.pendG = false
	rows := p.rows()

	if pendG {
		if msg.String() == "g" {
			p.cursor = 0
		}
		return ""
	}

	switch msg.String() {
	case "g":
		p.pendG = true
	case "G":
		if len(rows) > 0 {
			p.cursor = len(rows) - 1
		}
	case "P": // jump to the top (root)
		p.cursor = 0
	case "j":
		if p.cursor < len(rows)-1 {
			p.cursor++
		}
	case "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter", "l", "o":
		if row, ok := p.current(); ok && !row.file {
			p.expanded[row.name] = !p.expanded[row.name]
			p.clampCursor()
			return ""
		}
		return "open"
	case "O": // open the folder and all descendants recursively
		if row, ok := p.current(); ok && !row.file {
			p.setExpandedRecursive(row.name, true)
		}
	case "X": // collapse the folder and all descendants recursively
		if row, ok := p.current(); ok && !row.file {
			p.setExpandedRecursive(row.name, false)
			p.clampCursor()
		}
	case "h": // collapse an open folder, else jump to the parent folder
		if row, ok := p.current(); ok && !row.file && row.open {
			p.expanded[row.name] = false
			p.clampCursor()
		} else {
			p.jumpToParent()
		}
	case "p": // jump to the parent folder
		p.jumpToParent()
	case "x": // close the parent of the current node and land on it
		if row, ok := p.current(); ok {
			if parent := parentPath(row.name); parent != "" {
				p.expanded[parent] = false
				p.moveToFolder(parent)
				p.clampCursor()
			}
		}
	case "R": // reload from disk
		return "refresh"
	case "d":
		if pendD {
			return "delete"
		}
		p.pendD = true
	}
	return ""
}

// setExpandedRecursive opens/closes a folder and all of its descendants.
func (p *collectionPane) setExpandedRecursive(path string, open bool) {
	for k := range p.expanded {
		if k == path || strings.HasPrefix(k, path+"/") {
			p.expanded[k] = open
		}
	}
}

// jumpToParent moves the cursor to the parent folder of the current node.
func (p *collectionPane) jumpToParent() {
	row, ok := p.current()
	if !ok {
		return
	}
	if parent := parentPath(row.name); parent != "" {
		p.moveToFolder(parent)
	} else {
		p.cursor = 0
	}
}

// moveToFolder places the cursor on the folder row with the given path.
func (p *collectionPane) moveToFolder(path string) {
	for i, r := range p.rows() {
		if !r.file && r.name == path {
			p.cursor = i
			return
		}
	}
}

// parentPath returns the folder containing name ("" for a top-level entry).
func parentPath(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[:i]
	}
	return ""
}

func (p collectionPane) view() string {
	rootLabel := p.root
	if rootLabel == "" {
		rootLabel = "collections"
	}
	header := []string{
		title("COLLECTIONS"),
		dim("▾ " + rootLabel),
	}
	if len(p.items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, append(header,
			"",
			dim("empty — ")+keyHint("m a")+dim(" add request · ")+keyHint("m g")+dim(" new group"),
		)...)
	}

	lines := header
	for i, row := range p.rows() {
		text := strings.Repeat("  ", row.depth) + row.label
		st := lipgloss.NewStyle()
		if p.width > 0 {
			st = st.MaxWidth(p.width)
		}
		switch {
		case p.focused && i == p.cursor:
			st = st.Foreground(lipgloss.Color("#FFFFFF")).Background(colSel)
		case i == p.cursor:
			st = st.Foreground(colAccent)
		case row.file:
			st = st.Foreground(colFg)
		default:
			st = st.Foreground(colAccent) // groups stand out
		}
		lines = append(lines, st.Render(text))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p collectionPane) rows() []treeRow {
	type node struct {
		dirs  map[string]*node
		files []string
		path  string
	}
	root := &node{dirs: map[string]*node{}}

	ensureDir := func(parts []string) *node {
		cur := root
		path := ""
		for _, part := range parts {
			if path == "" {
				path = part
			} else {
				path += "/" + part
			}
			if cur.dirs[part] == nil {
				cur.dirs[part] = &node{dirs: map[string]*node{}, path: path}
			}
			cur = cur.dirs[part]
		}
		return cur
	}

	for _, it := range p.items {
		parts := strings.Split(filepath.ToSlash(it.Name), "/")
		if it.IsDir {
			ensureDir(parts)
			continue
		}
		parent := ensureDir(parts[:len(parts)-1])
		parent.files = append(parent.files, it.Name)
	}

	var out []treeRow
	var walk func(n *node, depth int)
	walk = func(n *node, depth int) {
		dirs := make([]string, 0, len(n.dirs))
		for name := range n.dirs {
			dirs = append(dirs, name)
		}
		sort.Strings(dirs)
		for _, name := range dirs {
			child := n.dirs[name]
			open := p.expanded[child.path]
			icon := "▸"
			if open {
				icon = "▾"
			}
			out = append(out, treeRow{label: icon + " " + name + "/", name: child.path, depth: depth, open: open})
			if open {
				walk(child, depth+1)
			}
		}
		sort.Strings(n.files)
		for _, f := range n.files {
			parts := strings.Split(filepath.ToSlash(f), "/")
			out = append(out, treeRow{label: "  " + parts[len(parts)-1], name: f, depth: depth, file: true})
		}
	}
	walk(root, 0)
	return out
}
