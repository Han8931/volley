package loadtest

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testSnapshot() Snapshot {
	return Snapshot{
		Elapsed:     30 * time.Second,
		Done:        true,
		Sent:        300,
		Completed:   290,
		Errors:      13,
		Canceled:    10,
		Dropped:     5,
		AchievedRPS: 9.7,
		P50:         42 * time.Millisecond,
		P90:         101 * time.Millisecond,
		P95:         118 * time.Millisecond,
		P99:         240 * time.Millisecond,
		Min:         12 * time.Millisecond,
		Mean:        48 * time.Millisecond,
		Max:         402 * time.Millisecond,
		StatusClasses: [6]int{
			0, 0, 277, 0, 0, 13,
		},
	}
}

func TestSummarize(t *testing.T) {
	p := Constant(10, 30*time.Second)
	p.Name = "steady"
	s := Summarize(p, "GET", "https://api.test/ping", time.Now(), testSnapshot(), true)

	if s.Profile != "steady" || s.Method != "GET" || !s.Stopped {
		t.Errorf("identity fields wrong: %+v", s)
	}
	if s.OK != 277 || s.Errors != 13 || s.Canceled != 10 || s.Dropped != 5 {
		t.Errorf("counts wrong: %+v", s)
	}
	if s.Planned != p.PlannedRequests() || s.TargetPeakRPS != 10 {
		t.Errorf("profile-derived fields wrong: %+v", s)
	}
	if got := s.ErrorRate(); got < 0.044 || got > 0.045 {
		t.Errorf("ErrorRate = %v, want ~13/290", got)
	}
}

func TestSummaryRender(t *testing.T) {
	p := Constant(10, 30*time.Second)
	p.Name = "steady"
	s := Summarize(p, "GET", "https://api.test/ping", time.Now(), testSnapshot(), false)
	out := s.Render()

	for _, want := range []string{
		"✗ steady — GET https://api.test/ping",
		"290 completed",
		"277 / 13 (4.5% errors)",
		"cancelled / dropped",
		"9.7 achieved · 10 peak target",
		"min 12ms · avg 48ms · max 402ms",
		"p50 42ms · p90 101ms · p95 118ms · p99 240ms",
		"2xx 277 · 5xx 13",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}

	// A clean run gets the checkmark and omits the cancelled/dropped row.
	clean := testSnapshot()
	clean.Errors, clean.Canceled, clean.Dropped = 0, 0, 0
	clean.StatusClasses = [6]int{0, 0, 290, 0, 0, 0}
	cs := Summarize(p, "GET", "https://api.test/ping", time.Now(), clean, false)
	cout := cs.Render()
	if !strings.HasPrefix(cout, "✓") || strings.Contains(cout, "cancelled") {
		t.Errorf("clean render wrong:\n%s", cout)
	}

	// Dotted labels align: every row's colon sits at the same column.
	lines := strings.Split(out, "\n")
	col := -1
	for _, ln := range lines[1:] {
		i := strings.Index(ln, ": ")
		if col == -1 {
			col = i
		}
		if i != col {
			t.Errorf("misaligned row %q (colon at %d, want %d)", ln, i, col)
		}
	}
}

func TestSummaryJSONRoundTrip(t *testing.T) {
	p := Constant(10, 30*time.Second)
	p.Name = "steady"
	s := Summarize(p, "GET", "https://api.test/ping", time.Now().UTC(), testSnapshot(), false)

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"latencyP99":"240ms"`) {
		t.Errorf("durations should serialize as strings: %s", b)
	}
	var back Summary
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back != s {
		t.Errorf("round trip changed the summary:\n got %+v\nwant %+v", back, s)
	}
}

func TestFmtLatency(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{750 * time.Microsecond, "750µs"},
		{42*time.Millisecond + 340*time.Microsecond, "42.3ms"},
		{1200 * time.Millisecond, "1.2s"},
	}
	for _, c := range cases {
		if got := fmtLatency(Duration(c.in)); got != c.want {
			t.Errorf("fmtLatency(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
