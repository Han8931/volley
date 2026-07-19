package tui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/loadtest"
)

// loadModel returns a sized model whose profile store lives in a temp dir.
func loadModel(t *testing.T) Model {
	t.Helper()
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.loadStore = loadtest.Store{Root: filepath.Join(t.TempDir(), "loadprofiles")}
	return m
}

func TestLoadPickerFlow(t *testing.T) {
	m := loadModel(t)
	next, _ := m.executeCommand("loadtest")
	m = next.(Model)
	if !m.loadPicker {
		t.Fatal(":loadtest should open the profile picker")
	}
	if len(m.pickerItems) != len(loadtest.DefaultProfiles()) {
		t.Fatalf("picker lists %d profiles, want the %d defaults", len(m.pickerItems), len(loadtest.DefaultProfiles()))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "LOAD TEST") || !strings.Contains(view, "constant") {
		t.Errorf("picker view missing content:\n%s", view)
	}

	m = step(m, runes("j"))
	if m.pickerIdx != 1 {
		t.Errorf("j should move the picker cursor, idx = %d", m.pickerIdx)
	}
	m = step(m, keyEsc)
	if m.loadPicker {
		t.Error("esc should close the picker")
	}
}

func TestLoadConfirmGuardsRun(t *testing.T) {
	m := loadModel(t)
	m.url.SetText("https://example.test")
	next, _ := m.executeCommand("loadtest spike")
	m = next.(Model)
	if !m.loadConfirm {
		t.Fatal(":loadtest <name> should arm the confirm prompt")
	}
	for _, want := range []string{"spike", "100", "example.test", "(y/n)"} {
		if !strings.Contains(m.statusMsg, want) {
			t.Errorf("confirm prompt should mention %q, got %q", want, m.statusMsg)
		}
	}
	// n declines: nothing starts.
	m = step(m, runes("n"))
	if m.loadConfirm || m.loadRun != nil {
		t.Error("declining must not start a run")
	}
}

func TestLoadConfirmRequiresURL(t *testing.T) {
	m := loadModel(t)
	next, _ := m.executeCommand("loadtest spike")
	m = next.(Model)
	if m.loadConfirm {
		t.Error("an empty URL must not reach the confirm prompt")
	}
	if !strings.Contains(m.statusMsg, "URL is empty") {
		t.Errorf("statusMsg = %q", m.statusMsg)
	}
}

func TestLoadRunLifecycle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := loadModel(t)
	m.url.SetText(srv.URL)
	m.req.URL = srv.URL

	quick := loadtest.Profile{
		Name:   "quick",
		Points: []loadtest.Point{{At: 0, RPS: 40}, {At: loadtest.Duration(300 * time.Millisecond), RPS: 40}},
	}
	next, _ := m.startLoadTest(quick)
	m = next.(Model)
	if m.loadRun == nil || m.focus != focusResponse {
		t.Fatal("starting a run should attach it and focus the response pane")
	}

	// Sends are blocked while the run is live.
	blocked, _ := m.send()
	if blocked.(Model).sending {
		t.Error("send must be refused during a load run")
	}

	// The run view renders during the run.
	if view := stripANSI(m.View()); !strings.Contains(view, "LOAD TEST quick") {
		t.Errorf("run view missing:\n%s", view)
	}

	// Poll until the engine reports done (the profile is 300ms).
	deadline := time.After(5 * time.Second)
	for !m.loadSnap.Done {
		select {
		case <-deadline:
			t.Fatal("run never finished")
		case <-time.After(50 * time.Millisecond):
		}
		next, _ = m.handleLoadTick(loadTickMsg{seq: m.loadSeq})
		m = next.(Model)
	}
	if m.loadSnap.Completed == 0 || m.loadSnap.Errors != 0 {
		t.Errorf("snapshot after clean run: %+v", m.loadSnap)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "done") || !strings.Contains(view, "achieved") {
		t.Errorf("finished view missing summary:\n%s", view)
	}

	// esc dismisses the finished results and frees the response pane.
	next, _ = m.updateNormal(keyEsc)
	m = next.(Model)
	if m.loadRun != nil {
		t.Error("esc should dismiss finished results")
	}
}

func TestStaleLoadTickIgnored(t *testing.T) {
	m := loadModel(t)
	m.loadSeq = 3
	next, cmd := m.handleLoadTick(loadTickMsg{seq: 2})
	if cmd != nil || next.(Model).loadSnap.Completed != 0 {
		t.Error("a stale tick must be ignored and not reschedule")
	}
}

func TestSparkline(t *testing.T) {
	if got := sparkline([]float64{0, 1, 2, 4}, 4); got != " ▂▄█" {
		t.Errorf("sparkline = %q", got)
	}
	if got := sparkline(nil, 10); got != "" {
		t.Errorf("empty series = %q", got)
	}
	// Resampling down max-pools, so a one-second spike survives.
	spiky := make([]float64, 60)
	spiky[30] = 100
	if got := sparkline(spiky, 6); !strings.Contains(got, "█") {
		t.Errorf("downsampled spike lost its peak: %q", got)
	}
	if got := len([]rune(sparkline(spiky, 6))); got != 6 {
		t.Errorf("width = %d, want 6", got)
	}
}

func TestTargetSeries(t *testing.T) {
	p := loadtest.Constant(10, 5*time.Second)
	s := targetSeries(p)
	if len(s) != 5 {
		t.Fatalf("len = %d, want 5", len(s))
	}
	for _, v := range s {
		if v != 10 {
			t.Errorf("constant series should be flat 10, got %v", s)
		}
	}
}
