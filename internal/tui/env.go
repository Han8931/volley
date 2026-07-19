package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/vars"
)

// envState is the named-environment concern: the on-disk store, plus which
// environment is active and its loaded variables. Session ":set" variables
// live in Model.vars and override the active environment; the process
// environment stays the final fallback (see Model.resolver).
type envState struct {
	envStore vars.EnvStore
	envName  string            // active environment, "" for none
	envVars  map[string]string // active environment's variables
}

// resolver is the placeholder resolution order for outgoing requests:
// session :set overrides, then the active environment, then the process env
// (the fallback built into vars.Layered).
func (m Model) resolver() vars.Layered {
	return vars.Layered{m.vars, m.envVars}
}

// listEnvs handles ":env" with no argument — the stored environments with the
// active one marked, in the status line.
func (m Model) listEnvs() (tea.Model, tea.Cmd) {
	names, err := m.envStore.List()
	if err != nil {
		m.statusMsg = "environments unavailable: " + err.Error()
		return m, nil
	}
	if len(names) == 0 {
		m.statusMsg = "no environments — :envnew <name> creates one"
		return m, nil
	}
	for i, n := range names {
		if n == m.envName {
			names[i] = "[" + n + "]"
		}
	}
	m.statusMsg = "environments: " + strings.Join(names, " · ") + " — :env <name> switches · :env off deactivates"
	return m, nil
}

// switchEnv activates a stored environment ("off"/"none" deactivates).
func (m Model) switchEnv(name string) (tea.Model, tea.Cmd) {
	if name == "off" || name == "none" || name == "-" {
		if m.envName == "" {
			m.statusMsg = "no environment active"
			return m, nil
		}
		prev := m.envName
		m.envName, m.envVars = "", nil
		m.statusMsg = "environment " + prev + " deactivated"
		return m, nil
	}
	vals, err := m.envStore.Load(name)
	if err != nil {
		if os.IsNotExist(err) {
			m.statusMsg = "no environment named " + name + " — :envnew " + name + " creates it"
		} else {
			m.statusMsg = "environment load failed: " + err.Error()
		}
		return m, nil
	}
	m.envName, m.envVars = name, vals
	m.statusMsg = fmt.Sprintf("environment %s — %d variable%s", name, len(vals), plural(len(vals)))
	return m, nil
}

// newEnv starts ":envnew" — a skeleton environment opened in $EDITOR. Nothing
// is stored until the editor round-trip saves cleanly, matching :loadnew.
func (m Model) newEnv(name string) (tea.Model, tea.Cmd) {
	if _, err := m.envStore.Load(name); err == nil {
		m.statusMsg = name + " already exists — use :envedit " + name
		return m, nil
	}
	return m.openEnvEditor(name, map[string]string{
		"base_url": "https://api.example.com",
	})
}

// editEnvByName starts ":envedit" — the environment's JSON in $EDITOR. With
// no name it edits the active environment.
func (m Model) editEnvByName(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		if m.envName == "" {
			m.statusMsg = "usage: :envedit <name> (no environment active)"
			return m, nil
		}
		name = m.envName
	}
	vals, err := m.envStore.Load(name)
	if err != nil {
		m.statusMsg = "no environment named " + name
		return m, nil
	}
	return m.openEnvEditor(name, vals)
}

// removeEnv handles ":envrm" — deletes the stored file and deactivates the
// environment if it was the active one.
func (m Model) removeEnv(name string) (tea.Model, tea.Cmd) {
	if err := m.envStore.Delete(name); err != nil {
		if os.IsNotExist(err) {
			m.statusMsg = "no environment named " + name
		} else {
			m.statusMsg = "environment delete failed: " + err.Error()
		}
		return m, nil
	}
	m.statusMsg = "deleted environment " + name
	if m.envName == name {
		m.envName, m.envVars = "", nil
		m.statusMsg += " (was active)"
	}
	return m, nil
}

// envEditorFinishedMsg reports the $EDITOR round-trip for an environment.
type envEditorFinishedMsg struct {
	path string // temp file holding the edited JSON
	name string // environment name to save under
	err  error
}

// openEnvEditor writes vals to a temp file and opens it in $EDITOR; the
// result is validated and saved under name when the editor exits.
func (m Model) openEnvEditor(name string, vals map[string]string) (tea.Model, tea.Cmd) {
	editor := resolveEditor()
	if editor == "" {
		m.statusMsg = "set $VISUAL or $EDITOR to edit environments"
		return m, nil
	}
	b, err := json.MarshalIndent(vals, "", "  ")
	if err != nil {
		m.statusMsg = "edit failed: " + err.Error()
		return m, nil
	}
	f, err := os.CreateTemp("", "volley-env-*.json")
	if err != nil {
		m.statusMsg = "edit failed: " + err.Error()
		return m, nil
	}
	path := f.Name()
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		os.Remove(path)
		m.statusMsg = "edit failed: " + err.Error()
		return m, nil
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		m.statusMsg = "edit failed: " + err.Error()
		return m, nil
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return envEditorFinishedMsg{path: path, name: name, err: err}
	})
}

// applyEnvEditorResult validates and stores the edited environment, then
// activates it — editing an environment almost always means you're about to
// use it, and activation makes the result visible in the status bar.
func (m Model) applyEnvEditorResult(msg envEditorFinishedMsg) (tea.Model, tea.Cmd) {
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
	var vals map[string]string
	if err := json.Unmarshal(b, &vals); err != nil {
		m.statusMsg = "environment parse failed (want a flat {\"name\": \"value\"} object): " + err.Error()
		return m, nil
	}
	if err := m.envStore.Save(msg.name, vals); err != nil {
		m.statusMsg = "environment save failed: " + err.Error()
		return m, nil
	}
	m.envName, m.envVars = msg.name, vals
	m.statusMsg = fmt.Sprintf("saved environment %s — active, %d variable%s", msg.name, len(vals), plural(len(vals)))
	return m, nil
}

// listVars handles ":set" with no argument — every name the resolver would
// substitute right now, by layer. Values are withheld: environments hold
// tokens, and the status bar is the most screenshotted row of the app.
func (m *Model) listVars() {
	var parts []string
	if names := sortedKeys(m.vars); len(names) > 0 {
		parts = append(parts, "session: "+strings.Join(names, " "))
	}
	if names := sortedKeys(m.envVars); len(names) > 0 {
		parts = append(parts, m.envName+": "+strings.Join(names, " "))
	}
	if len(parts) == 0 {
		m.statusMsg = "no variables — :set name=value · :env <name>"
		return
	}
	m.statusMsg = strings.Join(parts, " · ")
}

// envNameCandidates lists stored environment names for Tab completion.
func (m Model) envNameCandidates() ([]string, string, string) {
	names, err := m.envStore.List()
	if err != nil {
		return nil, "", "environments unavailable: " + err.Error()
	}
	return names, "environment", ""
}

func sortedKeys(vals map[string]string) []string {
	out := make([]string, 0, len(vals))
	for k := range vals {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
