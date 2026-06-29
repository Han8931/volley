package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/model"
)

// step applies a message and returns the concrete Model for chaining.
func step(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
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
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
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
