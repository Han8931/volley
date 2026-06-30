package tui

import (
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

	m.url.SetValue("https://x.test/{{tok}}")
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
