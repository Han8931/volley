package loadtest

import (
	"fmt"
	"strings"
	"time"
)

// Summary is the durable record of one finished run: the numbers a k6-style
// analysis prints and a future comparison needs, independent of live state.
// Durations serialize as strings ("42ms"), matching the profile format.
type Summary struct {
	Profile   string    `json:"profile"`
	Method    string    `json:"method"`
	URL       string    `json:"url"`
	StartedAt time.Time `json:"startedAt"`
	Elapsed   Duration  `json:"elapsed"`
	Stopped   bool      `json:"stopped,omitempty"` // ended early via esc

	Planned   int `json:"planned"` // arrivals the profile intended
	Sent      int `json:"sent"`
	Completed int `json:"completed"`
	OK        int `json:"ok"`
	Errors    int `json:"errors"` // transport errors + 5xx
	Canceled  int `json:"canceled,omitempty"`
	Dropped   int `json:"dropped,omitempty"`

	TargetPeakRPS float64 `json:"targetPeakRps"`
	AchievedRPS   float64 `json:"achievedRps"`

	LatencyMin  Duration `json:"latencyMin"`
	LatencyMean Duration `json:"latencyMean"`
	LatencyP50  Duration `json:"latencyP50"`
	LatencyP90  Duration `json:"latencyP90"`
	LatencyP95  Duration `json:"latencyP95"`
	LatencyP99  Duration `json:"latencyP99"`
	LatencyMax  Duration `json:"latencyMax"`

	// StatusClasses counts completions by response class: index 0 is
	// transport errors (no response), 1..5 are 1xx..5xx.
	StatusClasses [6]int `json:"statusClasses"`
}

// Summarize converts a finished run's snapshot into its durable Summary.
func Summarize(p Profile, method, url string, startedAt time.Time, snap Snapshot, stopped bool) Summary {
	return Summary{
		Profile:   p.Name,
		Method:    method,
		URL:       url,
		StartedAt: startedAt,
		Elapsed:   Duration(snap.Elapsed),
		Stopped:   stopped,

		Planned:   p.PlannedRequests(),
		Sent:      snap.Sent,
		Completed: snap.Completed,
		OK:        snap.Completed - snap.Errors,
		Errors:    snap.Errors,
		Canceled:  snap.Canceled,
		Dropped:   snap.Dropped,

		TargetPeakRPS: p.Peak(),
		AchievedRPS:   snap.AchievedRPS,

		LatencyMin:  Duration(snap.Min),
		LatencyMean: Duration(snap.Mean),
		LatencyP50:  Duration(snap.P50),
		LatencyP90:  Duration(snap.P90),
		LatencyP95:  Duration(snap.P95),
		LatencyP99:  Duration(snap.P99),
		LatencyMax:  Duration(snap.Max),

		StatusClasses: snap.StatusClasses,
	}
}

// ErrorRate is the fraction of completed requests that failed, in [0, 1].
func (s Summary) ErrorRate() float64 {
	if s.Completed == 0 {
		return 0
	}
	return float64(s.Errors) / float64(s.Completed)
}

// Render prints the summary as an aligned, k6-style analysis block. Plain
// text — callers style it (the TUI dims labels, a future CLI prints as-is).
func (s Summary) Render() string {
	mark := "✓"
	if s.Errors > 0 || s.Dropped > 0 {
		mark = "✗"
	}
	head := fmt.Sprintf("%s %s — %s %s  (%s", mark, s.Profile, s.Method, s.URL,
		time.Duration(s.Elapsed).Round(time.Millisecond))
	if s.Stopped {
		head += ", stopped early"
	}
	head += ")"

	rows := [][2]string{
		{"requests", fmt.Sprintf("%d sent of %d planned · %d completed", s.Sent, s.Planned, s.Completed)},
		{"ok / error", fmt.Sprintf("%d / %d (%.1f%% errors)", s.OK, s.Errors, 100*s.ErrorRate())},
	}
	if s.Canceled > 0 || s.Dropped > 0 {
		rows = append(rows, [2]string{"cancelled / dropped",
			fmt.Sprintf("%d / %d", s.Canceled, s.Dropped)})
	}
	rows = append(rows,
		[2]string{"rps", fmt.Sprintf("%.1f achieved · %.0f peak target", s.AchievedRPS, s.TargetPeakRPS)},
		[2]string{"latency", fmt.Sprintf("min %s · avg %s · max %s",
			fmtLatency(s.LatencyMin), fmtLatency(s.LatencyMean), fmtLatency(s.LatencyMax))},
		[2]string{"percentiles", fmt.Sprintf("p50 %s · p90 %s · p95 %s · p99 %s",
			fmtLatency(s.LatencyP50), fmtLatency(s.LatencyP90),
			fmtLatency(s.LatencyP95), fmtLatency(s.LatencyP99))},
	)
	if line := s.statusLine(); line != "" {
		rows = append(rows, [2]string{"status", line})
	}

	width := 0
	for _, r := range rows {
		if len(r[0]) > width {
			width = len(r[0])
		}
	}
	var b strings.Builder
	b.WriteString(head)
	for _, r := range rows {
		b.WriteString("\n  ")
		b.WriteString(r[0])
		b.WriteString(strings.Repeat(".", width-len(r[0])+2))
		b.WriteString(": ")
		b.WriteString(r[1])
	}
	return b.String()
}

// statusLine lists the non-zero response classes, e.g. "2xx 987 · 5xx 13".
// Transport failures (class 0) are labelled "net".
func (s Summary) statusLine() string {
	var parts []string
	for class, n := range s.StatusClasses {
		if n == 0 {
			continue
		}
		label := "net"
		if class > 0 {
			label = fmt.Sprintf("%dxx", class)
		}
		parts = append(parts, fmt.Sprintf("%s %d", label, n))
	}
	return strings.Join(parts, " · ")
}

// fmtLatency renders a latency rounded to a readable precision: microseconds
// under 1ms, otherwise tenths of a millisecond and coarser as it grows.
func fmtLatency(d Duration) string {
	v := time.Duration(d)
	switch {
	case v <= 0:
		return "0s"
	case v < time.Millisecond:
		return v.Round(time.Microsecond).String()
	case v < time.Second:
		return v.Round(time.Millisecond / 10).String()
	default:
		return v.Round(time.Millisecond).String()
	}
}
