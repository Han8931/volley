package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/loadtest"
	"github.com/tabularasa/volley/internal/model"
)

func sized() Model {
	return step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
}

func TestCommandSetMethod(t *testing.T) {
	m := sized()
	m = step(m, runes(":"))
	if !m.cmdActive || m.cmdKind != ':' {
		t.Fatal("\":\" should open the command line")
	}
	m = step(m, runes("method post"))
	m = step(m, keyEnter)
	if m.cmdActive {
		t.Error("command line should close on enter")
	}
	if m.req.Method != "POST" {
		t.Errorf("method = %q, want POST", m.req.Method)
	}
}

func TestEditorCommandRequiresEditorEnv(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	m := sized()
	next, cmd := m.executeCommand("editor")
	m = next.(Model)
	if cmd != nil {
		t.Fatal(":editor without env should not spawn a command")
	}
	if !strings.Contains(m.statusMsg, "VISUAL") || !strings.Contains(m.statusMsg, "EDITOR") {
		t.Fatalf("status = %q, want editor env hint", m.statusMsg)
	}
}

func TestEditorRequestRoundTrip(t *testing.T) {
	m := sized()
	m.req = model.Request{Method: "POST", URL: "https://old.test", Body: "old", Timeout: 2 * time.Second}
	m.url.SetText(m.req.URL)
	m.timeout = m.req.Timeout
	m.reqPane.setBodyText(m.req.Body)
	initial, err := editorInitialContent(m.rawRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initial, `"method": "POST"`) || !strings.Contains(initial, `"timeout": "2s"`) {
		t.Fatalf("request editor JSON missing fields:\n%s", initial)
	}
	// A request with no auth helper must not emit an "auth" block, matching the
	// on-disk storage shape.
	if strings.Contains(initial, `"auth"`) {
		t.Fatalf("no-auth request should omit the auth block:\n%s", initial)
	}

	path := t.TempDir() + "/request.txt"
	edited := `{
  "method": "put",
  "url": "https://new.test",
  "headers": [{"name":"X-Test","value":"yes","enabled":true}],
  "query": [{"key":"q","value":"1","enabled":true}],
  "timeout": "5s"
}

` + editorBodyMarker + `
edited body`
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	base := m.baseline
	next, _ := m.applyEditorResult(editorFinishedMsg{path: path})
	m = next.(Model)
	if m.req.Method != "PUT" || m.url.Text() != "https://new.test" || m.reqPane.bodyOut() != "edited body" || m.timeout != 5*time.Second {
		t.Fatalf("request not applied: method=%q url=%q body=%q timeout=%s", m.req.Method, m.url.Text(), m.reqPane.bodyOut(), m.timeout)
	}
	gotH := m.reqPane.headersOut()
	if len(gotH) != 1 || gotH[0] != (model.Header{Name: "X-Test", Value: "yes", Enabled: true}) {
		t.Fatalf("header did not round-trip through the editor: %+v", gotH)
	}
	gotQ := m.reqPane.queryOut()
	if len(gotQ) != 1 || gotQ[0] != (model.KV{Key: "q", Value: "1", Enabled: true}) {
		t.Fatalf("query did not round-trip through the editor: %+v", gotQ)
	}
	if !requestsEqual(m.baseline, base) || !m.dirty() {
		t.Fatal("request editor changes should preserve baseline and mark request dirty")
	}
}

// TestEditorAuthRoundTrip covers the pointer-based auth block: a request with a
// Bearer helper must emit a lowercase auth block and parse cleanly back.
func TestEditorAuthRoundTrip(t *testing.T) {
	req := model.Request{
		Method: "GET",
		URL:    "https://api.test",
		Auth:   model.Auth{Type: model.AuthBearer, Token: "secret"},
	}
	initial, err := editorInitialContent(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initial, `"type": "bearer"`) || !strings.Contains(initial, `"token": "secret"`) {
		t.Fatalf("auth block missing or mis-cased:\n%s", initial)
	}
	back, err := parseEditedRequest([]byte(initial))
	if err != nil {
		t.Fatal(err)
	}
	if back.Auth != req.Auth {
		t.Fatalf("auth did not round-trip: got %+v want %+v", back.Auth, req.Auth)
	}
}

// TestEditorMultilineBodyRoundTrip is the whole point of the raw-body section:
// a multi-line payload must appear verbatim in the editor document (not
// flattened into one escaped JSON string) and survive the round-trip intact.
func TestEditorMultilineBodyRoundTrip(t *testing.T) {
	body := "{\n  \"name\": \"alice\",\n  \"nested\": {\n    \"a\": 1\n  }\n}"
	req := model.Request{Method: "POST", URL: "https://api.test", Body: body}
	initial, err := editorInitialContent(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initial, body) {
		t.Fatalf("body should appear verbatim in the editor document:\n%s", initial)
	}
	if strings.Contains(initial, `\n`) {
		t.Fatalf("body must not be JSON-escaped onto one line:\n%s", initial)
	}
	back, err := parseEditedRequest([]byte(initial))
	if err != nil {
		t.Fatal(err)
	}
	if back.Body != body {
		t.Fatalf("body did not round-trip:\ngot  %q\nwant %q", back.Body, body)
	}
}

// TestEditorMissingBodyMarkerIsLenient: if the user deletes the marker line, the
// document is treated as metadata-only with an empty body rather than erroring.
func TestEditorMissingBodyMarkerIsLenient(t *testing.T) {
	req, err := parseEditedRequest([]byte(`{"method":"GET","url":"https://x.test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Body != "" || req.URL != "https://x.test" {
		t.Fatalf("metadata-only edit: body=%q url=%q", req.Body, req.URL)
	}
}

func TestEditorNamedRequestSavesAndLoadsTarget(t *testing.T) {
	m := sized()
	m.collectionStore.Root = t.TempDir()
	if err := m.collectionStore.Save("target", model.Request{Method: "GET", URL: "https://old.test"}); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/request.txt"
	edited := "{\"method\":\"POST\",\"url\":\"https://target.test\"}\n\n" + editorBodyMarker + "\nsaved"
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	next, _ := m.applyEditorResult(editorFinishedMsg{path: path, targetName: "target"})
	m = next.(Model)
	if m.currentName != "target" || m.req.Method != "POST" || m.url.Text() != "https://target.test" || m.reqPane.bodyOut() != "saved" {
		t.Fatalf("named editor should save+load target, current=%q method=%q url=%q body=%q", m.currentName, m.req.Method, m.url.Text(), m.reqPane.bodyOut())
	}
	if m.dirty() {
		t.Fatal("saved target should be clean after editor result")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file should be removed, stat err=%v", err)
	}
}

// TestEditorRejectsBadContent covers the failure branches of applyEditorResult:
// a garbled buffer, an empty buffer, and an unknown method must all be reported
// without mutating the in-progress request, and the temp file is still removed.
func TestEditorRejectsBadContent(t *testing.T) {
	for _, tc := range []struct {
		name, content, wantStatus string
	}{
		{"invalid json", `{not json`, "parse failed"},
		{"empty file", ``, "parse failed"},
		{"unknown method", `{"method":"FLY","url":"https://x.test"}`, "parse failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sized()
			m.req = model.Request{Method: "GET", URL: "https://keep.test"}
			m.url.SetText(m.req.URL)
			path := t.TempDir() + "/request.json"
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			next, _ := m.applyEditorResult(editorFinishedMsg{path: path})
			m = next.(Model)
			if !strings.Contains(m.statusMsg, tc.wantStatus) {
				t.Fatalf("status = %q, want it to contain %q", m.statusMsg, tc.wantStatus)
			}
			if m.req.Method != "GET" || m.url.Text() != "https://keep.test" {
				t.Fatalf("a rejected edit must not touch the request: method=%q url=%q", m.req.Method, m.url.Text())
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("temp file should be removed even on failure, stat err=%v", err)
			}
		})
	}
}

// TestEditorProcessErrorIsReported covers the editor exiting non-zero: the
// request is left untouched, the error is surfaced, and the temp file cleaned up.
func TestEditorProcessErrorIsReported(t *testing.T) {
	m := sized()
	m.req = model.Request{Method: "GET", URL: "https://keep.test"}
	m.url.SetText(m.req.URL)
	path := t.TempDir() + "/request.json"
	if err := os.WriteFile(path, []byte(`{"method":"POST"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	next, _ := m.applyEditorResult(editorFinishedMsg{path: path, err: fmt.Errorf("exit status 1")})
	m = next.(Model)
	if !strings.Contains(m.statusMsg, "editor failed") {
		t.Fatalf("status = %q, want editor-failed report", m.statusMsg)
	}
	if m.req.Method != "GET" {
		t.Fatalf("a failed editor must not touch the request, method=%q", m.req.Method)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file should be removed after a failed editor, stat err=%v", err)
	}
}

func TestCommandUnknown(t *testing.T) {
	m := sized()
	m = step(m, runes(":"))
	m = step(m, runes("bogus"))
	m = step(m, keyEnter)
	if !strings.Contains(m.statusMsg, "unknown command") {
		t.Errorf("statusMsg = %q, want unknown command", m.statusMsg)
	}
}

func TestCommandEscCancels(t *testing.T) {
	m := sized()
	m = step(m, runes(":"))
	m = step(m, keyEsc)
	if m.cmdActive {
		t.Error("esc should cancel the command line")
	}
}

func TestLoadEditTabCompletion(t *testing.T) {
	m := sized()
	m.loadStore.Root = t.TempDir()
	for _, name := range []string{"spike-fast", "spike-slow", "steady"} {
		if err := m.loadStore.Save(name, loadtest.Constant(1, time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	m = m.openCommandLineWith(':', "loadedit ste")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "loadedit steady" {
		t.Errorf("unique completion = %q, want %q", got, "loadedit steady")
	}

	m = m.openCommandLineWith(':', "loadedit spi")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "loadedit spike-" {
		t.Errorf("common-prefix completion = %q, want %q", got, "loadedit spike-")
	}
	if !strings.Contains(m.cmdHint, "spike-fast") || !strings.Contains(m.cmdHint, "spike-slow") {
		t.Errorf("ambiguous completion should list matches, hint = %q", m.cmdHint)
	}
}

func TestCommandVerbTabCompletion(t *testing.T) {
	m := sized()
	m = m.openCommandLineWith(':', "sen")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "send " {
		t.Errorf("unique verb completion = %q, want %q", got, "send ")
	}

	m = m.openCommandLineWith(':', "loade")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "loadedit" {
		t.Errorf("ambiguous verb completion = %q, want %q", got, "loadedit")
	}
	if !strings.Contains(m.cmdHint, "loadedit") || !strings.Contains(m.cmdHint, "loadeditor") {
		t.Errorf("ambiguous verb completion should list matches, hint = %q", m.cmdHint)
	}
}

func TestSavedRequestTabCompletion(t *testing.T) {
	m := sized()
	m.collectionStore = collections.Store{Root: t.TempDir()}
	for _, name := range []string{"auth/login", "auth/logout", "ping"} {
		if err := m.collectionStore.Save(name, model.NewRequest()); err != nil {
			t.Fatal(err)
		}
	}

	m = m.openCommandLineWith(':', "open p")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "open ping" {
		t.Errorf("request completion = %q, want %q", got, "open ping")
	}

	// A group completes with a trailing slash, and another Tab descends into it
	// instead of stalling on the group name itself.
	m = m.openCommandLineWith(':', "open au")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "open auth/" {
		t.Errorf("group completion = %q, want %q", got, "open auth/")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "open auth/log" {
		t.Errorf("descend completion = %q, want %q", got, "open auth/log")
	}
	if !strings.Contains(m.cmdHint, "auth/login") || !strings.Contains(m.cmdHint, "auth/logout") {
		t.Errorf("descend completion should list children, hint = %q", m.cmdHint)
	}
	// Typing any other key drops the stale candidate listing.
	m = step(m, runes("i"))
	if m.cmdHint != "" {
		t.Errorf("hint should clear on typing, got %q", m.cmdHint)
	}
}

func TestMethodTabCompletion(t *testing.T) {
	m := sized()
	m = m.openCommandLineWith(':', "method D")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "method DELETE" {
		t.Errorf("method completion = %q, want %q", got, "method DELETE")
	}
}

func TestTabCompletionIgnoresFreeTextArgs(t *testing.T) {
	m := sized()
	// ":set name=value" takes free text; Tab must leave it untouched.
	m = m.openCommandLineWith(':', "set tok")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.cmd.Value(); got != "set tok" {
		t.Errorf("free-text arg changed by Tab: %q", got)
	}
}

func TestLoadEditorCommand(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	m := sized()
	m.loadStore.Root = t.TempDir()
	if err := m.loadStore.Save("steady", loadtest.Constant(1, time.Second)); err != nil {
		t.Fatal(err)
	}

	next, cmd := m.executeCommand("loadeditor steady")
	m = next.(Model)
	if cmd != nil {
		t.Fatal(":loadeditor without editor env should not spawn a command")
	}
	if !strings.Contains(m.statusMsg, "VISUAL") || !strings.Contains(m.statusMsg, "EDITOR") {
		t.Fatalf("status = %q, want editor env hint", m.statusMsg)
	}

	next, _ = m.executeCommand("loadeditor nope")
	m = next.(Model)
	if !strings.Contains(m.statusMsg, "no load profile named nope") {
		t.Fatalf("status = %q, want missing-profile error", m.statusMsg)
	}

	t.Setenv("VISUAL", "true")
	next, cmd = m.executeCommand("loadeditor steady")
	m = next.(Model)
	if cmd == nil {
		t.Fatalf(":loadeditor with an editor should spawn it, status = %q", m.statusMsg)
	}
}

func TestCommandHistoryNavigationRestoresDraft(t *testing.T) {
	m := sized()
	for _, command := range []string{"method post", "timeout 7s"} {
		m = m.openCommandLineWith(':', command)
		m = step(m, keyEnter)
	}

	m = m.openCommandLineWith(':', "loadedit dra")
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.cmd.Value(); got != "timeout 7s" {
		t.Errorf("first up = %q", got)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.cmd.Value(); got != "method post" {
		t.Errorf("second up = %q", got)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.cmd.Value(); got != "timeout 7s" {
		t.Errorf("first down = %q", got)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.cmd.Value(); got != "loadedit dra" {
		t.Errorf("down should restore draft, got %q", got)
	}
}

func TestResponseSearch(t *testing.T) {
	m := sized()
	m = step(m, responseMsg{resp: model.Response{
		Status: "200 OK", StatusCode: 200,
		Body: []byte(`{"name":"volley","kind":"name-test"}`),
	}})
	m = m.setFocus(focusResponse)

	m = step(m, runes("/"))
	if !m.cmdActive || m.cmdKind != '/' {
		t.Fatal("\"/\" should open search in the response pane")
	}
	m = step(m, runes("name"))
	m = step(m, keyEnter)

	if len(m.searchHits) == 0 {
		t.Fatal("expected at least one search hit")
	}
	if !strings.HasPrefix(m.statusMsg, "match 1/") {
		t.Errorf("statusMsg = %q, want match 1/N", m.statusMsg)
	}

	// n cycles to the next match (pretty-printed body has name on 2 lines).
	prev := m.searchIdx
	m = step(m, runes("n"))
	if len(m.searchHits) > 1 && m.searchIdx == prev {
		t.Error("n should advance to the next match")
	}
}

func TestSearchNotFound(t *testing.T) {
	m := sized()
	m = step(m, responseMsg{resp: model.Response{Body: []byte(`hello`), StatusCode: 200}})
	m = m.setFocus(focusResponse)
	m = step(m, runes("/"))
	m = step(m, runes("zzz"))
	m = step(m, keyEnter)
	if !strings.Contains(m.statusMsg, "not found") {
		t.Errorf("statusMsg = %q, want not found", m.statusMsg)
	}
}

func TestSetVariableExpandsInRequest(t *testing.T) {
	m := sized()
	m = step(m, runes(":"))
	m = step(m, runes("set tok=secret"))
	m = step(m, keyEnter)

	m.url.SetText("https://x.test/{{tok}}")
	if got := m.buildRequest().URL; got != "https://x.test/secret" {
		t.Errorf("built URL = %q, want expanded", got)
	}
}

func TestTimeoutCommand(t *testing.T) {
	m := sized()
	m = step(m, runes(":"))
	m = step(m, runes("timeout 7s"))
	m = step(m, keyEnter)
	if m.timeout != 7*time.Second {
		t.Errorf("timeout = %v, want 7s", m.timeout)
	}
	if m.buildRequest().Timeout != 7*time.Second {
		t.Error("buildRequest should carry the timeout")
	}
}

func TestResponseHeadersTab(t *testing.T) {
	m := sized()
	m = step(m, responseMsg{resp: model.Response{
		StatusCode: 200, Status: "200 OK",
		Headers: []model.Header{{Name: "X-Trace", Value: "abc", Enabled: true}},
		Body:    []byte(`{}`),
	}})
	m = m.setFocus(focusResponse)

	if m.respTab != 0 {
		t.Fatalf("default response tab = %d, want Body", m.respTab)
	}
	m = step(m, runes("]")) // switch to Headers
	if m.respTab != 1 {
		t.Fatalf("after ] tab = %d, want Headers", m.respTab)
	}
	if !strings.Contains(m.currentResponseText(), "X-Trace") {
		t.Errorf("headers tab should show response headers, got:\n%s", m.currentResponseText())
	}
}

func TestCommandGhost(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	cases := []struct{ val, want string }{
		{"save ", "<group>/<name>"},   // fresh save
		{"save APISet1/", "<name>"},   // m a into a group
		{"mkgroup ", "<group>"},       // m g new group
		{"save APISet1/getUsers", ""}, // name already typed
		{"open ", "<group>/<name>"},   // open needs a name
	}
	for _, c := range cases {
		mm := m.openCommandLineWith(':', c.val)
		if got := mm.commandGhost(); got != c.want {
			t.Errorf("commandGhost(%q) = %q, want %q", c.val, got, c.want)
		}
	}
	// Search has no command ghost.
	if s := m.openCommandLineWith('/', "foo"); s.commandGhost() != "" {
		t.Error("search kind should have no ghost")
	}
}

func TestHelpToggle(t *testing.T) {
	m := sized()
	m = step(m, runes("?"))
	if !m.showHelp {
		t.Fatal("? should open help")
	}
	if !strings.Contains(m.View(), "keybindings") {
		t.Error("help view should render keybindings")
	}
	// j/k scroll the overlay rather than closing it.
	m = step(m, runes("j"))
	if !m.showHelp {
		t.Error("j should scroll help, not dismiss it")
	}
	if m.helpScroll == 0 && m.helpMaxScroll() > 0 {
		t.Error("j should advance the help scroll position")
	}
	m = step(m, runes("q")) // any non-scroll key closes
	if m.showHelp {
		t.Error("a non-scroll key press should dismiss help")
	}
	if m.helpScroll != 0 {
		t.Error("dismissing help should reset its scroll position")
	}
}
