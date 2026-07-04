package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/curl"
	"github.com/tabularasa/volley/internal/model"
)

// openCommandLine activates the bottom input for a ":" command or "/" search.
func (m Model) openCommandLine(kind rune) Model {
	return m.openCommandLineWith(kind, "")
}

func (m Model) openCommandLineWith(kind rune, value string) Model {
	m.cmdActive = true
	m.cmdKind = kind
	if kind == '/' {
		m.cmd.Placeholder = "search response…"
	} else {
		m.cmd.Placeholder = "e.g. save APISet1/getUsers · mkgroup APISet2 · method POST"
	}
	m.cmd.SetValue(value)
	m.cmd.CursorEnd()
	m.cmd.Focus()
	return m
}

// commandGhost returns a dim template shown after the cursor to guide the user
// while typing a ":" command — e.g. "<name>" after ":save APISet1/". It is
// purely advisory (not inserted); it appears only when the cursor is at the end
// of the input and a value is still expected.
func (m Model) commandGhost() string {
	if m.cmdKind != ':' {
		return ""
	}
	v := m.cmd.Value()
	if v == "" || m.cmd.Position() != len([]rune(v)) {
		return ""
	}
	switch {
	case strings.HasSuffix(v, "/"):
		return "<name>"
	case v == "save " || v == "w " || v == "write " || v == "new ":
		return "<group>/<name>"
	case v == "open " || v == "e " || v == "edit " || v == "delete " || v == "del " || v == "rm ":
		return "<group>/<name>"
	case v == "mkgroup " || v == "group " || v == "mkg " || v == "rmgroup " || v == "rmg ":
		return "<group>"
	}
	return ""
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
	// Volley edits one request at a time, so the Vim "all" variants (:qa, :wqa,
	// :xa) are accepted as aliases of their single-buffer forms — muscle memory
	// shouldn't error out.
	switch fields[0] {
	case "q", "quit", "qa", "qall", "quitall":
		return m.guardedQuit()
	case "q!", "quit!", "qa!", "qall!", "quitall!":
		return m, tea.Quit // force-quit, discarding unsaved edits
	case "wq", "x", "wqa", "wqall", "xa", "xall":
		if m.currentName == "" {
			m.statusMsg = "no name yet — use :w <name> first"
			return m, nil
		}
		m, ok := m.saveCurrentRequest(m.currentName)
		if !ok {
			return m, nil // save failed — stay open so edits aren't lost
		}
		return m, tea.Quit
	case "send":
		return m.send()
	case "new", "enew":
		if len(fields) > 1 {
			return m.guardedNewSaved(fields[1])
		}
		return m.guardedNewBlank()
	case "w", "write", "save":
		name := ""
		if len(fields) > 1 {
			name = fields[1]
		}
		m, _ := m.saveCurrentRequest(name)
		return m, nil
	case "e", "edit", "open":
		if len(fields) < 2 {
			m.statusMsg = "usage: :open name"
			return m, nil
		}
		return m.guardedOpen(fields[1])
	case "delete", "del", "rm":
		if len(fields) < 2 {
			m.statusMsg = "usage: :delete name"
			return m, nil
		}
		m.deleteSaved(fields[1])
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
	case "import":
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), fields[0]))
		parts := strings.Fields(rest)
		if len(parts) == 0 || parts[0] != "curl" {
			m.statusMsg = "usage: :import curl <command>"
			return m, nil
		}
		return m.guardedImportCurl(rest)
	case "copy", "cp":
		if len(fields) == 2 && fields[1] == "curl" {
			return m.copyAsCurl()
		}
		if len(fields) < 3 {
			m.statusMsg = "usage: :copy old new  (or :copy curl to copy the request as curl)"
			return m, nil
		}
		if err := m.collectionStore.Copy(fields[1], fields[2]); err != nil {
			m.statusMsg = "copy failed: " + err.Error()
		} else {
			m.statusMsg = "copied " + fields[1] + " → " + fields[2]
			m.refreshCollections()
		}
	case "mkgroup", "group", "mkg":
		if len(fields) < 2 {
			m.statusMsg = "usage: :mkgroup name"
			return m, nil
		}
		if err := m.collectionStore.CreateGroup(fields[1]); err != nil {
			m.statusMsg = "create group failed: " + err.Error()
		} else {
			m.statusMsg = "created group " + fields[1]
			m.refreshCollections()
			m.collectionPref = true
			m = m.applyCollectionVisibility()
			m = m.setFocus(focusCollection)
		}
	case "rmgroup", "rmg":
		if len(fields) < 2 {
			m.statusMsg = "usage: :rmgroup name"
			return m, nil
		}
		m.deleteGroup(fields[1])
	case "rengroup", "reng":
		if len(fields) < 3 {
			m.statusMsg = "usage: :rengroup old new"
			return m, nil
		}
		if err := m.collectionStore.RenameGroup(fields[1], fields[2]); err != nil {
			m.statusMsg = "rename group failed: " + err.Error()
		} else {
			m.statusMsg = "renamed group " + fields[1] + " → " + fields[2]
			m.refreshCollections()
		}
	case "ls", "list":
		m.refreshCollections()
		m.collectionPref = true // ensure the tree is visible (width permitting) before focusing it
		m = m.applyCollectionVisibility()
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
				m.timeoutInput.SetValue(d.String())
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

func (m Model) newBlankRequest() Model {
	m = m.applyRequest(model.NewRequest())
	m.currentName = ""
	m.statusMsg = "new request"
	return m
}

func (m Model) newSavedRequest(name string) Model {
	req := model.NewRequest()
	if err := m.collectionStore.Save(name, req); err != nil {
		// Don't alter the current editor or claim the name — no file was written.
		m.statusMsg = "create request failed: " + err.Error()
		return m
	}
	m = m.applyRequest(req)
	m.currentName = name
	m.statusMsg = "created " + name + " — edit URL, then :save"
	m.refreshCollections()
	return m
}

// saveCurrentRequest persists the current edits under name. The bool reports
// whether the save actually succeeded, so callers that would discard, replace,
// or quit afterwards can abort and keep the user's work when the write fails.
func (m Model) saveCurrentRequest(name string) (Model, bool) {
	if name == "" {
		name = m.currentName
	}
	if name == "" {
		m.statusMsg = "usage: :save name"
		return m, false
	}
	req := m.rawRequest()
	if err := m.collectionStore.Save(name, req); err != nil {
		m.statusMsg = "save failed: " + err.Error()
		return m, false
	}
	m.currentName = name
	m.baseline = req // current edits are now the on-disk state
	m.statusMsg = "saved " + name
	m.refreshCollections()
	return m, true
}

// guardedImportCurl parses cmd and loads it into the editor, first validating
// so parse errors surface immediately, and popping the unsaved-changes prompt
// when the current buffer has edits (import replaces it).
func (m Model) guardedImportCurl(cmd string) (tea.Model, tea.Cmd) {
	if _, _, err := curl.Parse(cmd); err != nil {
		m.statusMsg = "import failed: " + err.Error()
		return m, nil
	}
	if m.dirty() {
		return m.armSavePrompt(pendingImportCurl, cmd), nil
	}
	return m.applyCurlImport(cmd), nil
}

// applyCurlImport loads a parsed curl command into a fresh, unnamed buffer.
func (m Model) applyCurlImport(cmd string) Model {
	req, warns, err := curl.Parse(cmd)
	if err != nil {
		m.statusMsg = "import failed: " + err.Error()
		return m
	}
	m = m.applyRequest(req)
	m.currentName = "" // the imported request isn't tied to a saved file yet
	// An imported, unnamed request contains user data that is not on disk. Keep it
	// dirty so quit/open/new prompts protect it until the user saves or discards.
	m.baseline = model.NewRequest()
	msg := "imported curl request"
	if len(warns) > 0 {
		msg += " — " + strings.Join(warns, "; ")
	}
	m.statusMsg = msg
	return m
}

// copyAsCurl copies the current request (variables expanded, query folded into
// the URL) to the system clipboard as a runnable curl command.
func (m Model) copyAsCurl() (tea.Model, tea.Cmd) {
	if err := clipboard.WriteAll(curl.Format(m.buildRequest())); err != nil {
		m.statusMsg = "clipboard unavailable"
	} else {
		m.statusMsg = "copied request as curl to clipboard"
	}
	return m, nil
}

// applyRequest loads a Request into the editor panes (URL, method, tabs).
func (m Model) applyRequest(req model.Request) Model {
	m.req = req
	m.url.SetText(req.URL)
	m.timeout = req.Timeout
	m.timeoutInput.SetValue(formatTimeout(req.Timeout))
	m.methodIdx = 0
	for i, meth := range model.Methods {
		if meth == req.Method {
			m.methodIdx = i
			break
		}
	}
	m.reqPane.setRequest(req)
	// Capture the freshly-loaded state as the clean baseline for dirty checks.
	m.baseline = m.rawRequest()
	return m
}

func (m Model) commitTimeoutInput() Model {
	m.timeoutInput.Blur()
	v := strings.TrimSpace(m.timeoutInput.Value())
	if v == "" {
		m.timeout = 0
		m.statusMsg = "timeout reset to default"
		return m
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		m.timeoutInput.SetValue(formatTimeout(m.timeout))
		m.statusMsg = "bad timeout: use values like 500ms, 10s, 2m"
		return m
	}
	m.timeout = d
	m.timeoutInput.SetValue(d.String())
	m.statusMsg = "timeout set to " + d.String()
	return m
}

func formatTimeout(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}

func (m Model) loadSavedRequest(name string) Model {
	req, err := m.collectionStore.Load(name)
	if err != nil {
		m.statusMsg = "open failed: " + err.Error()
		return m
	}
	m = m.applyRequest(req)
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
