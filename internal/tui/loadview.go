package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

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
	loadStore loadtest.Store // profile files on disk (seeded on first use)

	loadRun     *loadtest.Run     // active or just-finished run; nil = no load view
	loadProfile loadtest.Profile  // profile of the current run
	loadTarget  model.Request     // request the run fires (frozen at start)
	loadSnap    loadtest.Snapshot // latest polled snapshot
	loadSeq     int               // invalidates stale tick messages after stop/restart
	loadStopped bool              // user pressed esc; distinguishes "stopped" from "done"

	loadPicker  bool // profile picker is up in the response pane
	pickerItems []loadtest.Profile
	pickerIdx   int

	loadConfirm    bool // y/n prompt before firing the run
	pendingProfile loadtest.Profile
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
	m.statusMsg = "load profile: j/k choose · ⏎ run · esc cancel"
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
	case "esc", "q":
		m.loadPicker = false
		m.statusMsg = "load test cancelled"
	}
	return m, nil
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
	m.statusMsg = fmt.Sprintf("run %q — peak %.0f rps for %s against %s %s? (y/n)",
		p.Name, p.Peak(), formatRunDuration(p.Duration()),
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
			resp := httpx.Do(ctx, built)
			return resp.StatusCode, resp.Err
		},
	}.Start(context.Background())
	if err != nil {
		m.statusMsg = "load test failed to start: " + err.Error()
		return m, nil
	}
	m.loadRun = run
	m.loadProfile = p
	m.loadTarget = built
	m.loadSnap = loadtest.Snapshot{}
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
		m.statusMsg = fmt.Sprintf("load test %s: %d ok · %d errors · %d dropped — esc to close",
			verb, m.loadSnap.Completed-m.loadSnap.Errors, m.loadSnap.Errors, m.loadSnap.Dropped)
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

// formatRunDuration trims the "0s" tail Go puts on round minutes ("1m0s").
func formatRunDuration(d time.Duration) string {
	s := d.Round(time.Second).String()
	s = strings.TrimSuffix(s, "0s")
	if s == "" {
		s = "0s"
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
			p.Peak(), formatRunDuration(p.Duration()), int(p.ExpectedArrivals(p.Duration())))),
		"",
		dim("⏎ run against the current request · esc cancel"),
	)
	return strings.Join(lines, "\n")
}

// viewLoadRun renders the live (or final) run view in the response pane.
func (m Model) viewLoadRun() string {
	width := m.vp.Width
	p, snap := m.loadProfile, m.loadSnap

	state := lipgloss.NewStyle().Foreground(colOK).Render("running")
	switch {
	case snap.Done && m.loadStopped:
		state = lipgloss.NewStyle().Foreground(colMethod).Render("stopped")
	case snap.Done:
		state = lipgloss.NewStyle().Foreground(colOK).Bold(true).Render("done")
	}
	head := title("LOAD TEST ") + lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(p.Name) +
		"  " + state +
		dim(fmt.Sprintf("  %s / %s", formatRunDuration(snap.Elapsed), formatRunDuration(p.Duration())))

	okCount := snap.Completed - snap.Errors
	counts := fmt.Sprintf(" ok %d · err %d · drop %d · in-flight %d",
		okCount, snap.Errors, snap.Dropped, snap.InFlight)
	countStyle := dim(counts)
	if snap.Errors > 0 || snap.Dropped > 0 {
		countStyle = lipgloss.NewStyle().Foreground(colMethod).Render(counts)
	}

	rates := dim(fmt.Sprintf(" rps %.1f achieved · %.1f target now", snap.AchievedRPS, p.TargetAt(snap.Elapsed)))
	lat := dim(fmt.Sprintf(" p50 %s · p95 %s · p99 %s · max %s",
		snap.P50.Round(time.Millisecond), snap.P95.Round(time.Millisecond),
		snap.P99.Round(time.Millisecond), snap.Max.Round(time.Millisecond)))

	chartW := min(width-11, 60)
	target := targetSeries(p)
	achieved := achievedSeries(snap, len(target))
	chart := []string{
		dim(fmt.Sprintf("%-9s ", "target")) + dim(sparkline(target, chartW)),
		dim(fmt.Sprintf("%-9s ", "achieved")) + keyHint(sparkline(achieved, chartW)),
	}

	foot := dim("esc stop")
	if snap.Done {
		foot = dim("esc close · T run again")
	}

	lines := []string{head, "", countStyle, rates, lat, ""}
	lines = append(lines, chart...)
	lines = append(lines, "", foot)
	out := strings.Join(lines, "\n")
	return lipgloss.NewStyle().MaxWidth(width).Render(out)
}
