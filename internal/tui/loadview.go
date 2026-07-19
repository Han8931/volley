package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/httpx"
	"github.com/tabularasa/volley/internal/loadtest"
	"github.com/tabularasa/volley/internal/model"
)

// This file owns the load-testing UI: the profile picker, the pre-run
// confirmation, the live run view that takes over the response pane, and the
// snapshot polling that drives it. The engine itself lives in
// internal/loadtest; the TUI only starts runs, polls Snapshot, and renders.

// loadState is the load-testing concern on the root Model.
type loadState struct {
	loadStore   loadtest.Store       // profile files on disk (seeded on first use)
	resultStore loadtest.ResultStore // finished-run summaries on disk

	loadRun       *loadtest.Run     // active or just-finished run; nil = no load view
	loadProfile   loadtest.Profile  // profile of the current run
	loadTarget    model.Request     // request the run fires (frozen at start)
	loadStartedAt time.Time         // wall-clock start, stamped into the summary
	loadSnap      loadtest.Snapshot // latest polled snapshot
	loadSummary   *loadtest.Summary // built once when the run completes
	loadSeq       int               // invalidates stale tick messages after stop/restart
	loadStopped   bool              // user pressed esc; distinguishes "stopped" from "done"

	loadPicker  bool // profile picker is up in the response pane
	pickerItems []loadtest.Profile
	pickerIdx   int

	loadConfirm    bool // y/n prompt before firing the run
	pendingProfile loadtest.Profile

	// The dedicated shape-editing mode (see shapeeditor.go): an interactive
	// point editor for building custom load shapes without leaving Volley.
	shapeEdit            bool
	shapeName            string           // profile name the shape saves to
	shapeBase            loadtest.Profile // carries description and run limits
	shapePoints          []loadtest.Point // working copy being edited
	shapeBaseline        []loadtest.Point // last saved/loaded points, for dirty checks
	shapeBaselineLimit   int              // saved maxRequests value for dirty checks
	shapeBaselineWorkers int              // saved maxWorkers value for dirty checks
	shapeSel             int              // selected point index
	shapeConfirmDiscard  bool             // esc pressed with unsaved changes
}

// loadTickMsg drives snapshot polling while a run is active.
type loadTickMsg struct{ seq int }

// loadPollEvery is the snapshot poll cadence: fast enough that the chart and
// counters feel live, slow enough to cost nothing.
const loadPollEvery = 500 * time.Millisecond

func loadTick(seq int) tea.Cmd {
	return tea.Tick(loadPollEvery, func(time.Time) tea.Msg { return loadTickMsg{seq: seq} })
}

// loadRunning reports whether a load run is still producing results.
func (m Model) loadRunning() bool {
	return m.loadRun != nil && !m.loadSnap.Done
}

// loadViewShown reports whether the response pane is showing load-test content.
func (m Model) loadViewShown() bool {
	return m.loadRun != nil || m.loadPicker
}

// openLoadPicker seeds the profile store on first use, lists the profiles, and
// opens the picker in the response pane.
func (m Model) openLoadPicker() (tea.Model, tea.Cmd) {
	if m.loadRunning() {
		m.statusMsg = "load test already running — esc to stop it first"
		return m, nil
	}
	if err := m.loadStore.EnsureDefaults(); err != nil {
		m.statusMsg = "load profiles unavailable: " + err.Error()
		return m, nil
	}
	items, err := m.loadStore.List()
	if err != nil {
		m.statusMsg = "load profiles unavailable: " + err.Error()
		return m, nil
	}
	if len(items) == 0 {
		m.statusMsg = "no load profiles — add JSON files under " + homeShorten(m.loadStore.Root)
		return m, nil
	}
	m.loadRun = nil // a finished run's view yields to the picker
	m.loadPicker = true
	m.pickerItems = items
	if m.pickerIdx >= len(items) {
		m.pickerIdx = 0
	}
	m.statusMsg = "load profile: j/k choose · ⏎ run · e edit · E $EDITOR · n new · esc cancel"
	return m, nil
}

// updateLoadPicker handles keys while the picker is up.
func (m Model) updateLoadPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.pickerIdx < len(m.pickerItems)-1 {
			m.pickerIdx++
		}
	case "k", "up":
		if m.pickerIdx > 0 {
			m.pickerIdx--
		}
	case "enter":
		p := m.pickerItems[m.pickerIdx]
		m.loadPicker = false
		return m.confirmLoadTest(p), nil
	case "e":
		// Reshape the highlighted profile in the dedicated editing mode.
		p := m.pickerItems[m.pickerIdx]
		return m.openShapeEditor(p.Name, p), nil
	case "E":
		// The highlighted profile's raw JSON in $EDITOR — exact values without
		// entering the shape editor first. The picker stays armed: the editor
		// round-trip reopens it on the saved profile.
		p := m.pickerItems[m.pickerIdx]
		return m.editLoadProfile(p.Name, p)
	case "n":
		// Hand off to :loadnew with the command line prefilled.
		m.loadPicker = false
		return m.openCommandLineWith(':', "loadnew "), nil
	case "esc", "q":
		m.loadPicker = false
		m.statusMsg = "load test cancelled"
	}
	return m, nil
}

// --- creating and editing profiles -----------------------------------------

// profileEditorFinishedMsg reports the $EDITOR round-trip for a load profile.
type profileEditorFinishedMsg struct {
	path string // temp file holding the edited JSON
	name string // profile name to save under
	err  error
}

// newLoadProfile starts :loadnew — a fresh profile named name, based on an
// existing profile (default: the built-in constant shape), opened in the
// shape-editing mode. Nothing is written to the store until the shape is
// saved, so an abandoned session leaves no half-made profile behind.
func (m Model) newLoadProfile(name, template string) (tea.Model, tea.Cmd) {
	if err := m.loadStore.EnsureDefaults(); err != nil {
		m.statusMsg = "load profiles unavailable: " + err.Error()
		return m, nil
	}
	if m.loadRunning() {
		m.statusMsg = "load test already running — esc to stop it first"
		return m, nil
	}
	if _, err := m.loadStore.Load(name); err == nil {
		m.statusMsg = name + " already exists — use :loadedit " + name
		return m, nil
	}
	base := loadtest.Constant(20, 30*time.Second)
	if template != "" {
		p, err := m.loadStore.Load(template)
		if err != nil {
			m.statusMsg = "no load profile named " + template + " to start from"
			return m, nil
		}
		base = p
	}
	base.Description = strings.TrimSpace(base.Description)
	m = m.openShapeEditor(name, base)
	// A new shape counts as unsaved from the start: it exists nowhere yet.
	m.shapeBaseline = nil
	return m, nil
}

// editLoadProfileByName starts :loadedit on a stored profile.
func (m Model) editLoadProfileByName(name string) (tea.Model, tea.Cmd) {
	if err := m.loadStore.EnsureDefaults(); err != nil {
		m.statusMsg = "load profiles unavailable: " + err.Error()
		return m, nil
	}
	if m.loadRunning() {
		m.statusMsg = "load test already running — esc to stop it first"
		return m, nil
	}
	p, err := m.loadStore.Load(name)
	if err != nil {
		m.statusMsg = "no load profile named " + name
		return m, nil
	}
	return m.openShapeEditor(name, p), nil
}

// editLoadProfileJSONByName starts :loadeditor — the stored profile's raw
// JSON opened directly in $VISUAL / $EDITOR, skipping the shape editor.
func (m Model) editLoadProfileJSONByName(name string) (tea.Model, tea.Cmd) {
	if err := m.loadStore.EnsureDefaults(); err != nil {
		m.statusMsg = "load profiles unavailable: " + err.Error()
		return m, nil
	}
	if m.loadRunning() {
		m.statusMsg = "load test already running — esc to stop it first"
		return m, nil
	}
	p, err := m.loadStore.Load(name)
	if err != nil {
		m.statusMsg = "no load profile named " + name
		return m, nil
	}
	return m.editLoadProfile(name, p)
}

// editLoadProfile writes p to a temp file and opens it in $EDITOR; the result
// is validated and saved under name when the editor exits.
func (m Model) editLoadProfile(name string, p loadtest.Profile) (tea.Model, tea.Cmd) {
	editor := resolveEditor()
	if editor == "" {
		m.statusMsg = "set $VISUAL or $EDITOR to edit load profiles"
		return m, nil
	}
	p.Name = name
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		m.statusMsg = "edit failed: " + err.Error()
		return m, nil
	}
	f, err := os.CreateTemp("", "volley-profile-*.json")
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
		return profileEditorFinishedMsg{path: path, name: name, err: err}
	})
}

// applyProfileEditorResult validates and stores the edited profile, then
// reopens the picker on it so the new shape is immediately visible.
func (m Model) applyProfileEditorResult(msg profileEditorFinishedMsg) (tea.Model, tea.Cmd) {
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
	var p loadtest.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		m.statusMsg = "profile parse failed: " + err.Error()
		return m, nil
	}
	if err := m.loadStore.Save(msg.name, p); err != nil {
		m.statusMsg = "profile save failed: " + err.Error()
		return m, nil
	}
	m.statusMsg = "saved load profile " + msg.name
	if m.loadRunning() {
		return m, nil // don't yank the pane away from a live run
	}
	next, cmd := m.openLoadPicker()
	nm := next.(Model)
	if nm.loadPicker {
		for i, item := range nm.pickerItems {
			if item.Name == msg.name {
				nm.pickerIdx = i
				break
			}
		}
		nm.statusMsg = "saved load profile " + msg.name + " — ⏎ runs it"
	}
	return nm, cmd
}

// confirmLoadTest arms the y/n prompt, spelling out exactly what is about to
// be fired at which target — a spike aimed at the wrong URL is the classic
// load-testing footgun.
func (m Model) confirmLoadTest(p loadtest.Profile) Model {
	built := m.buildRequest()
	if strings.TrimSpace(built.URL) == "" {
		m.statusMsg = "cannot load test: URL is empty"
		return m
	}
	m.loadConfirm = true
	m.pendingProfile = p
	m.statusMsg = fmt.Sprintf("run %q — peak %.0f rps · up to %d req · %s against %s %s? (y/n)",
		p.Name, p.Peak(), p.PlannedRequests(), formatRunDuration(p.Duration()),
		built.Method, truncateMiddle(built.URL, 40))
	return m
}

// resolveLoadConfirm handles the key pressed while the confirm is armed.
func (m Model) resolveLoadConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.pendingProfile
	m.loadConfirm = false
	m.pendingProfile = loadtest.Profile{}
	if msg.String() != "y" {
		m.statusMsg = "load test cancelled"
		return m, nil
	}
	return m.startLoadTest(p)
}

// startLoadTest fires the run: the current request is frozen (variables
// expanded, auth applied, query folded) and handed to the engine's workers.
func (m Model) startLoadTest(p loadtest.Profile) (tea.Model, tea.Cmd) {
	if m.sending {
		m.statusMsg = "a request is in flight — esc to cancel it first"
		return m, nil
	}
	if m.loadRunning() {
		m.statusMsg = "load test already running — esc to stop it first"
		return m, nil
	}
	built := m.buildRequest()
	run, err := loadtest.Runner{
		Profile: p,
		Do: func(ctx context.Context) (int, error) {
			return httpx.DoLoad(ctx, built)
		},
	}.Start(context.Background())
	if err != nil {
		m.statusMsg = "load test failed to start: " + err.Error()
		return m, nil
	}
	m.loadRun = run
	m.loadProfile = p
	m.loadTarget = built
	m.loadStartedAt = time.Now()
	m.loadSnap = loadtest.Snapshot{}
	m.loadSummary = nil
	m.loadStopped = false
	m.loadSeq++
	m = m.setFocus(focusResponse)
	m.statusMsg = fmt.Sprintf("load test %q started — esc to stop", p.Name)
	return m, loadTick(m.loadSeq)
}

// handleLoadTick polls the run and keeps ticking until it reports done.
func (m Model) handleLoadTick(msg loadTickMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.loadSeq || m.loadRun == nil {
		return m, nil // a stale tick from a stopped or replaced run
	}
	m.loadSnap = m.loadRun.Snapshot()
	if m.loadSnap.Done {
		verb := "finished"
		if m.loadStopped {
			verb = "stopped"
		}
		saved := ""
		if m.loadSummary == nil { // first tick after completion: summarize once
			sum := loadtest.Summarize(m.loadProfile, m.loadTarget.Method, m.loadTarget.URL,
				m.loadStartedAt, m.loadSnap, m.loadStopped)
			m.loadSummary = &sum
			if name, err := m.resultStore.Save(sum); err != nil {
				saved = " · result save failed: " + err.Error()
			} else {
				saved = " · saved " + name
			}
		}
		m.statusMsg = fmt.Sprintf("load test %s%s — esc to close · T to rerun", verb, saved)
		return m, nil
	}
	return m, loadTick(m.loadSeq)
}

// stopLoadTest aborts a running test (in-flight requests are cancelled); on an
// already-finished run it dismisses the results view instead.
func (m Model) stopLoadTest() (tea.Model, tea.Cmd) {
	if m.loadRun == nil {
		return m, nil
	}
	if m.loadRunning() {
		m.loadRun.Stop()
		m.loadStopped = true
		m.statusMsg = "stopping load test…"
		return m, nil // the tick loop observes Done and reports
	}
	m.loadRun = nil
	m.statusMsg = ""
	return m, nil
}

// --- rendering -------------------------------------------------------------

// sparkBlocks are the eight partial-height cells a sparkline is built from.
var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// sparkline renders vals as one row of block characters, scaled to the series
// maximum. Zero renders as an empty cell so quiet seconds stay visually quiet.
func sparkline(vals []float64, width int) string {
	if width <= 0 || len(vals) == 0 {
		return ""
	}
	vals = resample(vals, width)
	max := 0.0
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range vals {
		switch {
		case max <= 0 || v <= 0:
			b.WriteRune(' ')
		default:
			// ceil maps any nonzero value to at least the lowest block and the
			// maximum to the full block, proportionally in between.
			i := int(math.Ceil(v/max*float64(len(sparkBlocks)))) - 1
			if i >= len(sparkBlocks) {
				i = len(sparkBlocks) - 1
			}
			if i < 0 {
				i = 0
			}
			b.WriteRune(sparkBlocks[i])
		}
	}
	return b.String()
}

// resample squeezes or stretches vals to exactly width entries (max-pooling
// when squeezing, so short spikes are never averaged away).
func resample(vals []float64, width int) []float64 {
	if len(vals) == width {
		return vals
	}
	out := make([]float64, width)
	for i := 0; i < width; i++ {
		lo := i * len(vals) / width
		hi := (i + 1) * len(vals) / width
		if hi <= lo {
			hi = lo + 1
		}
		if hi > len(vals) {
			hi = len(vals)
		}
		max := 0.0
		for _, v := range vals[lo:hi] {
			if v > max {
				max = v
			}
		}
		out[i] = max
	}
	return out
}

// targetSeries samples the profile's target rate once per second across its
// full duration, for the shape preview and the target row of the run chart.
func targetSeries(p loadtest.Profile) []float64 {
	secs := int(p.Duration() / time.Second)
	if secs < 1 {
		secs = 1
	}
	out := make([]float64, secs)
	for i := range out {
		// Sample mid-second so an instantaneous jump doesn't misrepresent the
		// whole first second of a plateau.
		out[i] = p.TargetAt(time.Duration(i)*time.Second + time.Second/2)
	}
	return out
}

// achievedSeries is completions per second from the run's buckets, padded to
// the profile's full duration so the chart timeline matches the target row.
func achievedSeries(snap loadtest.Snapshot, secs int) []float64 {
	if secs < len(snap.Buckets) {
		secs = len(snap.Buckets)
	}
	out := make([]float64, secs)
	for i, b := range snap.Buckets {
		out[i] = float64(b.Completed)
	}
	return out
}

// formatRunDuration trims the zero tails Go puts on round durations — "1m0s"
// reads as "1m", "2h0m0s" as "2h" — without touching plain seconds like "30s".
func formatRunDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// viewLoadPicker renders the profile chooser in the response pane: the list on
// top, a preview of the highlighted shape below it.
func (m Model) viewLoadPicker() string {
	width := m.vp.Width
	lines := []string{
		title("LOAD TEST") + dim(" — choose a profile"),
		"",
	}
	for i, p := range m.pickerItems {
		marker, st := "  ", lipgloss.NewStyle().Foreground(colFg)
		if i == m.pickerIdx {
			marker, st = "› ", lipgloss.NewStyle().Foreground(colSelFg).Background(colSel).Bold(true)
		}
		row := fmt.Sprintf("%-14s %s", truncateMiddle(p.Name, 14), p.Description)
		lines = append(lines, marker+st.Render(truncateRunes(row, width-2)))
	}
	p := m.pickerItems[m.pickerIdx]
	lines = append(lines,
		"",
		dim("shape"),
		keyHint(sparkline(targetSeries(p), min(width, 48))),
		dim(fmt.Sprintf("peak %.0f rps · %s · %d req total",
			p.Peak(), formatRunDuration(p.Duration()), p.PlannedRequests())),
		"",
		dim("⏎ run against the current request · e edit shape · E edit JSON · n new · esc cancel"),
	)
	return strings.Join(lines, "\n")
}

// viewLoadRun renders the live (or final) run view in the response pane.
// During an active drag-selection the styled view yields to the plain-text
// grid with the selected span reversed, so the span math and the text that
// lands on the clipboard agree exactly with what's on screen.
func (m Model) viewLoadRun() string {
	if m.selecting && m.selLoad {
		return renderWithSelection(m.loadRunPlainText(), m.selAnchor, m.selCursor)
	}
	return m.viewLoadRunStyled()
}

// loadRunPlainText is the run view as an unstyled character grid — the source
// for drag-selection and clipboard copies.
func (m Model) loadRunPlainText() string {
	return stripStyles(m.viewLoadRunStyled())
}

// loadRunHeader builds the run view's header row. On a finished run the copy
// pill is right-aligned on the row; when it can't fit beside the full header
// the elapsed/total clock is shed first, and the pill is dropped only if even
// that is too wide. The bool reports whether the pill made it onto the row so
// click hit-testing agrees with what's rendered.
func (m Model) loadRunHeader(width int) (string, bool) {
	p, snap := m.loadProfile, m.loadSnap

	state := lipgloss.NewStyle().Foreground(colOK).Render("running")
	switch {
	case snap.Done && m.loadStopped:
		state = lipgloss.NewStyle().Foreground(colMethod).Render("stopped")
	case snap.Done:
		state = lipgloss.NewStyle().Foreground(colOK).Bold(true).Render("done")
	}
	base := title("LOAD TEST ") + lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(p.Name) +
		"  " + state
	// The live elapsed clock is truncated to 100ms — raw time.Since output
	// would flicker nanosecond noise ("2.000418958s") in the header.
	head := base + dim(fmt.Sprintf("  %s / %s", formatRunDuration(snap.Elapsed.Truncate(100*time.Millisecond)),
		formatRunDuration(p.Duration())))
	if !snap.Done || m.loadSummary == nil {
		return head, false
	}
	pill := m.copyButtonView()
	pillW := lipgloss.Width(pill)
	if pad := width - lipgloss.Width(head) - pillW; pad > 0 {
		return head + strings.Repeat(" ", pad) + pill, true
	}
	if pad := width - lipgloss.Width(base) - pillW; pad > 0 {
		return base + strings.Repeat(" ", pad) + pill, true
	}
	return head, false
}

func (m Model) viewLoadRunStyled() string {
	width := m.vp.Width
	p, snap := m.loadProfile, m.loadSnap
	head, _ := m.loadRunHeader(width)

	lines := []string{head}
	if !snap.Done {
		frac := 0.0
		if total := p.Duration(); total > 0 {
			frac = float64(snap.Elapsed) / float64(total)
		}
		lines = append(lines, " "+loadProgressBar(frac, min(width-8, 46)))
	}
	lines = append(lines, "")

	if snap.Done && m.loadSummary != nil {
		// The finished view swaps the live counters for the k6-style analysis.
		lines = append(lines, styleSummary(*m.loadSummary)...)
	} else {
		lines = append(lines, m.viewLiveCounters(p, snap)...)
	}

	// Right-hand chart annotations get whatever the widest note actually
	// needs — a clipped "max 3ms" would read as minutes. On panes too narrow
	// to fit a useful sparkline beside them, the annotations are dropped.
	notes := [3]string{
		fmt.Sprintf("peak %.0f/s", p.Peak()),
		fmt.Sprintf("%.1f/s avg", snap.AchievedRPS),
		fmt.Sprintf("mean/s · max %s", snap.Max.Round(time.Millisecond)),
	}
	noteW := 0
	for _, n := range notes {
		noteW = max(noteW, lipgloss.Width(n)+2)
	}
	chartW := min(width-10-noteW, 48)
	annotate := chartW >= 16
	if !annotate {
		chartW = min(width-11, 48)
	}
	target := targetSeries(p)
	achieved := achievedSeries(snap, len(target))
	latency := latencySeries(snap, len(target))
	note := func(s string) string {
		if !annotate {
			return ""
		}
		return dim("  " + s)
	}
	latStyle := lipgloss.NewStyle().Foreground(colMethod)
	chart := []string{
		dim(fmt.Sprintf("%-9s ", "target")) + dim(sparkline(target, chartW)) + note(notes[0]),
		dim(fmt.Sprintf("%-9s ", "achieved")) + keyHint(sparkline(achieved, chartW)) + note(notes[1]),
		dim(fmt.Sprintf("%-9s ", "latency")) + latStyle.Render(sparkline(latency, chartW)) + note(notes[2]),
	}

	foot := dim("esc stop")
	if snap.Done {
		foot = dim("esc close · T rerun · y copy · drag selects text")
	}

	lines = append(lines, "")
	lines = append(lines, chart...)
	lines = append(lines, "", foot)
	out := strings.Join(lines, "\n")
	return lipgloss.NewStyle().MaxWidth(width).Render(out)
}

// viewLiveCounters is the in-flight stats block: dim labels, status-colored
// values — green ok, red errors, amber drops — never color without its label.
func (m Model) viewLiveCounters(p loadtest.Profile, snap loadtest.Snapshot) []string {
	value := lipgloss.NewStyle().Foreground(colFg)
	stat := func(label, v string, st lipgloss.Style) string { return dim(label+" ") + st.Render(v) }
	zeroOr := func(n int, hot lipgloss.AdaptiveColor) lipgloss.Style {
		if n > 0 {
			return lipgloss.NewStyle().Foreground(hot)
		}
		return lipgloss.NewStyle().Foreground(colDim)
	}

	workers := p.MaxWorkers
	if workers <= 0 {
		workers = loadtest.DefaultMaxWorkers
	}
	counts := " " + stat("ok", fmt.Sprintf("%d", snap.Completed-snap.Errors), lipgloss.NewStyle().Foreground(colOK)) +
		dim(" · ") + stat("err", fmt.Sprintf("%d", snap.Errors), zeroOr(snap.Errors, colErr)) +
		dim(" · ") + stat("cancel", fmt.Sprintf("%d", snap.Canceled), lipgloss.NewStyle().Foreground(colDim)) +
		dim(" · ") + stat("drop", fmt.Sprintf("%d", snap.Dropped), zeroOr(snap.Dropped, colMethod)) +
		dim(" · ") + stat("in-flight", fmt.Sprintf("%d/%d", snap.InFlight, workers), value)

	rates := " " + stat("rps", fmt.Sprintf("%.1f", snap.AchievedRPS), value) + dim(" achieved · ") +
		value.Render(fmt.Sprintf("%.1f", p.TargetAt(snap.Elapsed))) + dim(" target now")

	ms := func(d time.Duration) string { return d.Round(time.Millisecond).String() }
	lat := " " + stat("p50", ms(snap.P50), value) + dim(" · ") +
		stat("p90", ms(snap.P90), value) + dim(" · ") +
		stat("p95", ms(snap.P95), value) + dim(" · ") +
		stat("p99", ms(snap.P99), value) + dim(" · ") +
		stat("max", ms(snap.Max), value)

	return []string{counts, rates, lat}
}

// loadProgressBar draws run progress as an accent-filled bar with a percent
// readout. frac is clamped to [0, 1].
func loadProgressBar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	if width < 8 {
		width = 8
	}
	filled := int(frac*float64(width) + 0.5)
	bar := lipgloss.NewStyle().Foreground(colAccent).Render(strings.Repeat("█", filled)) +
		dim(strings.Repeat("░", width-filled))
	return bar + dim(fmt.Sprintf(" %3.0f%%", frac*100))
}

// latencySeries is per-second mean latency in milliseconds, aligned with the
// target/achieved series so the three sparklines share a time axis.
func latencySeries(snap loadtest.Snapshot, secs int) []float64 {
	if secs < len(snap.Buckets) {
		secs = len(snap.Buckets)
	}
	out := make([]float64, secs)
	for i, b := range snap.Buckets {
		out[i] = float64(b.MeanLatency()) / float64(time.Millisecond)
	}
	return out
}

// yankLoadSummary copies the finished run's k6-style analysis to the
// clipboard — the y key and the header copy pill both land here.
func (m Model) yankLoadSummary() (tea.Model, tea.Cmd) {
	if m.loadSummary == nil {
		return m, nil
	}
	if err := clipboard.WriteAll(m.loadSummary.Render()); err != nil {
		m.statusMsg = "clipboard unavailable"
	} else {
		m.statusMsg = "copied run analysis to clipboard"
	}
	return m, nil
}

// styleSummary colors the k6-style analysis block for the results view: the
// verdict line green (clean) or warning-toned (errors/drops), dotted labels
// dimmed, values in the default foreground.
func styleSummary(s loadtest.Summary) []string {
	raw := strings.Split(s.Render(), "\n")
	headStyle := lipgloss.NewStyle().Foreground(colOK).Bold(true)
	if s.Errors > 0 || s.Dropped > 0 {
		headStyle = lipgloss.NewStyle().Foreground(colMethod).Bold(true)
	}
	out := []string{headStyle.Render(raw[0])}
	for _, ln := range raw[1:] {
		if label, value, ok := strings.Cut(ln, ": "); ok {
			out = append(out, dim(label+": ")+value)
		} else {
			out = append(out, ln)
		}
	}
	return out
}
