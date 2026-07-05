package tui

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
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

// sendNow fires the request via the :send command — the keyboard send path now
// that the Enter key no longer sends.
func sendNow(m Model) Model {
	tm, _ := m.executeCommand("send")
	return tm.(Model)
}

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

	// ctrl+w j moves down from URL directly into the request Body editor.
	m := step(step(base, keyCtrlW), runes("j"))
	if m.focus != focusRequest || m.reqPane.tab != tabBody {
		t.Errorf("ctrl+w j: focus/tab = %v/%d, want Request/Body", m.focus, m.reqPane.tab)
	}
	// ctrl+w l moves request -> response.
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

	// The method selector is also on the top bar, so ctrl+w j goes to Body too.
	m = step(step(base.setFocus(focusMethod), keyCtrlW), runes("j"))
	if m.focus != focusRequest || m.reqPane.tab != tabBody {
		t.Errorf("ctrl+w j from Method: focus/tab = %v/%d, want Request/Body", m.focus, m.reqPane.tab)
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
	m := step(step(base, keyCtrlW), keyDown)
	if m.focus != focusRequest || m.reqPane.tab != tabBody {
		t.Errorf("ctrl+w ↓ from URL: focus/tab = %v/%d, want Request/Body", m.focus, m.reqPane.tab)
	}
	if got := step(step(base.setFocus(focusResponse), keyCtrlW), keyUp).focus; got != focusURL {
		t.Errorf("ctrl+w ↑ from Response: focus = %v, want URL (top row)", got)
	}
}

func TestMethodPaneRKeyCyclesMethod(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40}).setFocus(focusMethod)
	if m.req.Method != "GET" {
		t.Fatalf("default method = %q, want GET", m.req.Method)
	}
	// 'r' advances the method (GET -> POST -> PUT).
	m = step(m, runes("r"))
	if m.req.Method != "POST" {
		t.Errorf("after r: method = %q, want POST", m.req.Method)
	}
	m = step(m, runes("r"))
	if m.req.Method != "PUT" {
		t.Errorf("after rr: method = %q, want PUT", m.req.Method)
	}
	// Only r cycles: R, j/k and space are all inert.
	for _, k := range []string{"R", "j", "k", " "} {
		if got := step(m, runes(k)).req.Method; got != "PUT" {
			t.Errorf("%q must not change the method, got %q", k, got)
		}
	}
}

func TestURLBarTypesDirectly(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})

	// On launch the URL bar is ready for input — no 'i' needed.
	if !base.urlInsert() {
		t.Fatal("URL bar should accept typing immediately on launch")
	}
	m := step(base, runes("https://api.test/x"))
	if got := m.url.Text(); got != "https://api.test/x" {
		t.Fatalf("typed URL = %q, want the literal text", got)
	}
	// Characters that are ex-command/normal keys elsewhere are just text here.
	m = step(m, runes("?a=:b["))
	if got := m.url.Text(); got != "https://api.test/x?a=:b[" {
		t.Errorf("special chars should type into the URL, got %q", got)
	}

	// Enter no longer sends — it's a harmless no-op in the single-line buffer.
	// Sending is only via :send.
	notSent, _ := m.Update(keyEnter)
	if notSent.(Model).sending {
		t.Error("enter in the URL bar must NOT send the request")
	}
	if s := sendNow(m); !s.sending {
		t.Error(":send should fire the request")
	}

	// esc drops to the NORMAL sub-mode (blurred), where shortcut keys work.
	n := urlNormal(m)
	if n.urlInsert() {
		t.Error("esc should leave the URL input (drop to NORMAL)")
	}
	if n.focus != focusURL {
		t.Errorf("esc should keep focus on the URL bar, got %v", n.focus)
	}
}

// TestURLBarLongURLIndicators verifies a clipped long URL is flagged: … when
// unfocused-truncated, and ‹ / › for text scrolled off an edge when focused.
func TestURLBarLongURLIndicators(t *testing.T) {
	const long = "https://api.example.com/v1/resource/12345?expand=orders"
	const width = 20

	// Unfocused: head shown, tail dropped behind a trailing … .
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText(long)
	m = m.setFocus(focusResponse) // focus off the URL bar
	if got := m.renderURLField(width); !strings.HasSuffix(got, "…") || strings.Contains(got, "orders") {
		t.Errorf("unfocused long URL = %q, want head + trailing … (no tail)", got)
	}

	// Focused with the cursor at the end (scrolled right): leading ‹ for the
	// hidden head.
	f := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	f.url.SetText(long) // cursor at end
	if got := f.renderURLField(width); !strings.Contains(got, "‹") {
		t.Errorf("focused, scrolled right = %q, want a leading ‹", got)
	}

	// Focused with the cursor at column 0: trailing › for the hidden tail.
	f2 := urlNormal(step(New(), tea.WindowSizeMsg{Width: 120, Height: 40}))
	f2.url.SetText(long)
	f2 = step(f2, runes("0")) // jump to the start in NORMAL
	if got := f2.renderURLField(width); !strings.Contains(got, "›") {
		t.Errorf("focused at start = %q, want a trailing ›", got)
	}
}

// TestURLNormalModeVimEdits exercises real Vim editing in the URL bar's NORMAL
// sub-mode: motions, delete, change-to-end, paste, and undo.
func TestURLNormalModeVimEdits(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m := step(base, runes("https://api.test/oldpath"))
	m = urlNormal(m) // drop to NORMAL (cursor on last char)

	// x deletes the char under the cursor (the trailing 'h').
	m = step(m, runes("x"))
	if got := m.url.Text(); got != "https://api.test/oldpat" {
		t.Fatalf("x = %q, want …oldpat", got)
	}

	// u undoes the delete.
	m = step(m, runes("u"))
	if got := m.url.Text(); got != "https://api.test/oldpath" {
		t.Fatalf("u = %q, want …oldpath restored", got)
	}

	// 0 to line start, then w/w to jump words; C changes to end of line.
	m = step(m, runes("0"))
	m = step(m, runes("C")) // delete to end, enter Insert
	if !m.urlInsert() {
		t.Fatal("C should enter Insert mode")
	}
	m = step(m, runes("https://new.test/ping"))
	if got := m.url.Text(); got != "https://new.test/ping" {
		t.Fatalf("C + retype = %q", got)
	}

	// Back to NORMAL; dd clears the line, then verify empty.
	m = urlNormal(m)
	m = step(m, runes("d"))
	m = step(m, runes("d"))
	if got := m.url.Text(); got != "" {
		t.Errorf("dd should clear the URL, got %q", got)
	}
}

// TestURLNormalWordDeleteAndPaste covers b/w word motion, dw, and p.
func TestURLNormalWordDeleteAndPaste(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m := urlNormal(step(base, runes("alpha beta")))

	// b moves back to the start of "beta"; dw deletes the word (register holds it).
	m = step(m, runes("b"))
	m = step(m, runes("d"))
	m = step(m, runes("w"))
	if got := m.url.Text(); got != "alpha " {
		t.Fatalf("dw = %q, want 'alpha '", got)
	}
	// p pastes the deleted "beta" back after the cursor. Single-line stays one line.
	m = step(m, runes("p"))
	if got := m.url.Text(); got != "alpha beta" {
		t.Errorf("p after dw = %q, want 'alpha beta'", got)
	}
}

func TestTimeoutEditableInline(t *testing.T) {
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})

	// Timeout is no longer a pane/tab stop: the ,t leader jumps straight to
	// editing the inline field, without moving focus off the URL bar. (URL
	// NORMAL mode itself is now pure Vim, so t is free for motions.)
	m := step(step(urlNormal(base), runes(",")), runes("t"))
	if !m.timeoutInput.Focused() || !m.editing() {
		t.Fatalf(",t should begin editing the timeout, focused=%v editing=%v",
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

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func assertViewFits(t *testing.T, m Model) {
	t.Helper()
	view := strings.TrimRight(stripANSI(m.View()), "\n")
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Fatalf("rendered height = %d, want <= %d", len(lines), m.height)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line %d rendered width = %d, want <= %d", i, w, m.width)
		}
	}
}

func clickAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
}

// TestSendButtonHitbox derives the SEND button's real on-screen position from
// the rendered view and checks the mouse hit-test agrees — guarding the offset
// for the method pane that precedes the URL bar.
func TestSendButtonHitbox(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	var x, y = -1, -1
	for i, ln := range strings.Split(m.View(), "\n") {
		vis := stripANSI(ln)
		if idx := strings.Index(vis, "SEND"); idx >= 0 {
			x = lipgloss.Width(vis[:idx]) // display column, not byte offset
			y = i
			break
		}
	}
	if x < 0 {
		t.Fatal("SEND not found in rendered view")
	}
	if !m.sendButtonClicked(clickAt(x, y)) {
		t.Errorf("click on SEND at (%d,%d) should hit the button", x, y)
	}
	if m.sendButtonClicked(clickAt(x-25, y)) {
		t.Error("a click well left of the button (over the URL input) should miss")
	}
	if m.sendButtonClicked(clickAt(x, y+1)) {
		t.Error("a click on the wrong row should miss")
	}
}

// findInView returns the display column and row of the first occurrence of
// substr in m's rendered view (ANSI stripped), or (-1, -1) if absent. Mouse
// tests derive click coordinates from the real render rather than hardcoding.
func findInView(m Model, substr string) (int, int) {
	for y, ln := range strings.Split(m.View(), "\n") {
		vis := stripANSI(ln)
		if idx := strings.Index(vis, substr); idx >= 0 {
			return lipgloss.Width(vis[:idx]), y
		}
	}
	return -1, -1
}

func TestClickFocusesMethodAndCycles(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	x, y := findInView(m, "GET")
	if x < 0 {
		t.Fatal("method label not found in view")
	}
	mm := clickModel(t, m, x, y)
	if mm.focus != focusMethod {
		t.Errorf("clicking the method pane should focus it, got %v", mm.focus)
	}
	if mm.req.Method != "POST" {
		t.Errorf("clicking the method should advance GET→POST, got %s", mm.req.Method)
	}
}

func TestClickPlacesURLCursor(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText("https://zzz.test/x") // short enough to fit, so scroll start = 0
	x, y := findInView(m, "https://zzz.test/x")
	if x < 0 {
		t.Fatal("URL text not found in view")
	}
	mm := clickModel(t, m, x+8, y) // click on the 9th character (index 8)
	if mm.focus != focusURL || !mm.urlInsert() {
		t.Errorf("clicking the URL should focus + Insert, focus=%v insert=%v", mm.focus, mm.urlInsert())
	}
	if _, col := mm.url.Cursor(); col != 8 {
		t.Errorf("clicked caret column = %d, want 8", col)
	}
}

func TestClickSwitchesRequestTab(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	x, y := findInView(m, "Auth")
	if x < 0 {
		t.Fatal("Auth tab not found in view")
	}
	mm := clickModel(t, m, x, y)
	if mm.focus != focusRequest {
		t.Errorf("clicking a tab should focus the request pane, got %v", mm.focus)
	}
	if mm.reqPane.tab != tabAuth {
		t.Errorf("clicking the Auth tab should select it, tab=%d want %d", mm.reqPane.tab, tabAuth)
	}
}

func TestClickSelectsCollectionRow(t *testing.T) {
	m, _ := seededModel(t)
	x, y := findInView(m, "seed")
	if x < 0 {
		t.Fatal("seed item not found in view")
	}
	mm := clickModel(t, m, x, y)
	if mm.focus != focusCollection {
		t.Errorf("clicking the tree should focus collections, got %v", mm.focus)
	}
	if sel, ok := mm.collectionPane.selected(); !ok || sel.Name != "seed" {
		t.Errorf("clicking the seed row should select it, got %q (ok=%v)", sel.Name, ok)
	}
}

func TestDoubleClickOpensTreeItem(t *testing.T) {
	m, _ := seededModel(t)
	x, y := findInView(m, "seed")
	if x < 0 {
		t.Fatal("seed item not found in view")
	}
	m = clickModel(t, m, x, y) // first click: select only
	if m.currentName == "seed" {
		t.Fatal("a single click must not open the request")
	}
	m = clickModel(t, m, x, y) // second click on the same row: open
	if m.currentName != "seed" {
		t.Errorf("double-click should open the seed request, currentName=%q", m.currentName)
	}
}

func TestClickIgnoredWhileModalOpen(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	x, y := findInView(m, "GET")
	m.showHelp = true
	got, cmd := m.handleClick(clickAt(x, y))
	if got.(Model).focus != focusURL || got.(Model).req.Method != "GET" || cmd != nil {
		t.Error("a click while help is open must be ignored")
	}
}

// clickModel dispatches a left-click and returns the resulting Model.
func clickModel(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	got, _ := m.handleClick(clickAt(x, y))
	return got.(Model)
}

func wheelAt(x, y int, btn tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: btn, Action: tea.MouseActionPress}
}

// responsePanePoint returns a coordinate known to fall inside the response pane.
func responsePanePoint(m Model) (int, int) {
	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	respX := rightX + l.reqInnerW + borderOverhead + l.gap
	return respX + 2, m.bodyTopY() + 2
}

func TestMouseWheelScrollsResponse(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.hasResp = true
	m.vp.SetContent(strings.Repeat("body line\n", 200)) // taller than the viewport
	x, y := responsePanePoint(m)

	// Wheel down over the response pane scrolls it down.
	down, _ := m.Update(wheelAt(x, y, tea.MouseButtonWheelDown))
	m = down.(Model)
	if m.vp.YOffset == 0 {
		t.Fatalf("wheel down over the response pane should scroll it, YOffset=%d", m.vp.YOffset)
	}
	scrolled := m.vp.YOffset

	// Wheel up scrolls back toward the top.
	up, _ := m.Update(wheelAt(x, y, tea.MouseButtonWheelUp))
	if up.(Model).vp.YOffset >= scrolled {
		t.Errorf("wheel up should scroll back up, YOffset=%d (was %d)", up.(Model).vp.YOffset, scrolled)
	}
}

func TestSelectedTextExtraction(t *testing.T) {
	text := "abcdefghij\nklmnop\nqrstuv"
	// Single line, mid-span.
	if got := selectedText(text, textPos{0, 2}, textPos{0, 5}); got != "cde" {
		t.Errorf("single-line select = %q, want cde", got)
	}
	// Reversed anchor/cursor normalizes.
	if got := selectedText(text, textPos{0, 5}, textPos{0, 2}); got != "cde" {
		t.Errorf("reversed select = %q, want cde", got)
	}
	// Multi-line: from line0 col8 through line2 col4.
	if got := selectedText(text, textPos{0, 8}, textPos{2, 4}); got != "ij\nklmnop\nqrst" {
		t.Errorf("multi-line select = %q", got)
	}
}

func TestRenderWithSelectionPreservesText(t *testing.T) {
	// Highlighting must never change the underlying characters, only their
	// styling (which lipgloss strips in this non-TTY test env). Span correctness
	// is covered by TestSelectedTextExtraction.
	text := "abcdefghij\nklmnop"
	for _, span := range [][2]textPos{
		{{0, 2}, {0, 5}}, // single line
		{{0, 8}, {1, 3}}, // across the line break
		{{1, 0}, {1, 6}}, // whole second line
	} {
		if got := stripANSI(renderWithSelection(text, span[0], span[1])); got != text {
			t.Errorf("selection %v corrupted the text: %q", span, got)
		}
	}
}

func TestDragSelectCopiesResponse(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.hasResp = true
	m.respText = "abcdefghij\nklmno"
	m.vp.SetContent(m.currentResponseText())

	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	contentX := rightX + l.reqInnerW + borderOverhead + l.gap + 2
	vpTopY := m.bodyTopY() + 1 + 1

	press := tea.MouseMsg{X: contentX, Y: vpTopY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	md, _ := m.Update(press)
	m = md.(Model)
	if !m.selecting {
		t.Fatal("press over the response body should start selecting")
	}

	drag := tea.MouseMsg{X: contentX + 5, Y: vpTopY, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion}
	mg, _ := m.Update(drag)
	m = mg.(Model)
	if !m.selDragged {
		t.Fatal("motion should mark the selection as a drag")
	}
	if got := selectedText(m.currentResponseText(), m.selAnchor, m.selCursor); got != "abcde" {
		t.Errorf("drag selection = %q, want abcde", got)
	}

	up := tea.MouseMsg{X: contentX + 5, Y: vpTopY, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
	mu, _ := m.Update(up)
	m = mu.(Model)
	if m.selecting {
		t.Error("release should end the selection")
	}
	// Either the copy succeeded or the clipboard was unavailable — both leave a
	// status message, proving the copy path ran on a non-empty selection.
	if !strings.Contains(m.statusMsg, "copied") && !strings.Contains(m.statusMsg, "clipboard") {
		t.Errorf("release after a drag should report a copy result, got %q", m.statusMsg)
	}
}

func TestResponseContentDoesNotExpandLayout(t *testing.T) {
	cases := []struct {
		name string
		w, h int
		resp model.Response
	}{
		{
			name: "long single line body",
			w:    120,
			h:    40,
			resp: model.Response{Status: "200 OK", StatusCode: 200, Body: []byte(strings.Repeat("0123456789", 1000))},
		},
		{
			name: "long error",
			w:    100,
			h:    30,
			resp: model.Response{Err: errors.New(strings.Repeat("network failure ", 80))},
		},
		{
			name: "narrow pretty json",
			w:    70,
			h:    24,
			resp: model.Response{Status: "200 OK", StatusCode: 200, Body: []byte(`{"items":[{"name":"` + strings.Repeat("x", 300) + `"}]}`)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := step(New(), tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
			m.hasResp = true
			m.resp = tc.resp
			m.respText = renderResponseBody(m.resp, m.vp.Width, false)
			m.vp.SetContent(m.currentResponseText())
			assertViewFits(t, m)
		})
	}
}

func TestMouseWheelIgnoredOutsideResponse(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.hasResp = true
	m.vp.SetContent(strings.Repeat("body line\n", 200))
	// A point over the request pane (left of the response pane) must not scroll.
	l := m.computeLayout()
	rightX := 0
	if m.collectionShown {
		rightX = l.collectionInnerW + borderOverhead + l.gap
	}
	reqX, y := rightX+2, m.bodyTopY()+2
	got, _ := m.Update(wheelAt(reqX, y, tea.MouseButtonWheelDown))
	if got.(Model).vp.YOffset != 0 {
		t.Errorf("wheel over the request pane must not scroll the response, YOffset=%d", got.(Model).vp.YOffset)
	}
}

// wheelSuppressModel focuses the tree with the cursor mid-list, over a
// scrollable response. A suppressed vs. real vertical-nav key is then observable
// as tree-cursor movement — the method pane no longer reacts to j/k/arrows.
func wheelSuppressModel(t *testing.T) Model {
	t.Helper()
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = collections.Store{Root: t.TempDir()}
	for _, n := range []string{"one", "two", "three", "four"} {
		if err := m.collectionStore.Save(n, model.Request{Method: "GET", URL: "https://x.test"}); err != nil {
			t.Fatal(err)
		}
	}
	m.refreshCollections()
	m = m.setFocus(focusCollection)
	m.collectionPane.cursor = 1 // mid-list: a real up or down key would move it
	m.hasResp = true
	m.vp.SetContent(strings.Repeat("body line\n", 200))
	return m
}

func TestMouseWheelOverResponseSuppressesSyntheticArrows(t *testing.T) {
	m := wheelSuppressModel(t)
	x, y := responsePanePoint(m)

	// Some terminals emit arrow keys after a wheel event, and the count per notch
	// is terminal-dependent. None of those synthetic Downs may leak into the
	// focused tree and move its cursor.
	wm, _ := m.Update(wheelAt(x, y, tea.MouseButtonWheelDown))
	m = wm.(Model)
	if m.collectionPane.cursor != 1 {
		t.Fatalf("wheel itself should not move the tree cursor, cursor=%d", m.collectionPane.cursor)
	}
	// Fire more synthetic arrows than mouseScrollLines to prove the suppression
	// isn't a fixed count that lets the tail leak through.
	for i := 0; i < mouseScrollLines+3; i++ {
		km, _ := m.Update(keyDown)
		m = km.(Model)
	}
	if m.collectionPane.cursor != 1 {
		t.Errorf("synthetic Downs after a wheel must all be suppressed, cursor=%d", m.collectionPane.cursor)
	}
}

func TestMouseWheelOverResponseSuppressesOppositeSyntheticArrows(t *testing.T) {
	m := wheelSuppressModel(t)
	x, y := responsePanePoint(m)

	wm, _ := m.Update(wheelAt(x, y, tea.MouseButtonWheelDown))
	m = wm.(Model)
	km, _ := m.Update(keyUp)
	m = km.(Model)
	if m.collectionPane.cursor != 1 {
		t.Errorf("opposite vertical arrows during a wheel burst must be suppressed, cursor=%d", m.collectionPane.cursor)
	}
}

func TestMouseWheelOverResponseSuppressesSyntheticJK(t *testing.T) {
	m := wheelSuppressModel(t)
	x, y := responsePanePoint(m)

	wm, _ := m.Update(wheelAt(x, y, tea.MouseButtonWheelDown))
	m = wm.(Model)
	km, _ := m.Update(runes("j"))
	m = km.(Model)
	if m.collectionPane.cursor != 1 {
		t.Errorf("synthetic j during a wheel burst must be suppressed, cursor=%d", m.collectionPane.cursor)
	}
}

func TestWheelArrowSuppressionExpires(t *testing.T) {
	m := wheelSuppressModel(t)
	x, y := responsePanePoint(m)

	wm, _ := m.Update(wheelAt(x, y, tea.MouseButtonWheelDown))
	m = wm.(Model)
	// Simulate the suppression window having elapsed: a later, intentional j
	// press must reach the tree and move the cursor.
	m.wheel.armedAt = m.wheel.armedAt.Add(-2 * wheelArrowWindow)
	km, _ := m.Update(runes("j"))
	m = km.(Model)
	if m.collectionPane.cursor == 1 {
		t.Error("a real j press after the window should not be suppressed")
	}
}

func TestWheelArrowSuppressionStopsOnOtherKey(t *testing.T) {
	m := wheelSuppressModel(t)
	x, y := responsePanePoint(m)

	wm, _ := m.Update(wheelAt(x, y, tea.MouseButtonWheelDown))
	m = wm.(Model)
	// A non-matching key within the window ends suppression, so a following j is
	// treated as a real press and moves the cursor.
	zm, _ := m.Update(runes("z")) // inert in the tree, but ends the wheel window
	m = zm.(Model)
	km, _ := m.Update(runes("j"))
	m = km.(Model)
	if m.collectionPane.cursor == 1 {
		t.Error("after an intentional key, a later j should no longer be suppressed")
	}
}

func TestCollectionsAutoHideOnNarrow(t *testing.T) {
	wide := step(New(), tea.WindowSizeMsg{Width: 120, Height: 24})
	if !wide.collectionShown {
		t.Fatal("tree should show on a wide terminal")
	}
	// Narrowing hides the tree but preserves the user's preference.
	narrow := step(wide, tea.WindowSizeMsg{Width: 50, Height: 24})
	if narrow.collectionShown {
		t.Error("tree should auto-hide on a narrow terminal")
	}
	if !narrow.collectionPref {
		t.Error("the show preference should be preserved while auto-hidden")
	}
	// Widening restores it.
	if back := step(narrow, tea.WindowSizeMsg{Width: 120, Height: 24}); !back.collectionShown {
		t.Error("tree should return when the terminal widens again")
	}
}

func TestFocusLeavesTreeWhenAutoHidden(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 24}).setFocus(focusCollection)
	m = step(m, tea.WindowSizeMsg{Width: 50, Height: 24})
	if m.focus == focusCollection {
		t.Error("focus must leave the tree when it auto-hides, not strand on an invisible pane")
	}
}

func TestManualHideSurvivesResize(t *testing.T) {
	// A user who hides the tree keeps it hidden even after widening.
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 24})
	m = m.toggleCollectionPane() // hide
	if m.collectionShown || m.collectionPref {
		t.Fatal("toggle should hide the tree and clear the preference")
	}
	if back := step(m, tea.WindowSizeMsg{Width: 130, Height: 24}); back.collectionShown {
		t.Error("a manually hidden tree should stay hidden on resize")
	}
}

func TestTimeoutReadoutHiddenWhenNarrow(t *testing.T) {
	topBar := func(v string) string {
		ls := strings.Split(v, "\n")
		if len(ls) > 5 { // the method/URL bar now sits below the tabline row
			ls = ls[:5]
		}
		return stripANSI(strings.Join(ls, "\n"))
	}
	wide := step(New(), tea.WindowSizeMsg{Width: 120, Height: 24})
	if !strings.Contains(topBar(wide.View()), "timeout") {
		t.Error("wide terminal should show the inline timeout readout")
	}
	narrow := step(New(), tea.WindowSizeMsg{Width: 46, Height: 24})
	if strings.Contains(topBar(narrow.View()), "timeout") {
		t.Error("narrow terminal should drop the inline timeout readout to protect the URL input")
	}
}

func TestTruncateMiddle(t *testing.T) {
	if got := truncateMiddle("short", 28); got != "short" {
		t.Errorf("short name should pass through, got %q", got)
	}
	got := truncateMiddle("auth/very/long/path/to/the/login", 20)
	if n := len([]rune(got)); n != 20 || !strings.Contains(got, "…") {
		t.Errorf("long name should be middle-truncated to 20 with an ellipsis, got %q (%d)", got, n)
	}
}

func TestImportCurlCommand(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	tm, _ := m.executeCommand(`import curl -X POST https://api.test/login -H 'Accept: application/json' -d '{"u":1}'`)
	m = tm.(Model)
	if m.req.Method != "POST" || m.url.Text() != "https://api.test/login" {
		t.Errorf("imported method/url = %q / %q", m.req.Method, m.url.Text())
	}
	built := m.buildRequest()
	if built.Body != `{"u":1}` {
		t.Errorf("imported body = %q", built.Body)
	}
	if hs := m.reqPane.headersOut(); len(hs) != 1 || hs[0].Name != "Accept" {
		t.Errorf("imported headers = %+v", hs)
	}
	if !strings.Contains(m.statusMsg, "imported") {
		t.Errorf("status = %q", m.statusMsg)
	}
	if !m.dirty() {
		t.Error("an imported unnamed request should be dirty until saved")
	}
}

func TestImportCurlGuardsUnsavedEdits(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = step(m, runes("https://existing.test")) // dirty the buffer

	tm, _ := m.executeCommand("import curl https://new.test")
	m = tm.(Model)
	if m.pendingAction != pendingImportCurl {
		t.Fatalf("a dirty import should arm the save prompt, got pendingAction=%v", m.pendingAction)
	}
	if m.url.Text() != "https://existing.test" {
		t.Errorf("import must not apply before the prompt is resolved, url=%q", m.url.Text())
	}
	// 'n' discards the edits and performs the deferred import.
	tm, _ = m.resolveSaveConfirm(runes("n"))
	nm := tm.(Model)
	if got := nm.url.Text(); got != "https://new.test" {
		t.Errorf("after discarding, the import should apply; url=%q", got)
	}
}

func TestImportCurlBadInput(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	tm, _ := m.executeCommand("import curl -X POST -H 'a: b'") // no URL
	m = tm.(Model)
	if !strings.Contains(m.statusMsg, "failed") || m.pendingAction != pendingNone {
		t.Errorf("a URL-less curl should fail cleanly without arming a prompt, status=%q pending=%v", m.statusMsg, m.pendingAction)
	}

	tm, _ = m.executeCommand("import curlfoo https://example.test")
	m = tm.(Model)
	if !strings.Contains(m.statusMsg, "usage") {
		t.Errorf("import curlfoo should be rejected as usage error, status=%q", m.statusMsg)
	}
}

func TestCopyCurlCommandRoutes(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText("https://api.test/x")
	tm, _ := m.executeCommand("copy curl")
	// Clipboard availability varies by environment; either way a status is set
	// and the command must not fall through to the ":copy old new" usage error.
	if s := tm.(Model).statusMsg; s == "" || strings.Contains(s, "usage") {
		t.Errorf(":copy curl should route to the exporter, status=%q", s)
	}
}

func TestRawPrettyToggle(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText("https://x.test")
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
	if m.focus != focusRequest || m.reqPane.tab != tabAuth {
		t.Fatalf("[ from URL normal focus = focus %v tab %d, want Request/Auth (last tab)", m.focus, m.reqPane.tab)
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

func TestSendCommandGoesInFlight(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText("https://example.test")
	m2, cmd := m.executeCommand("send")
	if !m2.(Model).sending {
		t.Error(":send should put the model in-flight")
	}
	if cmd == nil {
		t.Error(":send should return a command")
	}
}

// TestEnterDoesNotSend guards the decision that the Enter key never fires a
// request — use :send or the SEND button instead.
func TestEnterDoesNotSend(t *testing.T) {
	// From the URL bar in both Insert and NORMAL sub-modes, and from the method
	// pane, Enter must not start a request.
	base := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	base.url.SetText("https://example.test")

	for _, tc := range []struct {
		name string
		m    Model
	}{
		{"url insert", base},
		{"url normal", urlNormal(base)},
		{"method pane", base.setFocus(focusMethod)},
	} {
		if got := step(tc.m, keyEnter); got.sending {
			t.Errorf("%s: Enter must not send", tc.name)
		}
	}
}

func TestSendWarnsOnEmptyURL(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText("   ") // whitespace-only counts as empty
	m = sendNow(m)
	if m.sending {
		t.Fatal(":send with an empty URL must not start a request")
	}
	if !strings.Contains(m.statusMsg, "URL is empty") {
		t.Errorf("statusMsg = %q, want it to say the URL is empty", m.statusMsg)
	}
}

func TestSendWarnsOnUnresolvedVars(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText("https://{{host}}/ping")
	m = sendNow(m)
	if !m.sending {
		t.Fatal(":send should still send even with unresolved vars")
	}
	if !strings.Contains(m.statusMsg, "{{host}}") {
		t.Errorf("statusMsg = %q, want it to warn about {{host}}", m.statusMsg)
	}

	// A fully-resolved send leaves no unresolved-var warning.
	m2 := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m2.url.SetText("https://example.test/ping")
	m2 = sendNow(m2)
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
	m = sendNow(m)
	if !strings.Contains(m.statusMsg, "{{tok}}") {
		t.Errorf("statusMsg = %q, want warning about {{tok}} in the query", m.statusMsg)
	}
}

func TestWriteQuitBlockedWhenSaveFails(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = failingStore(t)
	m.currentName = "api/thing"
	m.url.SetText("https://example.test")

	_, cmd := m.executeCommand("wq")
	if isQuit(cmd) {
		t.Error(":wq must not quit when the save fails — that would lose edits")
	}
}

func TestSavePromptKeepsWorkWhenSaveFails(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = failingStore(t)
	m.currentName = "api/thing"
	m.url.SetText("https://changed.test") // make it dirty so quitting arms the prompt

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
	m.url.SetText("https://keep.test")
	m = m.newSavedRequest("api/new")
	if m.currentName == "api/new" {
		t.Error("currentName must not be set when the create/save failed")
	}
	if m.url.Text() != "https://keep.test" {
		t.Errorf("failed create must not blank the editor, url=%q", m.url.Text())
	}
	if !strings.Contains(m.statusMsg, "failed") {
		t.Errorf("statusMsg = %q, want a failure message", m.statusMsg)
	}
}

func TestEscCancelsInFlight(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.url.SetText("https://example.test")
	m = sendNow(m)
	if !m.sending {
		t.Fatal(":send should put the model in-flight")
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
	// precedes URL in reading order). Only r cycles the method — the arrow keys,
	// j/k and space are all intentionally inert.
	m = step(m, keyShiftTab)
	if m.focus != focusMethod {
		t.Fatalf("shift+tab from URL: focus = %v, want Method", m.focus)
	}
	for _, msg := range []tea.KeyMsg{keyDown, keyUp, runes("j"), runes("k"), runes(" ")} {
		m = step(m, msg)
		if m.req.Method != "GET" {
			t.Errorf("%v must not change the method, got %q", msg, m.req.Method)
		}
	}
	m = step(m, runes("r"))
	if m.req.Method != "POST" {
		t.Errorf("after r, method = %q, want POST", m.req.Method)
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
