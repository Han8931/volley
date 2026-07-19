package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/vars"
)

// envModel returns a sized model whose environment store lives in a temp dir.
func envModel(t *testing.T) Model {
	t.Helper()
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.envStore = vars.EnvStore{Root: filepath.Join(t.TempDir(), "environments")}
	return m
}

func TestEnvSwitchAndResolution(t *testing.T) {
	m := envModel(t)

	next, _ := m.executeCommand("env")
	m = next.(Model)
	if !strings.Contains(m.statusMsg, "no environments") {
		t.Errorf("bare :env with empty store: %q", m.statusMsg)
	}

	if err := m.envStore.Save("staging", map[string]string{"host": "staging.test", "tok": "env-tok"}); err != nil {
		t.Fatal(err)
	}

	next, _ = m.executeCommand("env staging")
	m = next.(Model)
	if m.envName != "staging" {
		t.Fatalf(":env staging should activate it, envName = %q", m.envName)
	}
	if !strings.Contains(m.statusMsg, "staging") || !strings.Contains(m.statusMsg, "2 variables") {
		t.Errorf("statusMsg = %q", m.statusMsg)
	}

	// The active environment shows in the status bar and resolves placeholders.
	if view := stripANSI(m.View()); !strings.Contains(view, "(staging)") {
		t.Error("status bar should show the active environment")
	}
	m.url.SetText("https://{{host}}/v1")
	if built := m.buildRequest(); built.URL != "https://staging.test/v1" {
		t.Errorf("built URL = %q", built.URL)
	}

	// Session :set overrides the environment.
	next, _ = m.executeCommand("set host=local.test")
	m = next.(Model)
	if built := m.buildRequest(); built.URL != "https://local.test/v1" {
		t.Errorf("session override: built URL = %q", built.URL)
	}

	// Bare :env lists with the active one marked.
	next, _ = m.executeCommand("env")
	m = next.(Model)
	if !strings.Contains(m.statusMsg, "[staging]") {
		t.Errorf("listing should mark the active env: %q", m.statusMsg)
	}

	// :env off deactivates; the placeholder falls back to the session var only.
	next, _ = m.executeCommand("env off")
	m = next.(Model)
	if m.envName != "" || m.envVars != nil {
		t.Error(":env off should deactivate")
	}
	if view := stripANSI(m.View()); strings.Contains(view, "(staging)") {
		t.Error("status bar chip should disappear with the environment")
	}
}

func TestEnvSwitchUnknownName(t *testing.T) {
	m := envModel(t)
	next, _ := m.executeCommand("env nosuch")
	m = next.(Model)
	if m.envName != "" || !strings.Contains(m.statusMsg, "no environment named nosuch") {
		t.Errorf("envName=%q statusMsg=%q", m.envName, m.statusMsg)
	}
}

func TestEnvRemove(t *testing.T) {
	m := envModel(t)
	if err := m.envStore.Save("staging", map[string]string{"a": "1"}); err != nil {
		t.Fatal(err)
	}
	next, _ := m.executeCommand("env staging")
	m = next.(Model)

	next, _ = m.executeCommand("envrm staging")
	m = next.(Model)
	if m.envName != "" {
		t.Error("removing the active environment must deactivate it")
	}
	if !strings.Contains(m.statusMsg, "was active") {
		t.Errorf("statusMsg = %q", m.statusMsg)
	}
	if names, _ := m.envStore.List(); len(names) != 0 {
		t.Errorf("store still lists %v", names)
	}
}

func TestEnvEditorRoundTrip(t *testing.T) {
	m := envModel(t)

	// Simulate the $EDITOR round-trip: the temp file the editor wrote comes
	// back via envEditorFinishedMsg.
	f := filepath.Join(t.TempDir(), "edited.json")
	if err := os.WriteFile(f, []byte(`{"host": "prod.test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	next, _ := m.applyEnvEditorResult(envEditorFinishedMsg{path: f, name: "prod"})
	m = next.(Model)
	if m.envName != "prod" || m.envVars["host"] != "prod.test" {
		t.Errorf("edited env should be saved and active: name=%q vars=%v", m.envName, m.envVars)
	}
	if got, err := m.envStore.Load("prod"); err != nil || got["host"] != "prod.test" {
		t.Errorf("store Load = %v, %v", got, err)
	}

	// Malformed JSON is rejected without touching the store or active env.
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"host": ["not", "flat"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	next, _ = m.applyEnvEditorResult(envEditorFinishedMsg{path: bad, name: "prod"})
	m = next.(Model)
	if !strings.Contains(m.statusMsg, "parse failed") {
		t.Errorf("statusMsg = %q", m.statusMsg)
	}
	if got, _ := m.envStore.Load("prod"); got["host"] != "prod.test" {
		t.Errorf("failed parse must not overwrite the store, got %v", got)
	}
}

func TestEnvCompletion(t *testing.T) {
	m := envModel(t)
	if err := m.envStore.Save("staging", nil); err != nil {
		t.Fatal(err)
	}

	names, what, errMsg := m.argCandidates("env", 0)
	if errMsg != "" || what != "environment" {
		t.Fatalf("argCandidates: what=%q err=%q", what, errMsg)
	}
	joined := strings.Join(names, " ")
	if !strings.Contains(joined, "staging") || !strings.Contains(joined, "off") {
		t.Errorf("env candidates = %v", names)
	}

	names, _, _ = m.argCandidates("envrm", 0)
	if strings.Contains(strings.Join(names, " "), "off") {
		t.Errorf(":envrm must not offer 'off': %v", names)
	}
}

func TestSetBareListsVariables(t *testing.T) {
	m := envModel(t)
	next, _ := m.executeCommand("set tok=abc")
	m = next.(Model)
	if err := m.envStore.Save("staging", map[string]string{"host": "x"}); err != nil {
		t.Fatal(err)
	}
	next, _ = m.executeCommand("env staging")
	m = next.(Model)

	next, _ = m.executeCommand("set")
	m = next.(Model)
	for _, want := range []string{"session: tok", "staging: host"} {
		if !strings.Contains(m.statusMsg, want) {
			t.Errorf("bare :set should list %q, got %q", want, m.statusMsg)
		}
	}
	if strings.Contains(m.statusMsg, "abc") {
		t.Error("bare :set must not print values")
	}
}
