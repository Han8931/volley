package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/model"
)

// failingStore returns a Store whose Root sits under a regular file, so every
// MkdirAll/write fails with ENOTDIR — used to exercise save-failure paths.
func failingStore(t *testing.T) collections.Store {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return collections.Store{Root: filepath.Join(blocker, "collections")}
}

// step applies a message and returns the concrete Model for chaining.
func step(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// runes builds a rune key message ("o", "X-Test", …).
func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// urlNormal drops the URL bar out of its always-on typing mode into the NORMAL
// sub-mode (method/timeout/nav shortcuts), the way pressing esc does.
func urlNormal(m Model) Model { return step(m, keyEsc) }

var (
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
)

var (
	keyCtrlW    = tea.KeyMsg{Type: tea.KeyCtrlW}
	keyDown     = tea.KeyMsg{Type: tea.KeyDown}
	keyUp       = tea.KeyMsg{Type: tea.KeyUp}
	keyTab      = tea.KeyMsg{Type: tea.KeyTab}
	keyShiftTab = tea.KeyMsg{Type: tea.KeyShiftTab}
	keyLeft     = tea.KeyMsg{Type: tea.KeyLeft}
	keyRight    = tea.KeyMsg{Type: tea.KeyRight}
)

func TestFocusNavigation(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	if base.focus != focusURL {
		t.Fatalf("initial focus = %v, want URL", base.focus)
	}

	// ctrl+w j moves down from URL into the collections tree.
	m := step(step(base, keyCtrlW), runes("j"))
	if m.focus != focusCollection {
		t.Errorf("ctrl+w j: focus = %v, want Collections", m.focus)
	}
	// ctrl+w l moves collections -> request -> response.
	m = step(step(m, keyCtrlW), runes("l"))
	if m.focus != focusRequest {
		t.Errorf("ctrl+w l: focus = %v, want Request", m.focus)
	}
	m = step(step(m, keyCtrlW), runes("l"))
	if m.focus != focusResponse {
		t.Errorf("second ctrl+w l: focus = %v, want Response", m.focus)
	}

	// ctrl+w h from the URL bar steps left onto the method selector; again to the tree.
	if got := step(step(base, keyCtrlW), runes("h")).focus; got != focusMethod {
		t.Errorf("ctrl+w h from URL: focus = %v, want Method", got)
	}
	if got := step(step(step(step(base, keyCtrlW), runes("h")), keyCtrlW), runes("h")).focus; got != focusCollection {
		t.Errorf("ctrl+w h h from URL: focus = %v, want Collections", got)
	}
	// The URL bar types directly, so a left arrow moves the cursor rather than
	// navigating — focus stays on the URL.
	if got := step(base, tea.KeyMsg{Type: tea.KeyLeft}).focus; got != focusURL {
		t.Errorf("left arrow from URL: focus = %v, want URL (cursor move)", got)
	}

	// Arrow keys no longer hop panes: in the URL text field down/up are inert,
	// so focus stays put. Pane moves are ctrl+w h/j/k/l or tab.
	if got := step(base, keyDown).focus; got != focusURL {
		t.Errorf("down arrow from URL should not change focus, got %v", got)
	}
	// Tab follows reading order: Collections, Method, URL, Request, Response.
	// From URL, tab advances to Request; shift+tab steps back to Method.
	if got := step(base, keyTab).focus; got != focusRequest {
		t.Errorf("tab from URL: focus = %v, want Request", got)
	}
	if got := step(step(base, keyTab), keyTab).focus; got != focusResponse {
		t.Errorf("tab tab from URL: focus = %v, want Response", got)
	}
	if got := step(base, keyShiftTab).focus; got != focusMethod {
		t.Errorf("shift+tab from URL: focus = %v, want Method", got)
	}
	// Bare keys no longer hop panes: j in the URL NORMAL sub-mode leaves focus put.
	if got := step(urlNormal(base), runes("j")).focus; got != focusURL {
		t.Errorf("j from URL normal should not change focus, got %v", got)
	}
}

func TestArrowsNeverChangeFocus(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})

	// From every pane, a bare arrow key does in-pane motion only — focus stays put.
	for _, f := range []focus{
		focusURL, focusMethod, focusCollection, focusRequest, focusResponse,
	} {
		m := base.setFocus(f)
		for _, k := range []tea.KeyMsg{keyUp, keyDown, keyLeft, keyRight} {
			if got := step(m, k).focus; got != f {
				t.Errorf("bare arrow from %v changed focus to %v", f, got)
			}
		}
	}

	// Focus changes require tab or the ctrl+w chord — ctrl+w + arrow works.
	if got := step(step(base, keyCtrlW), keyDown).focus; got != focusCollection {
		t.Errorf("ctrl+w ↓ from URL: focus = %v, want Collections", got)
	}
	if got := step(step(base.setFocus(focusResponse), keyCtrlW), keyUp).focus; got != focusURL {
		t.Errorf("ctrl+w ↑ from Response: focus = %v, want URL (top row)", got)
	}
}

func TestURLBarTypesDirectly(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})

	// On launch the URL bar is ready for input — no 'i' needed.
	if !base.url.Focused() {
		t.Fatal("URL bar should accept typing immediately on launch")
	}
	m := step(base, runes("https://api.test/x"))
	if got := m.url.Value(); got != "https://api.test/x" {
		t.Fatalf("typed URL = %q, want the literal text", got)
	}
	// Characters that are ex-command/normal keys elsewhere are just text here.
	m = step(m, runes("?a=:b["))
	if got := m.url.Value(); got != "https://api.test/x?a=:b[" {
		t.Errorf("special chars should type into the URL, got %q", got)
	}

	// Enter sends rather than inserting a newline.
	sent, cmd := m.Update(keyEnter)
	if !sent.(Model).sending || cmd == nil {
		t.Error("enter in the URL bar should send the request")
	}

	// esc drops to the NORMAL sub-mode (blurred), where shortcut keys work.
	n := urlNormal(m)
	if n.url.Focused() {
		t.Error("esc should leave the URL input (drop to NORMAL)")
	}
	if n.focus != focusURL {
		t.Errorf("esc should keep focus on the URL bar, got %v", n.focus)
	}
}

func TestTimeoutEditableInline(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})

	// Timeout is no longer a pane/tab stop: the 't' shortcut from the URL NORMAL
	// sub-mode jumps straight to editing the inline field, without moving focus
	// off the URL bar.
	m := step(urlNormal(base), runes("t"))
	if !m.timeoutInput.Focused() || !m.editing() {
		t.Fatalf("t from URL normal should begin editing the timeout, focused=%v editing=%v",
			m.timeoutInput.Focused(), m.editing())
	}
	if m.focus != focusURL {
		t.Errorf("editing timeout should keep focus on the URL bar, got %v", m.focus)
	}

	// Typing a duration and committing sets the timeout and ends editing.
	m = step(m, runes("2s"))
	m = step(m, keyEnter)
	if m.timeout != 2*time.Second {
		t.Errorf("timeout = %v, want 2s", m.timeout)
	}
	if m.timeoutInput.Focused() || m.editing() {
		t.Errorf("commit should end timeout editing, focused=%v editing=%v",
			m.timeoutInput.Focused(), m.editing())
	}

	// :timeout also sets it without any focus change.
	m2, _ := base.executeCommand("timeout 5s")
	if got := m2.(Model).timeout; got != 5*time.Second {
		t.Errorf(":timeout 5s = %v, want 5s", got)
	}
}

func TestRawPrettyToggle(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetValue("https://x.test")
	m = step(m, keyEnter)
	m = step(m, responseMsg{seq: m.sendSeq, resp: model.Response{
		Status: "200 OK", StatusCode: 200, Body: []byte(`{"a":1,"b":2}`),
	}})

	// Default view pretty-prints JSON (indented across multiple lines).
	if !strings.Contains(m.respText, "\n") {
		t.Fatalf("default body should be pretty-printed, got %q", m.respText)
	}

	// p on the response pane switches to the verbatim bytes.
	m = m.setFocus(focusResponse)
	m = step(m, runes("p"))
	if !m.rawBody || m.respText != `{"a":1,"b":2}` {
		t.Errorf("after p: rawBody=%v body=%q, want raw verbatim body", m.rawBody, m.respText)
	}
	if !strings.Contains(m.View(), "raw") {
		t.Error("tab bar should indicate the raw mode")
	}

	// p again restores pretty.
	m = step(m, runes("p"))
	if m.rawBody || !strings.Contains(m.respText, "\n") {
		t.Errorf("second p should restore pretty: rawBody=%v body=%q", m.rawBody, m.respText)
	}
}

func TestDocNameAndDirtyIndicator(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})

	// A fresh, unedited buffer shows [No Name] and no dirty marker.
	if v := base.View(); !strings.Contains(v, "[No Name]") || strings.Contains(v, "[+]") {
		t.Errorf("blank buffer should read [No Name] with no [+]")
	}

	// Editing the URL marks the buffer dirty.
	m := step(base, runes("https://x.test"))
	if !strings.Contains(m.View(), "[+]") {
		t.Errorf("an edited buffer should show the [+] dirty marker")
	}

	// A named, clean request shows its name and drops the marker.
	named := base
	named.currentName = "auth/login"
	named.baseline = named.rawRequest()
	v := named.View()
	if !strings.Contains(v, "auth/login") {
		t.Errorf("status bar should show the current request name")
	}
	if strings.Contains(v, "[+]") {
		t.Errorf("a clean named request should not show [+]")
	}
}

func TestTimeoutReadoutInTopBar(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})

	// The readout lives inline in the top bar (no separate options pane).
	if v := base.View(); !strings.Contains(v, "timeout") {
		t.Fatal("top bar should carry an inline timeout readout")
	}
	// After setting a value it shows through in the rendered top bar.
	m, _ := base.executeCommand("timeout 7s")
	if v := m.(Model).View(); !strings.Contains(v, "7s") {
		t.Errorf("top bar should show the current timeout value 7s")
	}
}

func TestEditEmptyHeaderImmediately(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m.setFocus(focusRequest) // Headers tab, no rows yet

	m = step(m, runes("i")) // should create a row AND start editing
	if !m.editing() {
		t.Fatal("i on an empty header list should create a row and edit it")
	}
	m = step(m, runes("Accept"))
	m = step(m, keyEsc)
	if got := m.buildRequest().Headers; len(got) != 1 || got[0].Name != "Accept" {
		t.Errorf("headers = %+v, want one Accept row", got)
	}
}

func TestRequestEditorVimTableMotions(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m.setFocus(focusRequest)

	m = step(m, runes("o"))
	m = step(m, runes("First"))
	m = step(m, keyEsc)
	m = step(m, runes("o"))
	m = step(m, runes("Second"))
	m = step(m, keyEsc)

	// gg jumps to the first row; A edits the value cell.
	m = step(step(m, runes("g")), runes("g"))
	m = step(m, runes("A"))
	m = step(m, runes("one"))
	m = step(m, keyEsc)

	// G jumps to the last row; $ also selects the value cell.
	m = step(m, runes("G"))
	m = step(m, runes("$"))
	m = step(m, runes("i"))
	m = step(m, runes("two"))
	m = step(m, keyEsc)

	got := m.buildRequest().Headers
	if len(got) != 2 || got[0].Value != "one" || got[1].Value != "two" {
		t.Fatalf("headers = %+v, want values set via gg/G motions", got)
	}
}

func TestRequestEditorBuildsHeaders(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m.setFocus(focusRequest) // Headers tab is active by default

	// o adds a row and starts editing the key cell.
	m = step(m, runes("o"))
	if !m.editing() {
		t.Fatal("after 'o' the editor should be editing the new row")
	}
	m = step(m, runes("X-Test"))
	m = step(m, keyEsc) // commit key

	// l → value cell, i → edit, type, esc.
	m = step(m, runes("l"))
	m = step(m, runes("i"))
	m = step(m, runes("yes"))
	m = step(m, keyEsc)

	got := m.buildRequest().Headers
	if len(got) != 1 || got[0].Name != "X-Test" || got[0].Value != "yes" || !got[0].Enabled {
		t.Fatalf("built headers = %+v, want [{X-Test yes true}]", got)
	}

	// space toggles the row off.
	m = step(m, runes(" "))
	if m.buildRequest().Headers[0].Enabled {
		t.Error("space should toggle the header off")
	}
}

func TestRequestTabsReachableFromURLFocus(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.focus != focusURL {
		t.Fatalf("initial focus = %v, want URL", m.focus)
	}

	m = step(urlNormal(m), runes("]"))
	if m.focus != focusRequest || m.reqPane.tab != tabBody {
		t.Fatalf("] from URL normal focus = focus %v tab %d, want Request/Body", m.focus, m.reqPane.tab)
	}

	m = step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = step(urlNormal(m), runes("["))
	if m.focus != focusRequest || m.reqPane.tab != tabQuery {
		t.Fatalf("[ from URL normal focus = focus %v tab %d, want Request/Query", m.focus, m.reqPane.tab)
	}
}

func TestTabSwitchingAndBodyEdit(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m.setFocus(focusRequest)

	m = step(m, runes("L")) // next tab: Body
	if m.reqPane.tab != tabBody {
		t.Fatalf("tab = %d, want Body", m.reqPane.tab)
	}
	m = step(m, runes("i")) // start editing body
	if !m.editing() {
		t.Fatal("body should be in edit mode after 'i'")
	}
	m = step(m, runes(`{"q":1}`))
	m = step(m, keyEsc)
	if got := m.buildRequest().Body; got != `{"q":1}` {
		t.Errorf("body = %q", got)
	}
}

func TestBodyVimModalFlow(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m.setFocus(focusRequest)
	m = step(m, runes("L")) // Body tab

	// i activates the editor in INSERT.
	m = step(m, runes("i"))
	if !m.inInsert() {
		t.Fatal("i should enter INSERT in the body")
	}
	m = step(m, runes("hello"))

	// esc drops to field-NORMAL: still capturing, but not inserting.
	m = step(m, keyEsc)
	if m.inInsert() {
		t.Error("esc should leave INSERT")
	}
	if !m.editing() {
		t.Error("field-normal should still capture keys")
	}

	// A Vim command works on the body: x deletes the char under the cursor.
	m = step(m, runes("x"))
	if got := m.buildRequest().Body; got != "hell" {
		t.Errorf("after x, body = %q, want hell", got)
	}
	// u undoes it.
	m = step(m, runes("u"))
	if got := m.buildRequest().Body; got != "hello" {
		t.Errorf("after u, body = %q, want hello", got)
	}

	// esc again releases back to pane navigation.
	m = step(m, keyEsc)
	if m.editing() {
		t.Error("second esc should release the field")
	}
}

func TestAppendQuery(t *testing.T) {
	got := appendQuery("https://x.test/api", []model.KV{
		{Key: "a", Value: "1", Enabled: true},
		{Key: "skip", Value: "9", Enabled: false},
	})
	if got != "https://x.test/api?a=1" {
		t.Errorf("appendQuery = %q", got)
	}
}

func TestEnterSendsAndGoesInFlight(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetValue("https://example.test")
	m2, cmd := m.Update(keyEnter)
	if !m2.(Model).sending {
		t.Error("Enter should put the model in-flight")
	}
	if cmd == nil {
		t.Error("Enter should return a command")
	}
}

func TestSendWarnsOnUnresolvedVars(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetValue("https://{{host}}/ping")
	m = step(m, keyEnter)
	if !m.sending {
		t.Fatal("Enter should still send even with unresolved vars")
	}
	if !strings.Contains(m.statusMsg, "{{host}}") {
		t.Errorf("statusMsg = %q, want it to warn about {{host}}", m.statusMsg)
	}

	// A fully-resolved send leaves no unresolved-var warning.
	m2 := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m2.url.SetValue("https://example.test/ping")
	m2 = step(m2, keyEnter)
	if strings.Contains(m2.statusMsg, "unresolved") {
		t.Errorf("statusMsg = %q, want no unresolved-var warning", m2.statusMsg)
	}
}

func TestSendWarnsOnUnresolvedQueryVar(t *testing.T) {
	// A var only in a query param must still be flagged, even though
	// buildRequest folds and percent-encodes the query into the URL.
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m.applyRequest(model.Request{
		Method: "GET",
		URL:    "https://example.test/x",
		Query:  []model.KV{{Key: "q", Value: "{{tok}}", Enabled: true}},
	})
	m = step(m, keyEnter)
	if !strings.Contains(m.statusMsg, "{{tok}}") {
		t.Errorf("statusMsg = %q, want warning about {{tok}} in the query", m.statusMsg)
	}
}

func TestWriteQuitBlockedWhenSaveFails(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = failingStore(t)
	m.currentName = "api/thing"
	m.url.SetValue("https://example.test")

	_, cmd := m.executeCommand("wq")
	if isQuit(cmd) {
		t.Error(":wq must not quit when the save fails — that would lose edits")
	}
}

func TestSavePromptKeepsWorkWhenSaveFails(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = failingStore(t)
	m.currentName = "api/thing"
	m.url.SetValue("https://changed.test") // make it dirty so quitting arms the prompt

	tm, _ := m.guardedQuit()
	m = tm.(Model)
	if m.pendingAction != pendingQuit {
		t.Fatalf("guardedQuit should arm the save prompt, pendingAction=%v", m.pendingAction)
	}

	// Answering 'y' triggers a save that fails: we must neither quit nor drop
	// the pending action (which would discard the edits).
	tm, cmd := m.resolveSaveConfirm(runes("y"))
	m = tm.(Model)
	if isQuit(cmd) {
		t.Error("failed save on 'y' must not quit")
	}
	if m.pendingAction != pendingQuit {
		t.Error("failed save must keep the prompt armed, not perform the pending transition")
	}
}

func TestNewSavedRequestDoesNotClaimNameOnFailure(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = failingStore(t)
	m = m.newSavedRequest("api/new")
	if m.currentName == "api/new" {
		t.Error("currentName must not be set when the create/save failed")
	}
	if !strings.Contains(m.statusMsg, "failed") {
		t.Errorf("statusMsg = %q, want a failure message", m.statusMsg)
	}
}

func TestEscCancelsInFlight(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetValue("https://example.test")
	m = step(m, keyEnter)
	if !m.sending {
		t.Fatal("Enter should put the model in-flight")
	}
	inFlightSeq := m.sendSeq

	// esc while sending aborts the request instead of its usual mode-exit.
	m = step(m, keyEsc)
	if m.sending {
		t.Error("esc should stop the in-flight request")
	}
	if m.cancel != nil {
		t.Error("cancel func should be cleared after abort")
	}
	if m.statusMsg != "request cancelled" {
		t.Errorf("statusMsg = %q, want \"request cancelled\"", m.statusMsg)
	}

	// The aborted request's late response must be dropped, not rendered.
	m = step(m, responseMsg{seq: inFlightSeq, resp: model.Response{
		Status: "200 OK", StatusCode: 200, Body: []byte("late"),
	}})
	if m.hasResp {
		t.Error("a cancelled request's late response should be ignored")
	}
}

func TestLayoutFitsWindow(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	out := m.View()
	if got := lipgloss.Width(out); got > 120 {
		t.Fatalf("view width = %d, want <= 120\n%s", got, out)
	}
	if got := lipgloss.Height(out); got > 40 {
		t.Fatalf("view height = %d, want <= 40\n%s", got, out)
	}
}

func TestToggleCollectionTree(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m.setFocus(focusCollection)
	m = step(step(m, runes(",")), runes("n"))
	if m.collectionShown {
		t.Fatal(",n should hide the collections tree")
	}
	if m.focus == focusCollection {
		t.Fatal("hiding the tree should move focus out of the collections pane")
	}
	if got := step(m, tea.KeyMsg{Type: tea.KeyLeft}).focus; got == focusCollection {
		t.Fatal("left focus should not enter hidden tree")
	}
	m = step(step(m, runes(",")), runes("n"))
	if !m.collectionShown {
		t.Fatal("second ,n should show the collections tree")
	}
}

func TestRenderLifecycle(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 100, Height: 30})

	// The URL bar accepts typing on launch, so the mode tag starts at INSERT;
	// esc drops it to NORMAL.
	if out := m.View(); !strings.Contains(out, "INSERT") {
		t.Error("status bar should show INSERT mode while the URL bar is active")
	}
	if out := urlNormal(m).View(); !strings.Contains(out, "NORMAL") {
		t.Error("status bar should show NORMAL mode after esc")
	}

	// A completed JSON response should render its status and pretty body.
	m = step(m, responseMsg{resp: model.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       []byte(`{"a":1}`),
		Size:       7,
	}})
	m.focus = focusResponse
	out := m.View()
	if !strings.Contains(out, "200 OK") {
		t.Error("response status line missing")
	}
	if !strings.Contains(out, `"a": 1`) {
		t.Errorf("body should be pretty-printed JSON, got:\n%s", out)
	}
}

func TestMethodCycle(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.req.Method != "GET" {
		t.Fatalf("default method = %q", m.req.Method)
	}
	// The method is its own pane: reach it (shift+tab from URL, since Method
	// precedes URL in reading order) and cycle with the down/up arrow keys.
	m = step(m, keyShiftTab)
	if m.focus != focusMethod {
		t.Fatalf("shift+tab from URL: focus = %v, want Method", m.focus)
	}
	m = step(m, keyDown) // ↓ == j == next
	if m.req.Method != "POST" {
		t.Errorf("after ↓, method = %q, want POST", m.req.Method)
	}
	m = step(m, keyUp) // ↑ == k == previous
	if m.req.Method != "GET" {
		t.Errorf("after ↑, method = %q, want GET", m.req.Method)
	}
	// j/k also work directly.
	m = step(m, runes("j"))
	if m.req.Method != "POST" {
		t.Errorf("after j, method = %q, want POST", m.req.Method)
	}
	m = step(m, runes("k"))
	if m.req.Method != "GET" {
		t.Errorf("after k, method = %q, want GET", m.req.Method)
	}
}

func TestPrettyJSON(t *testing.T) {
	if _, ok := prettyJSON([]byte("not json")); ok {
		t.Error("plain text should not be treated as JSON")
	}
	out, ok := prettyJSON([]byte(`{"x":[1,2]}`))
	if !ok || !strings.Contains(string(out), "\n") {
		t.Errorf("valid JSON should indent, got %q ok=%v", out, ok)
	}
}
