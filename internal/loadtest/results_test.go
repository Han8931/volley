package loadtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testSummary(profile string, start time.Time) Summary {
	p := Constant(10, 30*time.Second)
	p.Name = profile
	return Summarize(p, "GET", "https://api.test/ping", start, testSnapshot(), false)
}

func TestResultStoreSaveList(t *testing.T) {
	rs := ResultStore{Root: filepath.Join(t.TempDir(), "loadresults")}

	older := testSummary("steady", time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC))
	newer := testSummary("steady", time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC))
	other := testSummary("spike", time.Date(2026, 7, 19, 10, 30, 0, 0, time.UTC))
	for _, s := range []Summary{older, newer, other} {
		name, err := rs.Save(s)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(name, ".json") || !strings.HasPrefix(name, s.Profile+"-") {
			t.Errorf("file name = %q", name)
		}
	}

	all, err := rs.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("List returned %d summaries, want 3", len(all))
	}
	if !all[0].StartedAt.Equal(newer.StartedAt) {
		t.Errorf("List should be newest first, got %v", all[0].StartedAt)
	}

	latest, ok := rs.Latest("steady")
	if !ok || !latest.StartedAt.Equal(newer.StartedAt) {
		t.Errorf("Latest(steady) = %+v, %v", latest, ok)
	}
	if _, ok := rs.Latest("nope"); ok {
		t.Error("Latest of an unknown profile should report false")
	}
}

func TestResultStoreListMissingRoot(t *testing.T) {
	rs := ResultStore{Root: filepath.Join(t.TempDir(), "never-created")}
	all, err := rs.List()
	if err != nil || all != nil {
		t.Errorf("missing root should list empty, got %v, %v", all, err)
	}
}

func TestResultStoreSkipsCorruptFile(t *testing.T) {
	rs := ResultStore{Root: t.TempDir()}
	if _, err := rs.Save(testSummary("steady", time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rs.Root, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := rs.List()
	if err != nil || len(all) != 1 {
		t.Errorf("corrupt file should be skipped, got %d summaries, %v", len(all), err)
	}
}

func TestSanitizeResultName(t *testing.T) {
	cases := map[string]string{
		"steady":       "steady",
		"group/steady": "group-steady",
		"../escape":    "--escape",
		"  ":           "run",
	}
	for in, want := range cases {
		if got := sanitizeResultName(in); got != want {
			t.Errorf("sanitizeResultName(%q) = %q, want %q", in, got, want)
		}
	}
}
