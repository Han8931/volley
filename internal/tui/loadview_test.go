package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
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
	dir := t.TempDir()
	m.loadStore = loadtest.Store{Root: filepath.Join(dir, "loadprofiles")}
	m.resultStore = loadtest.ResultStore{Root: filepath.Join(dir, "loadresults")}
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

	// Completion builds the k6-style analysis once and auto-saves it.
	if m.loadSummary == nil {
		t.Fatal("finished run should build a summary")
	}
	if m.loadSummary.OK == 0 || m.loadSummary.Profile != "quick" {
		t.Errorf("summary = %+v", m.loadSummary)
	}
	if !strings.Contains(view, "percentiles") || !strings.Contains(view, "p90") {
		t.Errorf("finished view missing analysis block:\n%s", view)
	}
	files, err := os.ReadDir(m.resultStore.Root)
	if err != nil || len(files) != 1 {
		t.Fatalf("finished run should save one result file, got %v, %v", files, err)
	}
	if !strings.HasPrefix(files[0].Name(), "quick-") {
		t.Errorf("result file = %q", files[0].Name())
	}
	if !strings.Contains(m.statusMsg, "saved "+files[0].Name()) {
		t.Errorf("status should report the saved result, got %q", m.statusMsg)
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

func TestLoadNewEntersShapeMode(t *testing.T) {
	m := loadModel(t)
	next, _ := m.executeCommand("loadnew constant")
	if got := next.(Model).statusMsg; !strings.Contains(got, "already exists") {
		t.Errorf("existing name must be refused, got %q", got)
	}
	next, _ = m.executeCommand("loadnew mine spike")
	got := next.(Model)
	if !got.shapeEdit || got.shapeName != "mine" {
		t.Fatalf("loadnew should enter shape mode on the new name, got edit=%v name=%q", got.shapeEdit, got.shapeName)
	}
	if got.shapeProfile().Peak() != 100 {
		t.Errorf("template shape should carry over, peak = %v", got.shapeProfile().Peak())
	}
	if !got.shapeDirty() {
		t.Error("a brand-new shape must count as unsaved")
	}
	if _, err := got.loadStore.Load("mine"); err == nil {
		t.Error("nothing should be stored before w")
	}
	next, _ = m.executeCommand("loadnew mine no-such-template")
	if got := next.(Model).statusMsg; !strings.Contains(got, "no-such-template") {
		t.Errorf("unknown template must be reported, got %q", got)
	}
	next, _ = m.executeCommand("loadedit nope")
	if got := next.(Model).statusMsg; !strings.Contains(got, "no load profile") {
		t.Errorf(":loadedit on a missing profile must error, got %q", got)
	}
}

func TestShapeEditorKeys(t *testing.T) {
	m := loadModel(t)
	next, _ := m.executeCommand("loadedit constant") // seeds defaults, opens mode
	m = next.(Model)
	if !m.shapeEdit || len(m.shapePoints) != 2 {
		t.Fatalf("mode should open on constant's 2 points, got %v", m.shapePoints)
	}

	// k raises the selected (first) point's rate; K by 10.
	m = step(step(m, runes("k")), runes("K"))
	if got := m.shapePoints[0].RPS; got != 31 {
		t.Errorf("rate after k K = %v, want 31", got)
	}
	// The first point's time is pinned.
	m = step(m, runes("L"))
	if m.shapePoints[0].At != 0 {
		t.Error("first point must stay at 0s")
	}
	// l selects the last point; L extends the duration by 1s.
	m = step(step(m, runes("l")), runes("L"))
	if got := m.shapeProfile().Duration(); got != 31*time.Second {
		t.Errorf("duration = %v, want 31s", got)
	}
	// ] provides fine-grained 100ms timing control.
	m = step(m, runes("]"))
	if got := m.shapeProfile().Duration(); got != 31100*time.Millisecond {
		t.Errorf("fine duration = %v, want 31.1s", got)
	}
	// Run details are editable without leaving the shape view.
	planned := m.shapeProfile().PlannedRequests()
	m = step(m, runes("-"))
	if got, want := m.shapeProfile().MaxRequests, planned-1; got != want {
		t.Errorf("request limit = %d, want %d", got, want)
	}
	m = step(m, runes("c"))
	if got := m.shapeProfile().MaxWorkers; got != loadtest.DefaultMaxWorkers+1 {
		t.Errorf("max workers = %d, want %d", got, loadtest.DefaultMaxWorkers+1)
	}
	// H beyond the previous point clamps against it.
	for i := 0; i < 41; i++ {
		m = step(m, runes("H"))
	}
	if got := time.Duration(m.shapePoints[1].At); got != 0 {
		t.Errorf("time clamps at the previous point, got %v", got)
	}
	// a adds a point after the last; x deletes it again.
	m = step(m, runes("a"))
	if len(m.shapePoints) != 3 || m.shapeSel != 2 {
		t.Fatalf("a should append and select, points=%d sel=%d", len(m.shapePoints), m.shapeSel)
	}
	m = step(m, runes("x"))
	if len(m.shapePoints) != 2 {
		t.Errorf("x should delete, points=%d", len(m.shapePoints))
	}
	// The two-point minimum is protected.
	m = step(m, runes("x"))
	if len(m.shapePoints) != 2 {
		t.Error("deleting below two points must be refused")
	}

	// The editor renders chart, readout, and marker.
	view := stripANSI(m.View())
	for _, want := range []string{"SHAPE constant", "point 2/2", "rps", "requests", "workers", "CONTROLS", "select point", "time ±100ms", "request limit", "save + run", "●"} {
		if !strings.Contains(view, want) {
			t.Errorf("editor view missing %q:\n%s", want, view)
		}
	}
}

func TestShapeEditorSaveAndDiscard(t *testing.T) {
	m := loadModel(t)
	next, _ := m.executeCommand("loadnew mine")
	m = next.(Model)

	// w persists the working shape.
	m = step(step(m, runes("K")), runes("w"))
	p, err := m.loadStore.Load("mine")
	if err != nil || p.Peak() != 30 {
		t.Fatalf("saved profile = %+v, %v", p, err)
	}
	if m.shapeDirty() {
		t.Error("saving must reset the dirty state")
	}

	// esc with edits arms the discard confirm; y leaves without saving.
	m = step(m, runes("K"))
	m = step(m, keyEsc)
	if !m.shapeConfirmDiscard {
		t.Fatal("esc with unsaved edits should ask before discarding")
	}
	m = step(m, runes("y"))
	if m.shapeEdit {
		t.Error("y should leave the editor")
	}
	if p, _ := m.loadStore.Load("mine"); p.Peak() != 30 {
		t.Errorf("discarded edit must not be stored, peak = %v", p.Peak())
	}

	// ⏎ saves and flows into the run confirmation.
	m.url.SetText("https://example.test")
	next, _ = m.executeCommand("loadedit mine")
	m = next.(Model)
	m = step(m, runes("K"))
	next, _ = m.updateShapeEditor(keyEnter)
	m = next.(Model)
	if m.shapeEdit || !m.loadConfirm {
		t.Errorf("enter should save and arm the run confirm, edit=%v confirm=%v", m.shapeEdit, m.loadConfirm)
	}
	if p, _ := m.loadStore.Load("mine"); p.Peak() != 40 {
		t.Errorf("enter must save first, peak = %v", p.Peak())
	}
}

func TestApplyProfileEditorResult(t *testing.T) {
	m := loadModel(t)
	if err := m.loadStore.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	write := func(content string) string {
		t.Helper()
		f := filepath.Join(t.TempDir(), "edited.json")
		if err := osWriteFile(f, content); err != nil {
			t.Fatal(err)
		}
		return f
	}

	// A valid edit is saved and the picker reopens on it.
	good := write(`{"name":"mine","points":[{"at":"0s","rps":2},{"at":"8s","rps":20}]}`)
	next, _ := m.applyProfileEditorResult(profileEditorFinishedMsg{path: good, name: "mine"})
	got := next.(Model)
	if p, err := got.loadStore.Load("mine"); err != nil || p.Peak() != 20 {
		t.Fatalf("edited profile not saved: %v %v", p, err)
	}
	if !got.loadPicker || got.pickerItems[got.pickerIdx].Name != "mine" {
		t.Errorf("picker should reopen on the saved profile, idx=%d", got.pickerIdx)
	}

	// Invalid JSON or an invalid shape is rejected and nothing is stored.
	for _, tc := range []struct{ name, content string }{
		{"broken", `{`},
		{"badshape", `{"points":[{"at":"0s","rps":-4}]}`},
	} {
		next, _ = m.applyProfileEditorResult(profileEditorFinishedMsg{path: write(tc.content), name: tc.name})
		if _, err := next.(Model).loadStore.Load(tc.name); err == nil {
			t.Errorf("%s must not be saved", tc.name)
		}
		if next.(Model).statusMsg == "" {
			t.Errorf("%s: failure must be reported", tc.name)
		}
	}
}

func TestPickerNPrefillsLoadnew(t *testing.T) {
	m := loadModel(t)
	next, _ := m.executeCommand("loadtest")
	m = next.(Model)
	next, _ = m.updateLoadPicker(runes("n"))
	m = next.(Model)
	if m.loadPicker || !m.cmdActive || m.cmd.Value() != "loadnew " {
		t.Errorf("n should close the picker and prefill :loadnew, got active=%v value=%q", m.cmdActive, m.cmd.Value())
	}
}

// osWriteFile writes a small fixture file for the editor-result tests.
func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestFormatRunDuration(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{50 * time.Second, "50s"},
		{time.Minute, "1m"},
		{90 * time.Second, "1m30s"},
		{2 * time.Hour, "2h"},
		{0, "0s"},
	} {
		if got := formatRunDuration(tc.d); got != tc.want {
			t.Errorf("formatRunDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
