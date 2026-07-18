package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
		// Quitting discards every open buffer, so the write covers them all:
		// the active editor plus any dirty background tab.
		if m.dirty() && m.currentName == "" {
			m.statusMsg = "no name yet — use :w <name> first"
			return m, nil
		}
		m, ok := m.saveAllDirty()
		if !ok {
			return m, nil // a save failed — stay open so edits aren't lost
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
			m = m.renameOpenBuffers(fields[1], fields[2])
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
	case "editor":
		name := ""
		if len(fields) > 1 {
			name = strings.Join(fields[1:], " ")
		}
		return m.openExternalEditor(name)
	case "tabnew", "tabe", "tabedit":
		if len(fields) < 2 {
			m.statusMsg = "usage: :tabnew <saved request>"
			return m, nil
		}
		return m.openTabByName(strings.Join(fields[1:], " "))
	case "tabclose", "tabc":
		return m.closeActiveTab()
	case "tabonly", "tabo":
		return m.closeOtherTabs()
	case "tabnext", "tabn":
		return m.switchOpenTab(1)
	case "tabprevious", "tabprev", "tabp", "tabN":
		return m.switchOpenTab(-1)
	case "help", "h":
		m.showHelp = true
		m.helpScroll = 0
	default:
		m.statusMsg = "unknown command: " + fields[0]
	}
	return m, nil
}

func (m Model) newBlankRequest() Model {
	m = m.applyRequest(model.NewRequest())
	m.currentName = ""
	m = m.syncActiveTab() // the active tab, if any, becomes the new scratch buffer
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
	m = m.syncActiveTab() // repurpose the active tab, if any, for the new request
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
	m = m.syncActiveTab() // :save <newname> repoints the active tab too
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
	m = m.syncActiveTab() // the active tab, if any, now holds the import
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

type editorFinishedMsg struct {
	path       string
	targetName string // saved request to write/load; empty means edit current buffer only
	err        error
}

func (m Model) openExternalEditor(name string) (tea.Model, tea.Cmd) {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		m.statusMsg = "set $VISUAL or $EDITOR to use :editor"
		return m, nil
	}
	req := m.rawRequest()
	if name != "" && name != m.currentName {
		if m.dirty() {
			m.statusMsg = "save or discard current edits before editing another request"
			return m, nil
		}
		loaded, err := m.collectionStore.Load(name)
		if err != nil {
			m.statusMsg = "no saved request named " + name
			return m, nil
		}
		req = loaded
	}
	f, err := os.CreateTemp("", "volley-request-*.txt")
	if err != nil {
		m.statusMsg = "editor failed: " + err.Error()
		return m, nil
	}
	path := f.Name()
	initial, err := editorInitialContent(req)
	if err != nil {
		f.Close()
		os.Remove(path)
		m.statusMsg = "editor failed: " + err.Error()
		return m, nil
	}
	if _, err := f.WriteString(initial); err != nil {
		f.Close()
		os.Remove(path)
		m.statusMsg = "editor failed: " + err.Error()
		return m, nil
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		m.statusMsg = "editor failed: " + err.Error()
		return m, nil
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{path: path, targetName: name, err: err}
	})
}

// editableRequest is the metadata block of the :editor document — everything
// except the body. It mirrors the on-disk storage shape (lowercase keys, an
// omitted-when-empty auth block) so the two hand-editable forms read
// consistently, and — like the storage DTO — keeps format concerns out of the
// domain model.Request. It lives only in a temp file for one round-trip, so it
// has no persisted-schema compatibility to honor. json.Unmarshal matches these
// tags case-insensitively, so a stray "Name"/"URL" from the user still parses.
//
// The body is deliberately NOT a field here: embedding it as a JSON string
// flattens a multi-line payload into one escaped line, which is miserable to
// edit — the exact thing :editor exists to make easy. Instead the document is a
// JSON metadata block, the editorBodyMarker line, then the raw body verbatim.
type editableRequest struct {
	Method  string           `json:"method"`
	URL     string           `json:"url"`
	Headers []editableHeader `json:"headers,omitempty"`
	Query   []editableKV     `json:"query,omitempty"`
	Auth    *editableAuth    `json:"auth,omitempty"`
	Timeout string           `json:"timeout,omitempty"`
}

// editorBodyMarker separates the JSON metadata block from the raw request body
// in the :editor document. Everything on the lines after it is the body, sent
// verbatim. splitEditorContent matches it by prefix so trimming the trailing
// hint text doesn't break parsing.
const editorBodyMarker = "===== request body (keep this line; text below is sent verbatim) ====="

const editorBodyMarkerPrefix = "===== request body"

type editableHeader struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type editableKV struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type editableAuth struct {
	Type     string `json:"type,omitempty"`
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Key      string `json:"key,omitempty"`
	Value    string `json:"value,omitempty"`
	InQuery  bool   `json:"inQuery,omitempty"`
}

func editorInitialContent(req model.Request) (string, error) {
	b, err := json.MarshalIndent(editableFromRequest(req), "", "  ")
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.Write(b)
	sb.WriteString("\n\n")
	sb.WriteString(editorBodyMarker)
	sb.WriteString("\n")
	sb.WriteString(req.Body)
	sb.WriteString("\n")
	return sb.String(), nil
}

// splitEditorContent divides an edited document into its JSON metadata block and
// the raw body below the marker. A missing marker (the user deleted it) is
// treated leniently as metadata-only with an empty body rather than an error.
func splitEditorContent(s string) (meta, body string) {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), editorBodyMarkerPrefix) {
			meta = strings.Join(lines[:i], "\n")
			// Drop the single trailing newline editorInitialContent appends, so a
			// round-trip with no edits preserves the body byte-for-byte.
			body = strings.TrimSuffix(strings.Join(lines[i+1:], "\n"), "\n")
			return meta, body
		}
	}
	return s, ""
}

func editableFromRequest(req model.Request) editableRequest {
	out := editableRequest{
		Method: req.Method,
		URL:    req.URL,
	}
	for _, h := range req.Headers {
		out.Headers = append(out.Headers, editableHeader{Name: h.Name, Value: h.Value, Enabled: h.Enabled})
	}
	for _, q := range req.Query {
		out.Query = append(out.Query, editableKV{Key: q.Key, Value: q.Value, Enabled: q.Enabled})
	}
	if req.Auth.Type != model.AuthNone {
		out.Auth = &editableAuth{
			Type:     req.Auth.Type,
			Token:    req.Auth.Token,
			Username: req.Auth.Username,
			Password: req.Auth.Password,
			Key:      req.Auth.Key,
			Value:    req.Auth.Value,
			InQuery:  req.Auth.InQuery,
		}
	}
	if req.Timeout > 0 {
		out.Timeout = req.Timeout.String()
	}
	return out
}

func parseEditedRequest(b []byte) (model.Request, error) {
	meta, body := splitEditorContent(string(b))
	var in editableRequest
	if err := json.Unmarshal([]byte(meta), &in); err != nil {
		return model.Request{}, err
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = "GET"
	}
	if !validMethod(method) {
		return model.Request{}, fmt.Errorf("unknown method %q", in.Method)
	}
	req := model.Request{
		Method: method,
		URL:    strings.TrimSpace(in.URL),
		Body:   body,
	}
	for _, h := range in.Headers {
		req.Headers = append(req.Headers, model.Header{Name: h.Name, Value: h.Value, Enabled: h.Enabled})
	}
	for _, q := range in.Query {
		req.Query = append(req.Query, model.KV{Key: q.Key, Value: q.Value, Enabled: q.Enabled})
	}
	if in.Auth != nil {
		req.Auth = model.Auth{
			Type:     in.Auth.Type,
			Token:    in.Auth.Token,
			Username: in.Auth.Username,
			Password: in.Auth.Password,
			Key:      in.Auth.Key,
			Value:    in.Auth.Value,
			InQuery:  in.Auth.InQuery,
		}
	}
	if strings.TrimSpace(in.Timeout) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(in.Timeout))
		if err != nil || d < 0 {
			return model.Request{}, fmt.Errorf("bad timeout %q", in.Timeout)
		}
		req.Timeout = d
	}
	return req, nil
}

func validMethod(method string) bool {
	for _, m := range model.Methods {
		if method == m {
			return true
		}
	}
	return false
}

func (m Model) applyEditorResult(msg editorFinishedMsg) (tea.Model, tea.Cmd) {
	defer os.Remove(msg.path)
	if msg.err != nil {
		m.statusMsg = "editor failed: " + msg.err.Error()
		return m, nil
	}
	b, err := os.ReadFile(msg.path)
	if err != nil {
		m.statusMsg = "editor failed: " + err.Error()
		return m, nil
	}
	req, err := parseEditedRequest(b)
	if err != nil {
		m.statusMsg = "editor parse failed: " + err.Error()
		return m, nil
	}
	if msg.targetName != "" {
		if err := m.collectionStore.Save(msg.targetName, req); err != nil {
			m.statusMsg = "editor save failed: " + err.Error()
			return m, nil
		}
		m.refreshCollections()
		loaded := m.loadSavedRequest(msg.targetName)
		if loaded.currentName != msg.targetName {
			return loaded, nil // saved, but the reload failed — surface its error, don't mask it
		}
		loaded.statusMsg = "updated " + msg.targetName + " from editor"
		return loaded, nil
	}
	baseline := m.baseline
	m = m.applyRequest(req)
	m.baseline = baseline // editing through $EDITOR must still count as unsaved changes
	m.statusMsg = "updated request from editor"
	return m, nil
}

func (m Model) loadSavedRequest(name string) Model {
	req, err := m.collectionStore.Load(name)
	if err != nil {
		m.statusMsg = "open failed: " + err.Error()
		return m
	}
	m = m.applyRequest(req)
	m.currentName = name
	// Opening into the active tab repurposes it: keep its slot (name, buffer,
	// baseline) in step so the tabline shows the new request immediately.
	m = m.syncActiveTab()
	m.statusMsg = "opened " + name
	return m
}

// openCollectionTabs opens the marked tree requests (or the current request when
// nothing is marked) as tabs. Newly-selected requests are appended to the open
// set — deduped — so tabs build up across presses rather than replacing each
// other, and opening never blocks on unsaved edits (the editor just loads the
// tab's on-disk request).
func (m Model) openCollectionTabs() (tea.Model, tea.Cmd) {
	names := m.collectionPane.markedRequests()
	if len(names) == 0 {
		m.statusMsg = "mark requests with space, or place cursor on a request, then press T"
		return m, nil
	}

	m = m.syncActiveTab() // preserve the current tab's edits before adding/switching

	open := make(map[string]bool, len(m.tabs))
	for _, t := range m.tabs {
		open[t.name] = true
	}
	tabs := cloneTabs(m.tabs)
	added, firstNew := 0, -1
	var failed []string // selections whose files couldn't be loaded (e.g. deleted externally)
	for _, n := range names {
		if open[n] {
			continue
		}
		req, err := m.collectionStore.Load(n)
		if err != nil {
			failed = append(failed, n)
			continue
		}
		open[n] = true
		if firstNew < 0 {
			firstNew = len(tabs)
		}
		tabs = append(tabs, newTabFromSaved(n, req))
		added++
	}
	m.tabs = tabs
	if len(m.tabs) == 0 {
		// Every selection failed to load and nothing was open to fall back to.
		m.statusMsg = "open failed: " + strings.Join(failed, ", ")
		return m, nil
	}

	// Focus the first newly-opened tab; if every selection was already open, jump
	// to the first one named so T still takes you there.
	target := firstNew
	if target < 0 {
		target = indexOfName(m.tabs, names[0])
	}
	m.activeTab = target
	m = m.loadActiveTab()
	switch {
	case added == 0:
		m.statusMsg = "switched to " + m.tabs[target].name
	case added == 1:
		m.statusMsg = "opened tab " + m.tabs[target].name
	default:
		m.statusMsg = fmt.Sprintf("opened %d tabs", added)
	}
	if len(failed) > 0 {
		m.statusMsg += " · failed to open: " + strings.Join(failed, ", ")
	}
	return m, nil
}

// indexOfName returns the position of the tab named name, or 0 when absent.
func indexOfName(tabs []tabBuf, name string) int {
	for i, t := range tabs {
		if t.name == name {
			return i
		}
	}
	return 0
}

// openTabByName opens a saved request as a tab (switching to it if already
// open) and loads it into the editor — the :tabnew entry point.
func (m Model) openTabByName(name string) (tea.Model, tea.Cmd) {
	for i, t := range m.tabs {
		if t.name == name {
			return m.switchOpenTabTo(i)
		}
	}
	req, err := m.collectionStore.Load(name)
	if err != nil {
		m.statusMsg = "no saved request named " + name
		return m, nil
	}
	m = m.syncActiveTab()
	m.tabs = append(cloneTabs(m.tabs), newTabFromSaved(name, req))
	m.activeTab = len(m.tabs) - 1
	m = m.loadActiveTab()
	m.statusMsg = "opened tab " + name
	return m, nil
}

// closeActiveTab drops the current tab and loads its neighbour.
func (m Model) closeActiveTab() (tea.Model, tea.Cmd) {
	if len(m.tabs) == 0 {
		m.statusMsg = "no open tabs"
		return m, nil
	}
	return m.closeTabAt(m.activeTab)
}

// closeTabAt removes tab idx. Because each tab now holds its own in-memory edits,
// closing any tab with unsaved changes — active or background — asks for
// confirmation first; a clean tab closes immediately.
func (m Model) closeTabAt(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.tabs) {
		return m, nil
	}
	if m.tabDirty(idx) {
		m.confirmCloseTab = true
		m.closeTabIdx = idx
		m.statusMsg = "close tab with unsaved changes? (y/n)"
		return m, nil
	}
	return m.closeTabAtDiscarding(idx), nil
}

func (m Model) closeTabAtDiscarding(idx int) Model {
	if idx < 0 || idx >= len(m.tabs) {
		return m
	}
	closingActive := idx == m.activeTab
	tabs := cloneTabs(m.tabs)
	tabs = append(tabs[:idx], tabs[idx+1:]...)
	m.tabs = tabs
	if len(m.tabs) == 0 {
		m.activeTab = 0
		m.statusMsg = "closed tab"
		return m
	}
	switch {
	case idx < m.activeTab:
		m.activeTab-- // the active tab shifted left; its live editor stays as-is
	case closingActive:
		if m.activeTab >= len(m.tabs) {
			m.activeTab = len(m.tabs) - 1
		}
		m = m.loadActiveTab() // restore the neighbour's buffer
	}
	m.statusMsg = "closed tab"
	return m
}

func (m Model) resolveTabCloseConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	idx := m.closeTabIdx
	m.confirmCloseTab = false
	m.closeTabIdx = 0
	if msg.String() != "y" {
		m.statusMsg = "tab close cancelled"
		return m, nil
	}
	return m.closeTabAtDiscarding(idx), nil
}

// closeOtherTabs keeps only the active tab (Vim's :tabonly).
func (m Model) closeOtherTabs() (tea.Model, tea.Cmd) {
	if len(m.tabs) <= 1 {
		m.statusMsg = "no other tabs"
		return m, nil
	}
	m = m.syncActiveTab() // capture the surviving tab's live edits before collapsing
	m.tabs = []tabBuf{m.tabs[m.activeTab]}
	m.activeTab = 0
	m.statusMsg = "closed other tabs"
	return m, nil
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
		m.vp.SetContent(m.currentResponseViewText())
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
