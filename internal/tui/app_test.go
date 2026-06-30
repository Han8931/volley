package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/model"
)

// step applies a message and returns the concrete Model for chaining.
func step(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// runes builds a rune key message ("o", "X-Test", …).
func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
)

var (
	keyCtrlW = tea.KeyMsg{Type: tea.KeyCtrlW}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
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

	// ctrl+w h and left arrow from the method/URL bar go directly to the tree.
	if got := step(step(base, keyCtrlW), runes("h")).focus; got != focusCollection {
		t.Errorf("ctrl+w h from URL: focus = %v, want Collections", got)
	}
	if got := step(base, tea.KeyMsg{Type: tea.KeyLeft}).focus; got != focusCollection {
		t.Errorf("left arrow from URL: focus = %v, want Collections", got)
	}

	// Arrow down from URL also reaches the collections tree.
	if got := step(base, keyDown).focus; got != focusCollection {
		t.Errorf("down arrow: focus = %v, want Collections", got)
	}
	// Tab cycles URL -> Collections.
	if got := step(base, keyTab).focus; got != focusCollection {
		t.Errorf("tab: focus = %v, want Collections", got)
	}
	// Plain j from the URL bar drops into the request pane for quick editing.
	if got := step(base, runes("j")).focus; got != focusRequest {
		t.Errorf("j from URL: focus = %v, want Request", got)
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

	m = step(m, runes("]"))
	if m.focus != focusRequest || m.reqPane.tab != tabBody {
		t.Fatalf("] from URL focus = focus %v tab %d, want Request/Body", m.focus, m.reqPane.tab)
	}

	m = step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = step(m, runes("["))
	if m.focus != focusRequest || m.reqPane.tab != tabQuery {
		t.Fatalf("[ from URL focus = focus %v tab %d, want Request/Query", m.focus, m.reqPane.tab)
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

	if out := m.View(); !strings.Contains(out, "NORMAL") {
		t.Error("status bar should show NORMAL mode")
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
	m = step(m, runes("l"))
	if m.req.Method != "POST" {
		t.Errorf("after l, method = %q, want POST", m.req.Method)
	}
	m = step(m, runes("h"))
	if m.req.Method != "GET" {
		t.Errorf("after h, method = %q, want GET", m.req.Method)
	}
	m = step(m, runes("m"))
	if m.req.Method != "POST" {
		t.Errorf("after m, method = %q, want POST", m.req.Method)
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
