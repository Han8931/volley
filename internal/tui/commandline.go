package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/model"
)

// openCommandLine activates the bottom input for a ":" command or "/" search.
func (m Model) openCommandLine(kind rune) Model {
	return m.openCommandLineWith(kind, "")
}

func (m Model) openCommandLineWith(kind rune, value string) Model {
	m.cmdActive = true
	m.cmdKind = kind
	m.cmd.SetValue(value)
	m.cmd.CursorEnd()
	m.cmd.Focus()
	return m
}

func (m Model) closeCommandLine() Model {
	m.cmdActive = false
	m.cmd.Blur()
	return m
}

// updateCommandLine routes keys while the command line is open.
func (m Model) updateCommandLine(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m.closeCommandLine(), nil
	case tea.KeyEnter:
		input := m.cmd.Value()
		kind := m.cmdKind
		m = m.closeCommandLine()
		if kind == ':' {
			return m.executeCommand(input)
		}
		return m.runSearch(input), nil
	}
	var cmd tea.Cmd
	m.cmd, cmd = m.cmd.Update(msg)
	return m, cmd
}

// executeCommand interprets a ":" ex-style command.
func (m Model) executeCommand(input string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m, nil
	}
	switch fields[0] {
	case "q", "quit":
		return m, tea.Quit
	case "w", "write", "save":
		if len(fields) < 2 {
			m.statusMsg = "usage: :save name"
			return m, nil
		}
		return m.saveCurrentRequest(fields[1]), nil
	case "e", "edit", "open":
		if len(fields) < 2 {
			m.statusMsg = "usage: :open name"
			return m, nil
		}
		return m.loadSavedRequest(fields[1]), nil
	case "delete", "del", "rm":
		if len(fields) < 2 {
			m.statusMsg = "usage: :delete name"
			return m, nil
		}
		if err := m.collectionStore.Delete(fields[1]); err != nil {
			m.statusMsg = "delete failed: " + err.Error()
		} else {
			m.statusMsg = "deleted " + fields[1]
			m.refreshCollections()
		}
	case "rename", "move", "mv":
		if len(fields) < 3 {
			m.statusMsg = "usage: :rename old new"
			return m, nil
		}
		if err := m.collectionStore.Rename(fields[1], fields[2]); err != nil {
			m.statusMsg = "rename failed: " + err.Error()
		} else {
			m.statusMsg = "renamed " + fields[1] + " → " + fields[2]
			m.refreshCollections()
		}
	case "copy", "cp":
		if len(fields) < 3 {
			m.statusMsg = "usage: :copy old new"
			return m, nil
		}
		if err := m.collectionStore.Copy(fields[1], fields[2]); err != nil {
			m.statusMsg = "copy failed: " + err.Error()
		} else {
			m.statusMsg = "copied " + fields[1] + " → " + fields[2]
			m.refreshCollections()
		}
	case "ls", "list":
		m.refreshCollections()
		m = m.setFocus(focusCollection)
	case "method", "m":
		if len(fields) > 1 {
			want := strings.ToUpper(fields[1])
			for i, meth := range model.Methods {
				if meth == want {
					m.req.Method, m.methodIdx = meth, i
					return m, nil
				}
			}
			m.statusMsg = "unknown method: " + fields[1]
		}
	case "set":
		m.setVariable(input)
	case "timeout":
		if len(fields) > 1 {
			d, err := time.ParseDuration(fields[1])
			if err != nil {
				m.statusMsg = "bad duration: " + fields[1]
			} else {
				m.timeout = d
				m.statusMsg = "timeout set to " + d.String()
			}
		}
	case "help", "h":
		m.showHelp = true
	default:
		m.statusMsg = "unknown command: " + fields[0]
	}
	return m, nil
}

func (m Model) saveCurrentRequest(name string) Model {
	req := m.req
	req.URL = m.url.Value()
	req.Headers = m.reqPane.headersOut()
	req.Query = m.reqPane.queryOut()
	req.Body = m.reqPane.bodyOut()
	req.Timeout = m.timeout
	if err := m.collectionStore.Save(name, req); err != nil {
		m.statusMsg = "save failed: " + err.Error()
		return m
	}
	m.currentName = name
	m.statusMsg = "saved " + name
	m.refreshCollections()
	return m
}

func (m Model) loadSavedRequest(name string) Model {
	req, err := m.collectionStore.Load(name)
	if err != nil {
		m.statusMsg = "open failed: " + err.Error()
		return m
	}
	m.req = req
	m.url.SetValue(req.URL)
	m.timeout = req.Timeout
	m.methodIdx = 0
	for i, meth := range model.Methods {
		if meth == req.Method {
			m.methodIdx = i
			break
		}
	}
	m.reqPane.setRequest(req)
	m.currentName = name
	m.statusMsg = "opened " + name
	return m
}

func (m *Model) refreshCollections() {
	items, err := m.collectionStore.List()
	if err != nil {
		m.statusMsg = "list failed: " + err.Error()
		return
	}
	m.collectionPane.SetItems(items)
}

// setVariable handles ":set name=value" (value may contain spaces).
func (m *Model) setVariable(input string) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "set"))
	name, value, ok := strings.Cut(rest, "=")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		m.statusMsg = "usage: :set name=value"
		return
	}
	m.vars.Set(name, strings.TrimSpace(value))
	m.statusMsg = "set " + name
}

// resetSearch clears search state and restores the active tab's plain text.
func (m *Model) resetSearch() {
	m.searchQuery = ""
	m.searchHits = nil
	m.searchIdx = 0
	if m.hasResp {
		m.vp.SetContent(m.currentResponseText())
	}
}

// runSearch highlights query matches in the response and jumps to the first.
func (m Model) runSearch(query string) Model {
	if query == "" {
		m.resetSearch()
		return m
	}
	hits, content := highlightMatches(m.currentResponseText(), query)
	m.searchQuery = query
	m.searchHits = hits
	m.searchIdx = 0
	m.vp.SetContent(content)
	if len(hits) == 0 {
		m.statusMsg = "pattern not found: " + query
		return m
	}
	m.vp.SetYOffset(hits[0])
	m.statusMsg = fmt.Sprintf("match 1/%d", len(hits))
	return m
}

// jumpMatch moves to the next (dir=+1) or previous (dir=-1) match line.
func (m Model) jumpMatch(dir int) Model {
	n := len(m.searchHits)
	if n == 0 {
		if m.searchQuery != "" {
			m.statusMsg = "pattern not found: " + m.searchQuery
		}
		return m
	}
	m.searchIdx = (m.searchIdx + dir + n) % n
	m.vp.SetYOffset(m.searchHits[m.searchIdx])
	m.statusMsg = fmt.Sprintf("match %d/%d", m.searchIdx+1, n)
	return m
}

// yankResponse copies the raw response body to the system clipboard.
func (m Model) yankResponse() (tea.Model, tea.Cmd) {
	if !m.hasResp {
		return m, nil
	}
	data := string(m.resp.Body)
	if err := clipboard.WriteAll(data); err != nil {
		m.statusMsg = "clipboard unavailable"
	} else {
		m.statusMsg = fmt.Sprintf("yanked %d bytes to clipboard", len(data))
	}
	return m, nil
}

var searchHighlight = lipgloss.NewStyle().
	Background(lipgloss.Color("#F59E0B")).Foreground(lipgloss.Color("#000000"))

// highlightMatches returns the line offsets containing a case-insensitive
// match and a copy of text with every match wrapped in the highlight style.
func highlightMatches(text, query string) ([]int, string) {
	lines := strings.Split(text, "\n")
	ql := strings.ToLower(query)
	var hits []int
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), ql) {
			hits = append(hits, i)
			lines[i] = highlightLine(line, query)
		}
	}
	return hits, strings.Join(lines, "\n")
}

func highlightLine(line, query string) string {
	var b strings.Builder
	ll, ql := strings.ToLower(line), strings.ToLower(query)
	i := 0
	for {
		rel := strings.Index(ll[i:], ql)
		if rel < 0 {
			b.WriteString(line[i:])
			break
		}
		start := i + rel
		b.WriteString(line[i:start])
		b.WriteString(searchHighlight.Render(line[start : start+len(query)]))
		i = start + len(query)
	}
	return b.String()
}
