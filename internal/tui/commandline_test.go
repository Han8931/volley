package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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

	path := t.TempDir() + "/request.json"
	edited := `{
  "method": "put",
  "url": "https://new.test",
  "headers": [{"name":"X-Test","value":"yes","enabled":true}],
  "query": [{"key":"q","value":"1","enabled":true}],
  "body": "edited body",
  "timeout": "5s"
}`
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

func TestEditorNamedRequestSavesAndLoadsTarget(t *testing.T) {
	m := sized()
	m.collectionStore.Root = t.TempDir()
	if err := m.collectionStore.Save("target", model.Request{Method: "GET", URL: "https://old.test"}); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/request.json"
	edited := `{"method":"POST","url":"https://target.test","body":"saved"}`
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
	m = step(m, runes("j")) // any key closes
	if m.showHelp {
		t.Error("a key press should dismiss help")
	}
}
