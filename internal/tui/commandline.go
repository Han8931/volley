package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
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
	if kind == ':' {
		m.cmdHistoryIdx = len(m.cmdHistory)
		m.cmdDraft = value
	}
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
	case v == "loadtest " || v == "lt " || v == "loadedit " || v == "ltedit " ||
		v == "loadeditor " || v == "lteditor ":
		return "<profile>"
	case v == "loadnew " || v == "ltnew ":
		return "<name> [template]"
	}
	return ""
}

func (m Model) closeCommandLine() Model {
	m.cmdActive = false
	m.cmdHint = ""
	m.cmd.Blur()
	return m
}

// updateCommandLine routes keys while the command line is open.
func (m Model) updateCommandLine(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m.closeCommandLine(), nil
	case tea.KeyUp:
		if m.cmdKind == ':' {
			return m.previousCommand(), nil
		}
	case tea.KeyDown:
		if m.cmdKind == ':' {
			return m.nextCommand(), nil
		}
	case tea.KeyTab:
		if m.cmdKind == ':' {
			return m.completeCommand(), nil
		}
	case tea.KeyEnter:
		input := m.cmd.Value()
		kind := m.cmdKind
		if kind == ':' {
			m = m.rememberCommand(input)
		}
		m = m.closeCommandLine()
		if kind == ':' {
			return m.executeCommand(input)
		}
		return m.runSearch(input), nil
	}
	m.cmdHint = "" // stale completion feedback drops on any other key
	var cmd tea.Cmd
	m.cmd, cmd = m.cmd.Update(msg)
	return m, cmd
}

const commandHistoryLimit = 100

// rememberCommand adds a non-empty command to the in-memory history. Adjacent
// duplicates are collapsed, matching common shell history behavior.
func (m Model) rememberCommand(input string) Model {
	input = strings.TrimSpace(input)
	if input == "" {
		return m
	}
	if len(m.cmdHistory) == 0 || m.cmdHistory[len(m.cmdHistory)-1] != input {
		m.cmdHistory = append(m.cmdHistory, input)
		if len(m.cmdHistory) > commandHistoryLimit {
			m.cmdHistory = append([]string(nil), m.cmdHistory[len(m.cmdHistory)-commandHistoryLimit:]...)
		}
	}
	m.cmdHistoryIdx = len(m.cmdHistory)
	return m
}

func (m Model) previousCommand() Model {
	if len(m.cmdHistory) == 0 {
		return m
	}
	if m.cmdHistoryIdx >= len(m.cmdHistory) {
		m.cmdDraft = m.cmd.Value()
		m.cmdHistoryIdx = len(m.cmdHistory)
	}
	if m.cmdHistoryIdx > 0 {
		m.cmdHistoryIdx--
	}
	m.cmd.SetValue(m.cmdHistory[m.cmdHistoryIdx])
	m.cmd.CursorEnd()
	return m
}

func (m Model) nextCommand() Model {
	if m.cmdHistoryIdx >= len(m.cmdHistory) {
		return m
	}
	m.cmdHistoryIdx++
	if m.cmdHistoryIdx == len(m.cmdHistory) {
		m.cmd.SetValue(m.cmdDraft)
	} else {
		m.cmd.SetValue(m.cmdHistory[m.cmdHistoryIdx])
	}
	m.cmd.CursorEnd()
	return m
}

// commandVerbs are the ":" commands offered by Tab completion — the canonical
// spelling of each command. Aliases (:e, :lt, :tabc, …) still execute; they
// just aren't offered, to keep ambiguous listings readable.
var commandVerbs = []string{
	"copy", "delete", "editor", "help", "import", "loadedit", "loadeditor",
	"loadnew", "loadtest", "ls", "method", "mkgroup", "new", "open", "quit",
	"rename", "rengroup", "rmgroup", "save", "send", "set", "tabclose",
	"tabnew", "tabnext", "tabonly", "tabprevious", "timeout", "wq",
}

// completeCommand implements Tab completion for the ":" command line: the
// command verb while the first word is being typed, then per-command argument
// values (saved requests, groups, load profiles, methods).
func (m Model) completeCommand() Model {
	value := m.cmd.Value()
	if strings.HasPrefix(value, " ") {
		return m
	}
	fields := strings.Fields(value)
	trailingSpace := strings.HasSuffix(value, " ")
	if len(fields) == 0 || (len(fields) == 1 && !trailingSpace) {
		prefix := ""
		if len(fields) == 1 {
			prefix = fields[0]
		}
		// A unique verb gets a trailing space so Tab can continue on its argument.
		return m.completeToken("", prefix, commandVerbs, "command", " ")
	}
	verb, args := fields[0], fields[1:]
	argIdx := len(args)
	prefix := ""
	if !trailingSpace {
		argIdx--
		prefix = args[argIdx]
	}
	candidates, what, errMsg := m.argCandidates(verb, argIdx)
	if errMsg != "" {
		m.cmdHint = errMsg
		return m
	}
	if candidates == nil {
		return m
	}
	head := strings.Join(append([]string{verb}, args[:argIdx]...), " ") + " "
	return m.completeToken(head, prefix, candidates, what, "")
}

// completeToken replaces the token after head with the completion of prefix
// against candidates. A unique match is inserted (followed by uniqueSuffix);
// multiple matches extend to their shared prefix and are listed in the status
// line.
func (m Model) completeToken(head, prefix string, candidates []string, what, uniqueSuffix string) Model {
	m.cmdHint = ""
	matches := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			matches = append(matches, c)
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		m.cmdHint = "no " + what + " matching " + prefix
		return m
	}
	// A match identical to what's already typed adds nothing — drop it so a
	// completed group prefix ("auth/") can continue into its children.
	if len(matches) > 1 {
		kept := matches[:0]
		for _, c := range matches {
			if c != prefix {
				kept = append(kept, c)
			}
		}
		matches = kept
	}
	completed := matches[0] + uniqueSuffix
	if len(matches) > 1 {
		completed = commonPrefix(matches)
		m.cmdHint = what + "s: " + strings.Join(matches, " · ")
	}
	m.cmd.SetValue(head + completed)
	m.cmd.CursorEnd()
	return m
}

// argCandidates returns the completion candidates for the argIdx'th argument
// of verb, or nil when that position takes free text. errMsg is set when a
// candidate source fails and should be surfaced instead of completing.
func (m Model) argCandidates(verb string, argIdx int) (candidates []string, what string, errMsg string) {
	switch verb {
	case "open", "e", "edit", "delete", "del", "rm", "editor", "tabnew", "tabe", "tabedit",
		"new", "enew", "w", "write", "save", "rename", "move", "mv":
		if argIdx == 0 {
			return m.savedNameCandidates()
		}
	case "copy", "cp":
		if argIdx == 0 {
			names, what, errMsg := m.savedNameCandidates()
			return append(names, "curl"), what, errMsg
		}
	case "mkgroup", "group", "mkg", "rmgroup", "rmg", "rengroup", "reng":
		if argIdx == 0 {
			return m.groupNameCandidates()
		}
	case "method", "m":
		if argIdx == 0 {
			return model.Methods, "method", ""
		}
	case "loadtest", "lt", "loadedit", "ltedit", "loadeditor", "lteditor":
		if argIdx == 0 {
			return m.profileNameCandidates()
		}
	case "loadnew", "ltnew":
		if argIdx == 1 { // the optional template profile
			return m.profileNameCandidates()
		}
	case "import":
		if argIdx == 0 {
			return []string{"curl"}, "import source", ""
		}
	}
	return nil, "", ""
}

// savedNameCandidates lists saved request names, plus group names with a
// trailing slash so completion can descend into them.
func (m Model) savedNameCandidates() ([]string, string, string) {
	items, err := m.collectionStore.List()
	if err != nil {
		return nil, "", "collections unavailable: " + err.Error()
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		if it.IsDir {
			names = append(names, it.Name+"/")
		} else {
			names = append(names, it.Name)
		}
	}
	return names, "saved request", ""
}

func (m Model) groupNameCandidates() ([]string, string, string) {
	items, err := m.collectionStore.List()
	if err != nil {
		return nil, "", "collections unavailable: " + err.Error()
	}
	var names []string
	for _, it := range items {
		if it.IsDir {
			names = append(names, it.Name)
		}
	}
	return names, "group", ""
}

func (m Model) profileNameCandidates() ([]string, string, string) {
	if err := m.loadStore.EnsureDefaults(); err != nil {
		return nil, "", "load profiles unavailable: " + err.Error()
	}
	profiles, err := m.loadStore.List()
	if err != nil {
		return nil, "", "load profiles unavailable: " + err.Error()
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names, "load profile", ""
}

func commonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := []rune(values[0])
	for _, value := range values[1:] {
		runes := []rune(value)
		n := len(prefix)
		if len(runes) < n {
			n = len(runes)
		}
		i := 0
		for i < n && prefix[i] == runes[i] {
			i++
		}
		prefix = prefix[:i]
	}
	return string(prefix)
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
	case "loadnew", "ltnew":
		if len(fields) < 2 {
			m.statusMsg = "usage: :loadnew <name> [template profile]"
			return m, nil
		}
		template := ""
		if len(fields) > 2 {
			template = fields[2]
		}
		return m.newLoadProfile(fields[1], template)
	case "loadedit", "ltedit":
		if len(fields) < 2 {
			m.statusMsg = "usage: :loadedit <profile>"
			return m, nil
		}
		return m.editLoadProfileByName(fields[1])
	case "loadeditor", "lteditor":
		if len(fields) < 2 {
			m.statusMsg = "usage: :loadeditor <profile>"
			return m, nil
		}
		return m.editLoadProfileJSONByName(fields[1])
	case "loadtest", "lt":
		if len(fields) < 2 {
			return m.openLoadPicker()
		}
		name := strings.Join(fields[1:], " ")
		if err := m.loadStore.EnsureDefaults(); err != nil {
			m.statusMsg = "load profiles unavailable: " + err.Error()
			return m, nil
		}
		p, err := m.loadStore.Load(name)
		if err != nil {
			m.statusMsg = "no load profile named " + name
			return m, nil
		}
		if m.loadRunning() {
			m.statusMsg = "load test already running — esc to stop it first"
			return m, nil
		}
		m.loadRun = nil
		return m.confirmLoadTest(p), nil
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
	m.baseline = req      // current edits are now the on-disk state
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

// resolveEditor returns the user's editor command ($VISUAL, then $EDITOR).
func resolveEditor() string {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	return editor
}

func (m Model) openExternalEditor(name string) (tea.Model, tea.Cmd) {
	editor := resolveEditor()
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
