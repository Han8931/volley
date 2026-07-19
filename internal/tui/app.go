// Package tui implements Volley's terminal UI on top of Bubble Tea.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/httpx"
	"github.com/tabularasa/volley/internal/loadtest"
	"github.com/tabularasa/volley/internal/model"
	"github.com/tabularasa/volley/internal/vars"
	"github.com/tabularasa/volley/internal/vimtext"
)

// focus identifies which pane currently receives navigation/edit keys.
type focus int

const (
	focusURL focus = iota
	focusMethod
	focusCollection
	focusRequest
	focusResponse
)

// tabOrder is the sequence tab / shift+tab walk, following the screen's natural
// reading order: the collections sidebar, the method + URL on the top row, then
// the request pane, then the response pane. Timeout is edited inline (t or
// :timeout), not a tab stop.
var tabOrder = []focus{
	focusCollection,
	focusMethod,
	focusURL,
	focusRequest,
	focusResponse,
}

// pendingKind identifies a transition deferred behind an unsaved-changes prompt.
type pendingKind int

const (
	pendingNone pendingKind = iota
	pendingOpenRequest
	pendingNewBlank
	pendingNewNamed
	pendingQuit
	pendingImportCurl
)

// Model is the root Bubble Tea model. Its fields are grouped into embedded
// concern-structs (editorState, responseState, …) rather than one flat wall of
// state. The groups are anonymous, so Go promotes their fields: the rest of the
// package still reads m.hasResp / m.url with no qualifier, while the type reads
// as a handful of documented concerns instead of ~45 mixed fields. Modal state
// is derived from whether a child is editing.
type Model struct {
	width, height int
	focus         focus
	statusMsg     string // ephemeral feedback shown in the status bar

	vars vars.Store

	editorState
	treeState
	responseState
	selectionState
	searchState
	cmdlineState
	promptState
	chordState
	inputState
	loadState
	envState
}

// editorState is the request currently being edited in the top bar + request
// pane, plus the baseline used to detect unsaved changes.
type editorState struct {
	req          model.Request
	url          vimtext.Buffer // URL editor: a single-line modal (Normal/Insert) vim buffer
	timeoutInput textinput.Model
	methodIdx    int
	timeout      time.Duration
	reqPane      requestPane
	currentName  string        // saved request backing the editor, "" for an unsaved buffer
	baseline     model.Request // last saved/loaded request, compared against to detect edits
}

// treeState is the collections sidebar and the row of open request tabs.
type treeState struct {
	collectionStore collections.Store
	collectionPane  collectionPane
	collectionShown bool     // effective tree visibility: the preference, gated by width
	collectionPref  bool     // user's show/hide preference, restored when the window is wide enough
	collectionWide  bool     // NerdTree-style zoom: widen the tree so long request names are visible
	tabs            []tabBuf // open request tabs, each a live in-memory editor buffer
	activeTab       int
}

// responseState is the response viewer and the lifecycle of the in-flight send.
type responseState struct {
	vp          viewport.Model
	spin        spinner.Model
	sending     bool
	cancel      context.CancelFunc // aborts the in-flight request, if any
	sendSeq     int                // identifies the in-flight send; stale/cancelled responses are dropped
	resp        model.Response
	hasResp     bool
	respTab     int    // 0 = body, 1 = headers
	respText    string // rendered body, kept for search
	respHeaders string // rendered headers
	rawBody     bool   // sticky view preference: show the body verbatim instead of pretty-printing JSON (mode is shown in respTabBar)
	pendingG    bool   // for the "gg" motion in the response viewport
}

// selectionState is drag-to-select in the response viewport. selecting is true
// between a left press and release over the response body; selDragged
// distinguishes a drag (selection) from a plain click. Positions index the text.
type selectionState struct {
	selecting  bool
	selDragged bool
	selLoad    bool // selection is over the load-run view, not the response body
	selAnchor  textPos
	selCursor  textPos
}

// searchState is the "/" incremental search over the response body.
type searchState struct {
	searchQuery string
	searchHits  []int // line offsets containing a match
	searchIdx   int
}

// cmdlineState is the ":" command / "/" search input line.
type cmdlineState struct {
	cmd           textinput.Model
	cmdActive     bool
	cmdKind       rune // ':' or '/'
	cmdHistory    []string
	cmdHistoryIdx int
	cmdDraft      string
	cmdHint       string // Tab-completion feedback, shown inline while typing
}

// promptState holds the transient overlays and confirmations that gate input:
// the help screen, the tree menu, delete/close confirmations, and the
// unsaved-changes guard.
type promptState struct {
	showHelp        bool
	collectionMenu  bool   // NERDTree-like "m" filesystem menu is awaiting a key
	confirmDelete   string // name of a request/group awaiting y/n delete confirmation
	confirmGroup    bool   // whether confirmDelete refers to a group
	confirmCloseTab bool   // active tab has unsaved edits and awaits y/n close confirmation
	closeTabIdx     int    // tab index to close if confirmCloseTab is accepted
	// pendingAction is a discarding transition (open/new/quit) held behind an
	// unsaved-changes prompt; pendingArg carries its payload (e.g. a name).
	pendingAction pendingKind
	pendingArg    string
	helpScroll    int // first visible content row of the help overlay
}

// chordState tracks multi-key sequences awaiting their next keystroke.
type chordState struct {
	pendingWindow bool // for the "ctrl+w <hjkl>" window-navigation chord
	pendingComma  bool // for leader-style commands: ",n" tree toggle, ",g" focus hints
	focusHints    bool // awaiting a numeric pane target after ",g"
}

// inputState tracks low-level pointer state: the wheel-key suppressor and the
// double-click detector for the collections tree.
type inputState struct {
	// Swallows terminal-injected vertical navigation keys that can accompany
	// wheel mouse events, keeping other panes still while the response scrolls.
	wheel wheelSuppressor
	// Double-click tracking for the collections tree: a second click on the same
	// row within doubleClickWindow opens (a file) or toggles (a folder).
	lastTreeClickRow int
	lastTreeClick    time.Time
}

// homeShorten replaces the user's home directory prefix with "~".
func homeShorten(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// urlPlaceholder is shown in the URL bar when it is empty and unfocused.
const urlPlaceholder = "https://api.example.com/v1/ping"

// New builds the root model with a blank request ready to edit.
func New() Model {
	// The URL bar is a single-line vim buffer. It starts in Normal mode so the app
	// opens in a Vim-like command/navigation state; press i/a (or click the URL) to
	// enter text. It is held by value so each Model copy owns its own URL state.
	urlBuf := vimtext.New("", true)
	urlBuf.SetMode(vimtext.Normal)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	vp := viewport.New(0, 0)
	vp.KeyMap = vimViewportKeys()

	timeoutInput := textinput.New()
	timeoutInput.Placeholder = httpx.DefaultTimeout.String()
	timeoutInput.Prompt = ""
	// Kept in sync with timeoutValueW / timeoutReserve so the inline editor and
	// readout fit the URL bar's reserved budget.
	timeoutInput.CharLimit = timeoutValueW
	timeoutInput.Width = timeoutValueW

	cmd := textinput.New()
	cmd.Prompt = ""

	store := collections.DefaultStore()
	items, listErr := store.List()

	m := Model{
		vars: vars.New(),
		editorState: editorState{
			req:          model.NewRequest(),
			baseline:     model.NewRequest(),
			url:          *urlBuf,
			timeoutInput: timeoutInput,
			reqPane:      newRequestPane(),
		},
		treeState: treeState{
			collectionStore: store,
			collectionPane:  newCollectionPane(items, homeShorten(store.Root)),
			collectionShown: true,
			collectionPref:  true,
		},
		responseState: responseState{
			spin: sp,
			vp:   vp,
		},
		cmdlineState: cmdlineState{cmd: cmd},
		loadState: loadState{
			loadStore:   loadtest.DefaultStore(),
			resultStore: loadtest.DefaultResultStore(),
		},
		envState: envState{envStore: vars.DefaultEnvStore()},
	}
	m = m.setFocus(focusCollection)
	if listErr != nil {
		m.statusMsg = "failed to load collections: " + listErr.Error()
	}
	return m
}

// vimViewportKeys maps the response scroll viewport onto Vim motions.
func vimViewportKeys() viewport.KeyMap {
	return viewport.KeyMap{
		Up:           key.NewBinding(key.WithKeys("k")),
		Down:         key.NewBinding(key.WithKeys("j")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		PageUp:       key.NewBinding(key.WithKeys("ctrl+b")),
		PageDown:     key.NewBinding(key.WithKeys("ctrl+f")),
	}
}

// urlInsert reports whether the URL bar is focused and in text-entry (Insert)
// mode — the vim-buffer equivalent of the old textinput's Focused() state.
func (m Model) urlInsert() bool {
	return m.focus == focusURL && m.url.Mode() == vimtext.Insert
}

// editing reports whether a child field is capturing keys in Insert mode. This
// gates whether keys route to text entry (routeEditing) vs pane navigation. The
// URL bar's Vim NORMAL sub-mode is intentionally NOT "editing": its motion and
// operator keys (x/w/b/C/p/u…) flow through updateNormal → updateURLNormal.
func (m Model) editing() bool {
	if m.urlInsert() || m.timeoutInput.Focused() {
		return true
	}
	return m.focus == focusRequest && m.reqPane.editing()
}

// inInsert reports whether the active field is actually inserting text, for
// the INSERT/NORMAL status tag (a field can be captured but in Vim-normal).
func (m Model) inInsert() bool {
	if m.urlInsert() || m.timeoutInput.Focused() {
		return true
	}
	return m.focus == focusRequest && m.reqPane.inInsert()
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	// The alternate-scroll reset (DECSET ?1007) is emitted from the renderer via
	// safeModel.View, so it lands in the alternate-screen buffer every frame
	// rather than racing terminal setup here.
	return textinput.Blink
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.applyCollectionVisibility()
		m = m.applyLayout(m.computeLayout())
		m.rerenderResponse()
		return m, nil

	case tea.MouseMsg:
		switch {
		case msg.Action == tea.MouseActionPress &&
			(msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown):
			return m.handleScroll(msg)
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			return m.handleMouseDown(msg)
		case msg.Action == tea.MouseActionMotion && m.selecting:
			return m.handleMouseDrag(msg)
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease:
			return m.handleMouseUp(msg)
		}
		return m, nil

	case tea.KeyMsg:
		// Terminals can batch fast keystrokes (key repeat, laggy SSH) into one
		// KeyRunes message; every handler matches single keys, so split the batch
		// and process the runes in order. Bracketed paste stays intact for the
		// active text field.
		if msg.Type == tea.KeyRunes && !msg.Paste && len(msg.Runes) > 1 {
			return m.updateRuneBatch(msg.Runes)
		}
		if m.shouldSuppressWheelArrow(msg) {
			return m, nil
		}
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// While a request is in flight, esc aborts it rather than doing its
		// usual mode-exit, so a slow or hung send is never a dead end. The
		// same escape hatch stops a running load test.
		if m.sending && msg.Type == tea.KeyEsc {
			return m.cancelSend()
		}
		if m.loadRunning() && msg.Type == tea.KeyEsc {
			return m.stopLoadTest()
		}
		if m.showHelp {
			return m.updateHelp(msg), nil
		}
		if m.confirmDelete != "" {
			return m.resolveDeleteConfirm(msg), nil
		}
		if m.confirmCloseTab {
			return m.resolveTabCloseConfirm(msg)
		}
		if m.loadConfirm {
			return m.resolveLoadConfirm(msg)
		}
		if m.loadPicker {
			return m.updateLoadPicker(msg)
		}
		if m.shapeEdit {
			return m.updateShapeEditor(msg)
		}
		if m.pendingAction != pendingNone {
			return m.resolveSaveConfirm(msg)
		}
		if m.focusHints {
			return m.resolveFocusHint(msg), nil
		}
		if m.cmdActive {
			return m.updateCommandLine(msg)
		}
		if m.collectionMenu {
			return m.updateCollectionMenu(msg)
		}
		if m.editing() {
			return m.routeEditing(msg)
		}
		return m.updateNormal(msg)

	case editorFinishedMsg:
		return m.applyEditorResult(msg)

	case profileEditorFinishedMsg:
		return m.applyProfileEditorResult(msg)

	case envEditorFinishedMsg:
		return m.applyEnvEditorResult(msg)

	case loadTickMsg:
		return m.handleLoadTick(msg)

	case responseMsg:
		if msg.seq != m.sendSeq {
			return m, nil // a cancelled or superseded request's late result — ignore
		}
		m.sending = false
		m.cancel = nil
		m.resp = msg.resp
		m.hasResp = true
		m.respTab = 0
		m.respText = renderResponseBody(msg.resp, m.vp.Width, m.rawBody)
		m.respHeaders = renderResponseHeaders(msg.resp)
		m.searchQuery, m.searchHits, m.searchIdx = "", nil, 0
		m.vp.SetContent(m.currentResponseViewText())
		m.vp.GotoTop()
		return m, nil

	case spinner.TickMsg:
		if !m.sending {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}

	// Non-key messages (e.g. text-input cursor blink) go to whichever bubbles
	// textinput is active. The URL bar is a vimtext buffer and needs none.
	var cmd tea.Cmd
	if m.cmdActive {
		m.cmd, cmd = m.cmd.Update(msg)
	} else if m.timeoutInput.Focused() {
		m.timeoutInput, cmd = m.timeoutInput.Update(msg)
	}
	return m, cmd
}

// updateRuneBatch replays a batched multi-rune key message as individual
// keystrokes, so fast input never falls through the single-key handlers.
func (m Model) updateRuneBatch(runes []rune) (tea.Model, tea.Cmd) {
	var (
		cur  tea.Model = m
		cmds []tea.Cmd
	)
	for _, r := range runes {
		var cmd tea.Cmd
		cur, cmd = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cur, tea.Batch(cmds...)
}

// updateHelp handles keys while the help overlay is open: Vim scroll motions
// move through it, anything else dismisses it.
func (m Model) updateHelp(msg tea.KeyMsg) Model {
	page := m.helpPageRows()
	switch msg.String() {
	case "j", "down":
		m.helpScroll++
	case "k", "up":
		m.helpScroll--
	case "ctrl+d":
		m.helpScroll += page / 2
	case "ctrl+u":
		m.helpScroll -= page / 2
	case "g", "gg":
		m.helpScroll = 0
	case "G":
		m.helpScroll = m.helpMaxScroll()
	default:
		m.showHelp = false
		m.helpScroll = 0
		return m
	}
	m.helpScroll = clampInt(m.helpScroll, 0, m.helpMaxScroll())
	return m
}

// rerenderResponse re-renders the response text for the current viewport width
// (the body is soft-wrapped to it), preserving any active search highlight.
func (m *Model) rerenderResponse() {
	if !m.hasResp {
		return
	}
	m.respText = renderResponseBody(m.resp, m.vp.Width, m.rawBody)
	if m.searchQuery != "" {
		hits, content := highlightMatches(m.currentResponseText(), m.searchQuery)
		m.searchHits, m.searchIdx = hits, 0
		m.vp.SetContent(content)
		return
	}
	m.vp.SetContent(m.currentResponseViewText())
}

// routeEditing forwards keys to the active text field.
func (m Model) routeEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = "" // clear transient feedback on the next keystroke, like NORMAL
	if m.urlInsert() {
		// The URL bar types like a text field while inserting. A few structural
		// keys are intercepted so you can send and move panes without an esc dance;
		// everything else feeds the vim buffer (which handles esc→NORMAL itself).
		switch msg.Type {
		case tea.KeyTab:
			return m.cycleFocus(1), nil
		case tea.KeyShiftTab:
			return m.cycleFocus(-1), nil
		case tea.KeyEnter:
			return m.send() // browser/Postman muscle memory: Enter fires the request
		case tea.KeyCtrlW:
			// Begin the Vim window-nav chord; leave insert so the next key is nav.
			m.url.SetMode(vimtext.Normal)
			m.pendingWindow = true
			return m, nil
		}
		m.url.Feed(msg.String()) // includes esc → drops to NORMAL, staying focused
		m.req.URL = m.url.Text()
		return m, nil
	}
	if m.timeoutInput.Focused() {
		if msg.Type == tea.KeyEsc || msg.Type == tea.KeyEnter {
			return m.commitTimeoutInput(), nil
		}
		var cmd tea.Cmd
		m.timeoutInput, cmd = m.timeoutInput.Update(msg)
		return m, cmd
	}
	cmd := m.reqPane.updateEditing(msg)
	return m, cmd
}

// updateNormal handles NORMAL-mode keys: window navigation and per-pane verbs.
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = "" // clear transient feedback on the next keystroke

	// A finished load run's results stay up until dismissed; T reruns the same
	// profile from the response pane.
	if m.loadRun != nil && m.loadSnap.Done {
		switch {
		case msg.Type == tea.KeyEsc:
			return m.stopLoadTest() // dismisses the finished view
		case msg.String() == "T" && m.focus == focusResponse:
			return m.confirmLoadTest(m.loadProfile), nil
		case msg.String() == "y" && m.focus == focusResponse:
			return m.yankLoadSummary()
		}
	}

	// Arrow keys mirror h/j/k/l so they move *within* the focused pane (and drive
	// the ctrl+w chord), matching Vim. Pane/focus changes are ctrl+w h/j/k/l or
	// tab — arrows no longer hop panes.
	msg = arrowsAsHJKL(msg)

	// Leader-style shortcuts.
	if m.pendingComma {
		m.pendingComma = false
		switch msg.String() {
		case "n":
			return m.toggleCollectionPane(), nil
		case "t":
			return m.beginEditTimeout(), textinput.Blink
		case "g":
			m.focusHints = true
			m.statusMsg = "jump to pane: 1 tree · 2 method · 3 url · 4 request · 5 response"
			return m, nil
		}
		return m, nil
	}

	// Vim window-navigation chord: "ctrl+w" then h/j/k/l (or w to cycle).
	if m.pendingWindow {
		m.pendingWindow = false
		switch msg.String() {
		case "h":
			return m.setFocus(m.focusLeft()), nil
		case "j":
			return m.moveFocusDown(), nil
		case "k":
			return m.setFocus(m.focusUp()), nil
		case "l":
			return m.setFocus(m.focusRight()), nil
		case "w":
			return m.cycleFocus(1), nil
		case "q":
			return m.closeActiveTab() // Vim-style: ctrl+w q closes the active tab
		}
		return m, nil // anything else cancels the chord
	}

	switch msg.String() {
	case "H":
		// H/L walk the open request tabs from any pane. The request pane's own
		// Headers/Body/Params sub-tabs stay reachable via [ / ] once tabs are open.
		if len(m.tabs) > 0 {
			return m.switchOpenTab(-1)
		}
	case "L":
		if len(m.tabs) > 0 {
			return m.switchOpenTab(1)
		}
	case "?":
		m.showHelp = true
		m.helpScroll = 0
		return m, nil
	case "[", "]":
		// Likewise, bracket tab navigation from the URL bar should operate on the
		// request tabs (Response keeps its own [/] handling when focused there).
		if m.focus == focusURL {
			m = m.setFocus(focusRequest)
			if msg.String() == "]" {
				m.reqPane.selectTab(m.reqPane.tab + 1)
			} else {
				m.reqPane.selectTab(m.reqPane.tab - 1)
			}
			return m, nil
		}
	case ":":
		return m.openCommandLine(':'), nil
	case "ctrl+w":
		m.pendingWindow = true
		return m, nil
	case ",":
		m.pendingComma = true
		m.statusMsg = "leader: n toggle tree · t edit timeout · g jump pane"
		return m, nil
	case "tab":
		return m.cycleFocus(1), nil
	case "shift+tab":
		return m.cycleFocus(-1), nil
	case "q":
		return m.guardedQuit()
	}

	switch m.focus {
	case focusURL:
		return m.updateURLNormal(msg)
	case focusMethod:
		return m.updateMethodNormal(msg)
	case focusCollection:
		return m.updateCollectionNormal(msg)
	case focusRequest:
		cmd := m.reqPane.updateNormal(msg)
		return m, cmd
	case focusResponse:
		return m.updateResponseNav(msg)
	}
	return m, nil
}

// beginEditTimeout focuses the inline timeout editor in the top bar. Timeout is
// no longer a pane/tab stop: keys route to it purely while its input is focused,
// and committing (esc/enter) returns to whatever focus was active (the URL bar,
// where t is invoked). The readout lives in the URL bar; see viewURLBar.
func (m Model) beginEditTimeout() Model {
	m.timeoutInput.Focus()
	m.timeoutInput.CursorEnd()
	return m
}

func (m Model) resolveFocusHint(msg tea.KeyMsg) Model {
	m.focusHints = false
	switch msg.String() {
	case "1":
		if !m.collectionShown {
			m.statusMsg = "collections tree is hidden"
			return m
		}
		return m.setFocus(focusCollection)
	case "2":
		return m.setFocus(focusMethod)
	case "3":
		return m.setFocus(focusURL)
	case "4":
		return m.setFocus(focusRequest)
	case "5":
		return m.setFocus(focusResponse)
	default:
		m.statusMsg = "pane jump cancelled"
		return m
	}
}

// updateMethodNormal handles keys while the standalone method selector is
// focused: r (Vim-style "replace") advances the HTTP method; j/k (or ↓/↑) cycle
// either way. Pane moves are tab / ctrl+w.
func (m Model) updateMethodNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Only r/R cycle the HTTP method. j/k, space and the arrow keys are all inert
	// here so a stray keypress — or terminal wheel-arrow leakage — can't change
	// the method behind your back.
	switch msg.String() {
	case "r":
		return m.cycleMethod(1), nil
	case "R":
		return m.cycleMethod(-1), nil
	case "enter":
		return m.send()
	}
	return m, nil
}

// updateURLNormal handles keys while the URL bar is focused in Vim NORMAL mode.
// It is pure Vim: every key feeds the buffer, so the URL supports the full
// motion and edit set (x, w, b, e, C, D, dd, cw, p, u, i/a/I/A…) — except
// Enter, which sends the request. Pane moves stay on tab/ctrl+w and the
// timeout shortcut is the ,t leader — all handled upstream in updateNormal
// before reaching here.
func (m Model) updateURLNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		return m.send()
	}
	m.url.Feed(msg.String())
	m.req.URL = m.url.Text()
	return m, nil
}

func (m Model) updateCollectionNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "m" {
		m.collectionMenu = true
		m.statusMsg = "NERDTree menu: a add req · g new group · o open · r rename · c copy · d delete · q cancel"
		return m, nil
	}
	switch m.collectionPane.updateNormal(msg) {
	case "open":
		// Open as a tab, exactly like clicking the row — keyboard and mouse
		// must not diverge on the same target. (:open keeps vim's :e semantics
		// of loading into the current buffer, with its unsaved-edits guard.)
		if it, ok := m.collectionPane.selected(); ok {
			return m.openTabByName(it.Name)
		}
	case "delete":
		if row, ok := m.collectionPane.current(); ok && row.name != "" {
			m = m.askDeleteConfirm(row.name, !row.file)
		}
	case "toggle-wide":
		m.collectionWide = !m.collectionWide
		m = m.applyLayout(m.computeLayout())
		if m.collectionWide {
			m.statusMsg = "collections tree widened"
		} else {
			m.statusMsg = "collections tree restored"
		}
	case "open-tabs":
		return m.openCollectionTabs()
	case "refresh":
		m.refreshCollections()
		m.statusMsg = "reloaded collections"
	}
	return m, nil
}

// askDeleteConfirm arms a y/n confirmation before destroying a request or group.
func (m Model) askDeleteConfirm(name string, isGroup bool) Model {
	m.confirmDelete = name
	m.confirmGroup = isGroup
	if isGroup {
		m.statusMsg = "delete group " + name + " and all its requests? (y/n)"
	} else {
		m.statusMsg = "delete " + name + "? (y/n)"
	}
	return m
}

// resolveDeleteConfirm handles the key pressed while a delete is pending.
func (m Model) resolveDeleteConfirm(msg tea.KeyMsg) Model {
	name, isGroup := m.confirmDelete, m.confirmGroup
	m.confirmDelete, m.confirmGroup = "", false
	if msg.String() != "y" {
		m.statusMsg = "delete cancelled"
		return m
	}
	if isGroup {
		m.deleteGroup(name)
	} else {
		m.deleteSaved(name)
	}
	return m
}

// deleteSaved removes a saved request and refreshes the tree (shared by the
// tree, the menu, and the ":delete" command).
func (m *Model) deleteSaved(name string) {
	if err := m.collectionStore.Delete(name); err != nil {
		m.statusMsg = "delete failed: " + err.Error()
		return
	}
	m.statusMsg = "deleted " + name
	m.refreshCollections()
}

// deleteGroup removes a group and all requests under it.
func (m *Model) deleteGroup(name string) {
	if err := m.collectionStore.DeleteGroup(name); err != nil {
		m.statusMsg = "delete group failed: " + err.Error()
		return
	}
	m.statusMsg = "deleted group " + name
	m.refreshCollections()
}

func (m Model) toggleCollectionPane() Model {
	m.collectionPref = !m.collectionPref
	m = m.applyCollectionVisibility() // also moves focus off the tree if it hides
	switch {
	case m.collectionShown:
		m.statusMsg = "tree shown"
	case m.collectionPref:
		m.statusMsg = "tree hidden (terminal too narrow)"
	default:
		m.statusMsg = "tree hidden"
	}
	return m
}

// applyCollectionVisibility recomputes the effective tree visibility from the
// user's preference and the current width, auto-hiding the tree on terminals
// too narrow to show it beside the request/response panes. Focus is pulled off
// the tree when it hides so navigation can't strand on an invisible pane.
func (m Model) applyCollectionVisibility() Model {
	m.collectionShown = m.collectionPref && m.width >= collectionsMinWidth
	if !m.collectionShown && m.focus == focusCollection {
		m = m.setFocus(focusRequest)
	}
	return m
}

func (m Model) updateCollectionMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.collectionMenu = false

	// Establish the context: which node is under the cursor, and which group
	// new requests should default into.
	name, onDir, have := "", false, false
	if row, ok := m.collectionPane.current(); ok {
		name, onDir, have = row.name, !row.file, true
	}
	group := "" // the group a new sibling request/group belongs to
	if have {
		if onDir {
			group = name
		} else {
			group = parentPath(name)
		}
	}
	prefix := ""
	if group != "" {
		prefix = group + "/"
	}

	switch msg.String() {
	case "q", "esc":
		m.statusMsg = ""
	case "a": // add a request (defaults into the current group)
		m = m.openCommandLineWith(':', "new "+prefix)
	case "g": // create a new group (nested under the current group)
		m = m.openCommandLineWith(':', "mkgroup "+prefix)
	case "o":
		if have && !onDir {
			return m.openTabByName(name) // same tab-open as Enter/click on the row
		}
	case "d":
		if have && name != "" {
			m = m.askDeleteConfirm(name, onDir)
		}
	case "r", "m": // rename request or group
		if have && onDir && name != "" {
			m = m.openCommandLineWith(':', "rengroup "+name+" ")
		} else if have && !onDir {
			m = m.openCommandLineWith(':', "rename "+name+" ")
		}
	case "c":
		if have && !onDir {
			m = m.openCommandLineWith(':', "copy "+name+" ")
		}
	default:
		m.statusMsg = "" // ignore stray keys quietly
	}
	return m, nil
}

// topBarH is the height (rows) of the method/URL top bar: a content row between
// the pane's top and bottom borders.
const topBarH = 3

// tabBarH is the height of the tabline: a strip at the top of the right-hand
// column. The space is always reserved (even with no tabs open) so the layout
// never shifts when tabs appear or disappear.
func (m Model) tabBarH() int { return 1 }

// tablineTopGap is the blank row the right column reserves above the tabline
// when the tree is shown, so the tabline lines up with the tree's first content
// row (COLLECTIONS) and the tree's top border stays the clean topmost element.
func (m Model) tablineTopGap() int {
	if m.collectionShown {
		return 1
	}
	return 0
}

// tablineY / topBarY / bodyTopY are the top rows of the tabline, the method/URL
// bar, and the request/response body — all measured from the very top row.
func (m Model) tablineY() int { return m.tablineTopGap() }

func (m Model) topBarY() int { return m.tablineY() + m.tabBarH() }

func (m Model) bodyTopY() int { return m.topBarY() + topBarH }

// collectionsHeaderRows is the number of non-tree lines the collections pane
// renders before its first item (the "COLLECTIONS" title + the "root:" line).
const collectionsHeaderRows = 2

// mouseScrollLines is how many lines one wheel notch scrolls the response body.
const mouseScrollLines = 3

// handleScroll scrolls the response viewport when the wheel turns over the
// response pane. It leaves focus untouched — scrolling shouldn't steal it — and
// arms a short window in which matching synthetic arrow keys are swallowed.
func (m Model) handleScroll(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.overResponsePane(msg.X, msg.Y) {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if m.hasResp {
			m.vp.ScrollUp(mouseScrollLines)
		}
	} else {
		if m.hasResp {
			m.vp.ScrollDown(mouseScrollLines)
		}
	}
	m.wheel.Arm()
	return m, nil
}

// shouldSuppressWheelArrow reports whether msg is synthetic vertical navigation
// emitted by the terminal as part of a recent wheel notch, which we drop so the
// focused pane doesn't move while the wheel scrolls the response.
func (m *Model) shouldSuppressWheelArrow(msg tea.KeyMsg) bool {
	return m.wheel.ShouldSuppress(msg.String())
}

// overResponsePane reports whether (x,y) falls inside the response pane's box.
func (m Model) overResponsePane(x, y int) bool {
	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	if y < m.bodyTopY() { // top method/URL bar or request-tabs row, never the response pane
		return false
	}
	respX := rightX + l.reqInnerW + borderOverhead + l.gap
	respBottom := m.bodyTopY() + l.respInnerH + borderOverhead
	return y < respBottom && x >= respX && x < respX+l.respInnerW+borderOverhead
}

// textPos is a position in the response text: a 0-based line and rune column.
type textPos struct{ line, col int }

// selectionStyle reverses the selected span so it reads as a highlight.
var selectionStyle = lipgloss.NewStyle().Reverse(true)

// responseTextPos maps a screen coordinate to a position in the response text,
// accounting for the viewport's vertical scroll. It returns ok=false when the
// point isn't over the scrollable body (e.g. the status line or tab bar).
func (m Model) responseTextPos(x, y int) (textPos, bool) {
	if !m.overResponsePane(x, y) {
		return textPos{}, false
	}
	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	respX := rightX + l.reqInnerW + borderOverhead + l.gap
	contentX := respX + 2          // pane border + padding
	vpTopY := m.bodyTopY() + 1 + 1 // pane top border + combined tab/status header row
	if y < vpTopY {
		return textPos{}, false
	}
	line := m.vp.YOffset + (y - vpTopY)
	col := x - contentX
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	return textPos{line: line, col: col}, true
}

// loadTextPos maps a screen coordinate to a position in the load-run view's
// plain-text grid. Unlike responseTextPos there is no tab/status header row
// and no scroll offset: the view starts directly below the pane's top border.
func (m Model) loadTextPos(x, y int) (textPos, bool) {
	if !m.overResponsePane(x, y) {
		return textPos{}, false
	}
	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	respX := rightX + l.reqInnerW + borderOverhead + l.gap
	contentX := respX + 2 // pane border + left padding
	topY := m.bodyTopY() + 1
	if y < topY {
		return textPos{}, false
	}
	col := x - contentX
	if col < 0 {
		col = 0
	}
	return textPos{line: y - topY, col: col}, true
}

// loadCopyPillClicked reports whether msg hit the copy pill on the finished
// run view's header row — mirror of copyButtonClicked for the load view.
func (m Model) loadCopyPillClicked(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionRelease {
		return false
	}
	if m.loadRun == nil || !m.loadSnap.Done || m.loadSummary == nil || m.loadPicker || m.shapeEdit {
		return false
	}
	if _, shown := m.loadRunHeader(m.vp.Width); !shown {
		return false // pane too narrow — the pill isn't on screen
	}
	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	respX := rightX + l.reqInnerW + borderOverhead + l.gap
	contentX := respX + 2
	buttonW := lipgloss.Width(m.copyButtonView())
	buttonEndX := contentX + m.vp.Width - 1
	buttonStartX := buttonEndX - buttonW + 1
	buttonY := m.bodyTopY() + 1 // the run view's header row
	return msg.Y == buttonY && msg.X >= buttonStartX && msg.X <= buttonEndX
}

// handleMouseDown begins a text selection when the press lands on the response
// body or the load-run view. Elsewhere it is a no-op; the click resolves on
// release.
func (m Model) handleMouseDown(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.loadRun != nil && !m.loadPicker && !m.shapeEdit {
		pos, ok := m.loadTextPos(msg.X, msg.Y)
		if !ok {
			return m, nil
		}
		m.selecting = true
		m.selLoad = true
		m.selDragged = false
		m.selAnchor = pos
		m.selCursor = pos
		return m, nil
	}
	if !m.hasResp {
		return m, nil
	}
	pos, ok := m.responseTextPos(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	m.selecting = true
	m.selLoad = false
	m.selDragged = false
	m.selAnchor = pos
	m.selCursor = pos
	return m, nil
}

// handleMouseDrag extends the in-progress selection and repaints the highlight.
func (m Model) handleMouseDrag(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.selLoad {
		pos, ok := m.loadTextPos(msg.X, msg.Y)
		if !ok {
			return m, nil // dragged off the pane; keep the last extent
		}
		m.selCursor = pos
		m.selDragged = true
		return m, nil // viewLoadRun renders the highlight itself
	}
	pos, ok := m.responseTextPos(msg.X, msg.Y)
	if !ok {
		return m, nil // dragged off the body; keep the last extent
	}
	m.selCursor = pos
	m.selDragged = true
	m.vp.SetContent(renderWithSelection(m.currentResponseText(), m.selAnchor, m.selCursor))
	return m, nil
}

// handleMouseUp finalizes a selection: a real drag copies the span to the
// clipboard; a plain click (no drag) falls through to normal click routing.
func (m Model) handleMouseUp(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.selecting {
		return m.handleClick(msg)
	}
	m.selecting = false
	fromLoad := m.selLoad
	m.selLoad = false
	if !fromLoad {
		m.vp.SetContent(m.currentResponseViewText()) // drop the selection highlight either way
	}
	if !m.selDragged {
		return m.handleClick(msg) // it was a click, not a drag
	}
	source := m.currentResponseText()
	if fromLoad {
		source = m.loadRunPlainText()
	}
	sel := selectedText(source, m.selAnchor, m.selCursor)
	if sel == "" {
		return m, nil
	}
	if err := clipboard.WriteAll(sel); err != nil {
		m.statusMsg = "clipboard unavailable"
	} else {
		m.statusMsg = fmt.Sprintf("copied %d chars to clipboard", len([]rune(sel)))
	}
	return m, nil
}

// orderSelection returns a,b so that a is not after b.
func orderSelection(a, b textPos) (textPos, textPos) {
	if b.line < a.line || (b.line == a.line && b.col < a.col) {
		return b, a
	}
	return a, b
}

// renderWithSelection returns text with the [a,b] span reverse-video highlighted.
func renderWithSelection(text string, a, b textPos) string {
	a, b = orderSelection(a, b)
	lines := strings.Split(text, "\n")
	for i := a.line; i <= b.line && i < len(lines); i++ {
		if i < 0 {
			continue
		}
		r := []rune(lines[i])
		lo, hi := 0, len(r)
		if i == a.line {
			lo = clampInt(a.col, 0, len(r))
		}
		if i == b.line {
			hi = clampInt(b.col, 0, len(r))
		}
		if lo >= hi {
			continue
		}
		lines[i] = string(r[:lo]) + selectionStyle.Render(string(r[lo:hi])) + string(r[hi:])
	}
	return strings.Join(lines, "\n")
}

// selectedText extracts the plain text covered by the [a,b] span.
func selectedText(text string, a, b textPos) string {
	a, b = orderSelection(a, b)
	lines := strings.Split(text, "\n")
	var out []string
	for i := a.line; i <= b.line && i < len(lines); i++ {
		if i < 0 {
			continue
		}
		r := []rune(lines[i])
		lo, hi := 0, len(r)
		if i == a.line {
			lo = clampInt(a.col, 0, len(r))
		}
		if i == b.line {
			hi = clampInt(b.col, 0, len(r))
		}
		if lo > hi {
			lo = hi
		}
		out = append(out, string(r[lo:hi]))
	}
	return strings.Join(out, "\n")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// handleClick routes a left button release to the pane under the pointer: it
// focuses that pane and runs a pane-specific action — cycle the method, place
// the URL caret, switch a request tab, or move the tree selection. Sending
// stays on the SEND button (and :send); the button takes precedence here.
func (m Model) handleClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionRelease {
		return m, nil
	}
	// Don't let clicks reach the panes while a modal/prompt owns the screen.
	if m.showHelp || m.cmdActive || m.collectionMenu ||
		m.pendingAction != pendingNone || m.confirmDelete != "" || m.confirmCloseTab ||
		m.loadPicker || m.loadConfirm || m.shapeEdit {
		return m, nil
	}
	if m.loadCopyPillClicked(msg) {
		return m.yankLoadSummary()
	}
	if m.sendButtonClicked(msg) {
		return m.send()
	}
	if m.testButtonClicked(msg) {
		return m.openLoadPicker()
	}
	if m.copyButtonClicked(msg) {
		return m.yankResponse()
	}

	l := m.computeLayout()
	x, y := msg.X, msg.Y

	rightX := 0 // left border column of the right-hand region (tabline/method/url/req/resp)
	if m.collectionShown {
		collW := l.collectionInnerW + borderOverhead
		if x < collW {
			if y <= l.collectionInnerH+1 {
				return m.clickCollections(y)
			}
			return m, nil
		}
		rightX = collW + l.gap
	}

	if y == m.tablineY() { // tabline row (below the tree's top border)
		return m.clickOpenTab(x, rightX)
	}

	if y >= m.topBarY() && y < m.bodyTopY() { // method / URL top bar
		methodEnd := rightX + l.methodInnerW + borderOverhead
		if x >= rightX && x < methodEnd {
			return m.setFocus(focusMethod).cycleMethod(1), nil
		}
		urlX := methodEnd + l.gap
		if x >= urlX && x < urlX+l.urlInnerW+borderOverhead {
			return m.clickURL(x, urlX, l), nil
		}
		return m, nil
	}

	if y < m.bodyTopY() { // blank row above the tabline — nothing to click
		return m, nil
	}

	// Body row: request pane on the left, response on the right.
	reqEnd := rightX + l.reqInnerW + borderOverhead
	if x >= rightX && x < reqEnd {
		return m.clickRequest(x, y, rightX), nil
	}
	respX := reqEnd + l.gap
	if x >= respX && x < respX+l.respInnerW+borderOverhead {
		return m.setFocus(focusResponse), nil
	}
	return m, nil
}

// doubleClickWindow is how close two clicks on the same tree row must be to
// count as a double-click.
const doubleClickWindow = 500 * time.Millisecond

// clickCollections focuses the tree and moves the selection to the clicked row.
// Clicking a request opens it as a request tab. Folders keep the old behavior: a
// double-click on the same row toggles them, reusing the same path as Enter.
func (m Model) clickCollections(y int) (tea.Model, tea.Cmd) {
	m = m.setFocus(focusCollection)
	row := y - (1 + collectionsHeaderRows) // top border + header lines
	if row < 0 {
		return m, nil
	}
	rows := m.collectionPane.rows()
	if row >= len(rows) {
		return m, nil
	}
	double := row == m.lastTreeClickRow && !m.lastTreeClick.IsZero() &&
		time.Since(m.lastTreeClick) <= doubleClickWindow
	m.collectionPane.cursor = row
	m.lastTreeClickRow = row
	m.lastTreeClick = time.Now()
	if rows[row].file {
		m.lastTreeClick = time.Time{} // request clicks open immediately; no double-click state to keep
		return m.openTabByName(rows[row].name)
	}
	if double {
		m.lastTreeClick = time.Time{} // consume, so a triple-click doesn't re-fire
		m.collectionPane.updateNormal(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m, nil
}

// clickOpenTab handles a click on the tabline: the trailing ✕ closes that tab,
// anywhere else on it switches to it. The tabline starts at the left edge of
// the right-hand column, so hit-testing begins at rightX — offset by the strip
// scroll that keeps the active tab visible, mirroring viewOpenTabs.
func (m Model) clickOpenTab(x, rightX int) (tea.Model, tea.Cmd) {
	labels := m.tabLabels()
	first := tabStripFirst(labels, m.activeTab, tablineWidth(m.computeLayout()))
	idx, onClose, ok := openTabHit(x, rightX, labels[first:])
	if !ok {
		return m, nil
	}
	idx += first
	if onClose {
		return m.closeTabAt(idx)
	}
	return m.switchOpenTabTo(idx)
}

func (m Model) switchOpenTab(delta int) (tea.Model, tea.Cmd) {
	if len(m.tabs) == 0 {
		return m, nil
	}
	idx := (m.activeTab + delta + len(m.tabs)) % len(m.tabs)
	return m.switchOpenTabTo(idx)
}

// switchOpenTabTo activates tab idx. The outgoing tab's live edits are captured
// into its buffer and the incoming tab's edits restored, so switching never
// blocks on unsaved changes and never reloads from disk — each tab keeps its own
// in-flight work.
func (m Model) switchOpenTabTo(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.tabs) || idx == m.activeTab {
		return m, nil
	}
	m = m.syncActiveTab()
	m.activeTab = idx
	m = m.loadActiveTab()
	return m, nil
}

// openTabHit maps a click column to a tab, given the rendered tab labels (so its
// widths match the tabline exactly, dirty markers included). onClose reports
// whether the click landed on that tab's trailing ✕ close button (its last
// openTabCloseZone cells).
func openTabHit(x, startX int, labels []string) (idx int, onClose bool, ok bool) {
	col := startX
	for i, label := range labels {
		if i > 0 {
			col += openTabGap
		}
		cellW := lipgloss.Width(label)
		if x >= col && x < col+cellW {
			return i, x >= col+cellW-openTabCloseZone, true
		}
		col += cellW
	}
	return 0, false, false
}

// clickURL focuses the URL bar (Insert) and places the caret at the clicked
// column, accounting for the horizontal scroll that was in effect when clicked.
func (m Model) clickURL(x, urlX int, l layout) Model {
	width := urlInputWidth(l)
	start := 0 // the scroll offset the user saw: 0 unless it was focused & scrolled
	if m.focus == focusURL {
		if _, col := m.url.Cursor(); width > 0 && col >= width {
			start = col - width + 1
		}
	}
	m = m.setFocus(focusURL)
	clicked := x - (urlX + borderOverhead) // text begins after the border + padding
	if clicked < 0 {
		clicked = 0
	}
	m.url.SetCursorCol(start + clicked)
	return m
}

// clickRequest focuses the request pane and, when the click lands on the tab
// bar row, switches to the clicked tab.
func (m Model) clickRequest(x, y, rightX int) Model {
	m = m.setFocus(focusRequest)
	if y == m.bodyTopY()+1 { // tab bar is the first content row (below the pane's border)
		if tab, ok := requestTabAt(x, rightX+borderOverhead); ok {
			m.reqPane.selectTab(tab)
		}
	}
	return m
}

// requestTabAt maps a screen column to a request-tab index. Tab cells render as
// " name " left-to-right from contentX; a click past the last tab returns false.
func requestTabAt(x, contentX int) (int, bool) {
	col := contentX
	for i, name := range tabNames {
		cellW := len(name) + 2 // one padding space on each side
		if x >= col && x < col+cellW {
			return i, true
		}
		col += cellW
	}
	return 0, false
}

func (m Model) sendButtonClicked(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionRelease {
		return false
	}
	startX, endX, _, _, y := m.urlBarButtonRects()
	return msg.Y == y && msg.X >= startX && msg.X <= endX
}

// testButtonClicked reports whether msg hit the TEST button at the URL bar's
// right edge.
func (m Model) testButtonClicked(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionRelease {
		return false
	}
	_, _, startX, endX, y := m.urlBarButtonRects()
	return msg.Y == y && msg.X >= startX && msg.X <= endX
}

// urlBarButtonRects is the clickable geometry of the URL bar's buttons,
// mirroring viewURLBar's "… SEND TEST" order: TEST right-aligned at the pane
// edge, SEND one gap cell before it.
func (m Model) urlBarButtonRects() (sendStart, sendEnd, testStart, testEnd, y int) {
	l := m.computeLayout()
	paneX := 0
	if m.collectionShown {
		paneX = l.collectionInnerW + borderOverhead + l.gap
	}
	// The URL pane sits after the fixed-width method selector on the top row.
	urlPaneX := paneX + l.methodInnerW + borderOverhead + l.gap
	y = m.topBarY() + 1 // URL pane content row, below the top border.
	testEnd = urlPaneX + l.urlInnerW - 1
	testStart = testEnd - lipgloss.Width(m.testButtonView()) + 1
	sendEnd = testStart - 2 // skip the separating space
	sendStart = sendEnd - lipgloss.Width(m.sendButtonView()) + 1
	return sendStart, sendEnd, testStart, testEnd, y
}

// copyButtonClicked reports whether msg hit the copy pill on the response
// header. The button is only drawn for a completed (non-in-flight) response, so
// the geometry mirrors respHeaderBar: right-aligned within the viewport width,
// on the header row just below the pane's top border.
func (m Model) copyButtonClicked(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionRelease {
		return false
	}
	if !m.copyButtonShown(m.vp.Width) { // covers no-response, in-flight, and narrow panes
		return false
	}
	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	respX := rightX + l.reqInnerW + borderOverhead + l.gap
	contentX := respX + 2 // pane border + left padding, matching responseTextPos
	buttonW := lipgloss.Width(m.copyButtonView())
	buttonEndX := contentX + m.vp.Width - 1
	buttonStartX := buttonEndX - buttonW + 1
	buttonY := m.bodyTopY() + 1 // response header row, below the pane's top border
	return msg.Y == buttonY && msg.X >= buttonStartX && msg.X <= buttonEndX
}

func (m Model) cycleMethod(delta int) Model {
	n := len(model.Methods)
	m.methodIdx = (m.methodIdx + delta + n) % n
	m.req.Method = model.Methods[m.methodIdx]
	return m
}

// updateResponseNav handles Vim scroll motions in the response pane.
func (m Model) updateResponseNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.loadViewShown() {
		return m, nil // the load view owns the pane; its keys are routed upstream
	}
	// "gg" is the only two-key motion here; clear a stale pending 'g' for any
	// other key so it can't later fire an unexpected GotoTop.
	if msg.String() != "g" {
		m.pendingG = false
	}
	switch msg.String() {
	case "g":
		if m.pendingG {
			m.vp.GotoTop()
			m.pendingG = false
		} else {
			m.pendingG = true
		}
		return m, nil
	case "G":
		m.vp.GotoBottom()
		m.pendingG = false
		return m, nil
	case "[", "]":
		if m.hasResp {
			m.respTab = 1 - m.respTab
			m.resetSearch()
			m.vp.GotoTop()
		}
		return m, nil
	case "p":
		// Toggle raw ↔ pretty for the JSON body view.
		if m.hasResp && m.respTab == 0 {
			m.rawBody = !m.rawBody
			m.respText = renderResponseBody(m.resp, m.vp.Width, m.rawBody)
			m.resetSearch()
			m.vp.SetContent(m.currentResponseViewText())
			m.vp.GotoTop()
		}
		return m, nil
	case "/":
		if m.hasResp {
			return m.openCommandLine('/'), nil
		}
		return m, nil
	case "n":
		return m.jumpMatch(1), nil
	case "N":
		return m.jumpMatch(-1), nil
	case "y":
		return m.yankResponse()
	}
	m.pendingG = false
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// setFocus updates focus and keeps the request pane's highlight in sync.
func (m Model) setFocus(f focus) Model {
	if f == focusCollection && !m.collectionShown {
		f = focusRequest
	}
	m.focus = f
	m.collectionPane.focused = f == focusCollection
	m.reqPane.setFocused(f == focusRequest)
	// Focusing the URL bar begins typing immediately (Insert); leaving it drops
	// the buffer to NORMAL. (esc within the bar also drops to NORMAL — a vim
	// sub-mode with the full motion/edit set — without moving focus.)
	if f == focusURL {
		m.url.SetMode(vimtext.Insert)
		m.url.CursorEnd()
	} else {
		m.url.SetMode(vimtext.Normal)
	}
	return m
}

// arrowsAsHJKL rewrites an arrow key into its Vim h/j/k/l equivalent so the two
// are interchangeable in NORMAL mode. Non-arrow keys pass through unchanged.
func arrowsAsHJKL(msg tea.KeyMsg) tea.KeyMsg {
	switch msg.Type {
	case tea.KeyLeft:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	case tea.KeyDown:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	case tea.KeyUp:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	case tea.KeyRight:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	}
	return msg
}

// cycleFocus moves focus forward (dir=+1) or backward (dir=-1), skipping the
// Collections pane while it is hidden so backward cycling can't get stuck.
func (m Model) cycleFocus(dir int) Model {
	n := len(tabOrder)
	idx := 0
	for i, f := range tabOrder {
		if f == m.focus {
			idx = i
			break
		}
	}
	for i := 0; i < n; i++ {
		idx = (idx + dir + n) % n
		f := tabOrder[idx]
		if f == focusCollection && !m.collectionShown {
			continue
		}
		return m.setFocus(f)
	}
	return m
}

// focus movement helpers — the top row is the Method selector then the URL bar
// (which carries the inline timeout readout); below it the Request pane on the
// left and the Response pane on the right.
func (m Model) focusLeft() focus {
	switch m.focus {
	case focusURL:
		return focusMethod
	case focusMethod:
		if m.collectionShown {
			return focusCollection
		}
	case focusResponse:
		return focusRequest
	case focusRequest:
		if m.collectionShown {
			return focusCollection
		}
	}
	return m.focus
}
func (m Model) focusRight() focus {
	switch m.focus {
	case focusMethod:
		return focusURL
	case focusCollection:
		return focusRequest
	case focusRequest, focusURL:
		return focusResponse
	}
	return m.focus
}
func (m Model) focusUp() focus {
	switch m.focus {
	case focusResponse, focusCollection, focusRequest:
		// The top row (URL bar) spans above the request/response panes.
		return focusURL
	}
	return m.focus
}

// (focusMethod sits on the top row; it has no pane above it.)
func (m Model) focusDown() focus {
	switch m.focus {
	case focusURL, focusMethod:
		return focusRequest
	}
	return m.focus
}

// moveFocusDown handles ctrl+w j / ctrl+w ↓. From the top method/URL bar, the
// pane directly below is the request body editor, not the collections tree.
func (m Model) moveFocusDown() Model {
	if m.focus == focusURL || m.focus == focusMethod {
		m = m.setFocus(focusRequest)
		m.reqPane.selectTab(tabBody)
		return m
	}
	return m.setFocus(m.focusDown())
}

// currentResponseText returns the text shown in the response viewport for
// the active response tab.
func (m Model) currentResponseText() string {
	if m.respTab == 1 {
		return m.respHeaders
	}
	return m.respText
}

func (m Model) currentResponseViewText() string {
	if m.respTab == 1 {
		return m.respHeaders
	}
	return highlightResponseText(m.respText)
}

// send fires the current request (merging headers/body/query) if idle. The
// request runs under a cancellable context whose cancel func is stashed on the
// model so esc can abort it mid-flight.
func (m Model) send() (tea.Model, tea.Cmd) {
	if m.sending {
		return m, nil
	}
	if m.loadRunning() {
		m.statusMsg = "load test running — esc to stop it before sending"
		return m, nil
	}
	if strings.TrimSpace(m.url.Text()) == "" {
		m.statusMsg = "cannot send: URL is empty"
		return m, nil
	}
	m.loadRun = nil // a normal send reclaims the response pane from old results
	m.loadPicker = false
	built := m.buildRequest()
	if missing := vars.Unresolved(built); len(missing) > 0 {
		wrapped := make([]string, len(missing))
		for i, n := range missing {
			wrapped[i] = "{{" + n + "}}"
		}
		m.statusMsg = "sending with unresolved vars: " + strings.Join(wrapped, ", ")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.sendSeq++
	m.sending = true
	return m, tea.Batch(m.spin.Tick, sendCmd(ctx, m.sendSeq, built))
}

// cancelSend aborts an in-flight request. The pending sendCmd still delivers a
// responseMsg (carrying the context-cancelled error), but bumping sendSeq marks
// that result stale so it is dropped, and clearing sending stops the spinner and
// frees the UI for a fresh send immediately.
func (m Model) cancelSend() (tea.Model, tea.Cmd) {
	if !m.sending {
		return m, nil
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.sendSeq++
	m.sending = false
	m.statusMsg = "request cancelled"
	return m, nil
}
