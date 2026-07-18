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
	label  string
	name   string
	method string // HTTP method for file rows (shown as a colored badge)
	depth  int
	file   bool
	open   bool
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
	marked   map[string]bool // multi-selected request names
}

func newCollectionPane(items []collections.Item, root string) collectionPane {
	p := collectionPane{expanded: map[string]bool{"": true}, marked: map[string]bool{}, root: root}
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

	// Keep marks only for saved requests that still exist. Directories are not
	// selectable, so they never enter the marked set.
	validFiles := make(map[string]bool)
	for _, it := range items {
		if !it.IsDir {
			validFiles[it.Name] = true
		}
	}
	if p.marked == nil {
		p.marked = map[string]bool{}
	} else {
		for name := range p.marked {
			if !validFiles[name] {
				delete(p.marked, name)
			}
		}
	}
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
	case "A": // NerdTree-style zoom: widen/narrow the tree pane
		return "toggle-wide"
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
	case " ":
		if row, ok := p.current(); ok && row.file {
			if p.marked[row.name] {
				delete(p.marked, row.name)
			} else {
				p.marked[row.name] = true
			}
		}
		if p.cursor < len(rows)-1 {
			p.cursor++
		}
	case "T":
		return "open-tabs"
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

// markedRequests returns marked request names in visible tree order. If nothing
// is marked, it falls back to the request under the cursor.
func (p collectionPane) markedRequests() []string {
	var names []string
	if len(p.marked) > 0 {
		for _, row := range p.rows() {
			if row.file && p.marked[row.name] {
				names = append(names, row.name)
			}
		}
		return names
	}
	if row, ok := p.current(); ok && row.file {
		return []string{row.name}
	}
	return nil
}

// setExpandedRecursive opens/closes a folder and all of its descendants.
func (p *collectionPane) setExpandedRecursive(path string, open bool) {
	for k := range p.expanded {
		if path == "" || k == path || strings.HasPrefix(k, path+"/") {
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
	return p.viewWithTitle(title("COLLECTIONS"))
}

func (p collectionPane) viewWithTitle(headerTitle string) string {
	rootLabel := p.root
	if rootLabel == "" {
		rootLabel = "collections"
	}
	// Keep the root line to a single row (like the tree rows below) so a long
	// path doesn't wrap — that would both look messy and desync mouse hit-testing,
	// which assumes a fixed-height header. Truncate physically (the pane's word
	// wrap would otherwise break a long path across several rows). p.width is
	// already the pane's padded content width (see applyLayout).
	rootLine := "root: " + rootLabel
	if p.width > 0 {
		rootLine = truncateRunes(rootLine, p.width)
	}
	header := []string{
		headerTitle,
		dim(rootLine),
	}

	lines := header
	for i, row := range p.rows() {
		indent := strings.Repeat("  ", row.depth)
		selected := p.focused && i == p.cursor
		onCursor := i == p.cursor

		// File rows show a colored HTTP-method badge before the name (Bruno-style,
		// e.g. "POST api_test_1"). Marked rows get a compact light block over the
		// request text itself — no checkbox glyph and no full-width fill, so long
		// selections don't visually swamp the tree or affect layout.
		if row.file && row.method != "" {
			marked := p.marked[row.name]
			nameSt := lipgloss.NewStyle()
			methodSt := lipgloss.NewStyle().Foreground(methodColor(row.method)).Bold(true)
			switch {
			case selected:
				nameSt = nameSt.Foreground(colSelFg).Background(colSel)
				methodSt = methodSt.Background(colSel)
			case marked:
				nameSt = nameSt.Foreground(colSelFg).Background(colMarked)
				methodSt = methodSt.Background(colMarked)
			case onCursor:
				nameSt = nameSt.Foreground(colAccent)
			default:
				nameSt = nameSt.Foreground(colFg)
			}
			line := indent + methodSt.Render(row.method) + nameSt.Render(" "+row.label)
			if p.width > 0 {
				line = lipgloss.NewStyle().MaxWidth(p.width).Render(line)
			}
			lines = append(lines, line)
			continue
		}

		text := indent + row.label
		marked := row.file && p.marked[row.name]
		st := lipgloss.NewStyle()
		if p.width > 0 {
			st = st.MaxWidth(p.width)
		}
		switch {
		case selected:
			st = st.Foreground(colSelFg).Background(colSel)
		case marked:
			st = st.Foreground(colSelFg).Background(colMarked)
		case onCursor:
			st = st.Foreground(colAccent)
		case row.file:
			st = st.Foreground(colFg)
		default:
			st = st.Foreground(colAccent) // groups stand out
		}
		lines = append(lines, st.Render(text))
	}
	if len(p.items) == 0 {
		lines = append(lines,
			"",
			dim("empty — with root selected, ")+keyHint("m g")+dim(" creates a top-level group"),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// methodColor maps an HTTP method to its badge color, reusing the JSON
// highlighter's palette so the UI reads as one system.
func methodColor(method string) lipgloss.TerminalColor {
	switch method {
	case "GET":
		return lipgloss.AdaptiveColor{Light: "#059669", Dark: "#34D399"} // green
	case "POST":
		return lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"} // amber
	case "PUT":
		return lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#60A5FA"} // blue
	case "PATCH":
		return lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#C084FC"} // purple
	case "DELETE":
		return lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"} // red
	default:
		return lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#93C5FD"} // HEAD / OPTIONS / other
	}
}

func (p collectionPane) rows() []treeRow {
	type node struct {
		dirs  map[string]*node
		files []collections.Item
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
		parent.files = append(parent.files, it)
	}

	rootOpen := p.expanded[""]
	rootIcon := "▸"
	if rootOpen {
		rootIcon = "▾"
	}
	out := []treeRow{{label: rootIcon + " /", name: "", depth: 0, open: rootOpen}}
	if !rootOpen {
		return out
	}

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
		sort.Slice(n.files, func(i, j int) bool { return n.files[i].Name < n.files[j].Name })
		for _, f := range n.files {
			parts := strings.Split(filepath.ToSlash(f.Name), "/")
			out = append(out, treeRow{
				label:  parts[len(parts)-1],
				name:   f.Name,
				method: f.Method,
				depth:  depth,
				file:   true,
			})
		}
	}
	walk(root, 1)
	return out
}
