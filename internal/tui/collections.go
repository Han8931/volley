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
	pendD    bool
	pendG    bool
	expanded map[string]bool
}

func newCollectionPane(items []collections.Item) collectionPane {
	p := collectionPane{expanded: map[string]bool{"": true}}
	p.SetItems(items)
	return p
}

func (p *collectionPane) SetItems(items []collections.Item) {
	p.items = items
	if p.expanded == nil {
		p.expanded = map[string]bool{"": true}
	}
	for _, it := range items {
		parts := strings.Split(filepath.ToSlash(it.Name), "/")
		path := ""
		for i := 0; i < len(parts)-1; i++ {
			if path == "" {
				path = parts[i]
			} else {
				path += "/" + parts[i]
			}
			if _, ok := p.expanded[path]; !ok {
				p.expanded[path] = true
			}
		}
	}
	rows := p.rows()
	if p.cursor >= len(rows) && p.cursor > 0 {
		p.cursor = len(rows) - 1
	}
	if len(rows) == 0 {
		p.cursor = 0
	}
}

func (p *collectionPane) selected() (collections.Item, bool) {
	rows := p.rows()
	if len(rows) == 0 || p.cursor < 0 || p.cursor >= len(rows) || !rows[p.cursor].file {
		return collections.Item{}, false
	}
	name := rows[p.cursor].name
	for _, it := range p.items {
		if it.Name == name {
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
			return ""
		}
	}

	switch msg.String() {
	case "g":
		p.pendG = true
	case "G":
		if len(rows) > 0 {
			p.cursor = len(rows) - 1
		}
	case "j":
		if pendD {
			return "delete"
		}
		if p.cursor < len(rows)-1 {
			p.cursor++
		}
	case "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter", "l", "o":
		if len(rows) > 0 && p.cursor < len(rows) && !rows[p.cursor].file {
			p.expanded[rows[p.cursor].name] = !p.expanded[rows[p.cursor].name]
			return ""
		}
		return "open"
	case "h":
		if len(rows) > 0 && p.cursor < len(rows) && !rows[p.cursor].file && rows[p.cursor].open {
			p.expanded[rows[p.cursor].name] = false
		}
	case "d":
		if pendD {
			return "delete"
		}
		p.pendD = true
	}
	return ""
}

func (p collectionPane) view() string {
	if len(p.items) == 0 {
		return title("COLLECTIONS") + "\n\n" +
			dim("No saved requests yet.\nUse ") + keyHint(":save name") +
			dim(" or ") + keyHint("m a") + dim(" to add one.")
	}
	lines := []string{title("COLLECTIONS"), dim("▾ ~/.config/volley/collections")}
	rows := p.rows()
	for i, row := range rows {
		text := strings.Repeat("  ", row.depth) + row.label
		st := lipgloss.NewStyle().MaxWidth(p.width)
		if p.focused && i == p.cursor {
			st = st.Foreground(lipgloss.Color("#FFFFFF")).Background(colSel)
		} else if i == p.cursor {
			st = st.Foreground(colAccent)
		} else if row.file {
			st = st.Foreground(colFg)
		} else {
			st = st.Foreground(colDim)
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
	for _, it := range p.items {
		parts := strings.Split(filepath.ToSlash(it.Name), "/")
		cur := root
		path := ""
		for _, part := range parts[:len(parts)-1] {
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
		cur.files = append(cur.files, it.Name)
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
			out = append(out, treeRow{label: " " + parts[len(parts)-1], name: f, depth: depth, file: true})
		}
	}
	walk(root, 0)
	return out
}
