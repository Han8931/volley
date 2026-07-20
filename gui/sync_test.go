package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSyncRoundTrip drives setup → commit → push against a local bare repo
// standing in for GitHub. Skipped when git isn't installed.
func TestSyncRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	a := testApp(t)
	root := a.syncRoot()
	if err := os.MkdirAll(a.store.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveRequest("demo/ping", RequestDTO{Method: "GET", URL: "https://x.test"}); err != nil {
		t.Fatal(err)
	}

	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("bare init: %v %s", err, out)
	}

	st := a.SyncStatus()
	if st.Configured {
		t.Fatal("fresh dir must not report configured")
	}

	// Pre-init with a repo-local committer identity so SyncSetup's initial
	// commit works on machines without global git config.
	if out, err := exec.Command("git", "-C", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	for _, kv := range [][2]string{{"user.email", "volley@test"}, {"user.name", "volley test"}} {
		if _, err := a.git("config", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}

	st, err := a.SyncSetup(bare)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Configured || st.Remote != bare {
		t.Fatalf("after setup: %+v", st)
	}
	if b, err := os.ReadFile(filepath.Join(root, ".gitignore")); err != nil || !strings.Contains(string(b), "environments/") {
		t.Errorf("setup should gitignore environments/: %v %s", err, b)
	}

	// First sync pushes the initial state (the setup commit may have failed
	// pre-identity; SyncNow commits whatever is pending).
	report, err := a.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Pushed {
		t.Fatalf("first sync should push: %+v", report)
	}

	// A new request syncs as a new commit on the remote.
	if err := a.SaveRequest("demo/two", RequestDTO{Method: "GET", URL: "https://y.test"}); err != nil {
		t.Fatal(err)
	}
	report, err = a.SyncNow()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Committed || !report.Pushed {
		t.Fatalf("second sync: %+v", report)
	}
	out, err := exec.Command("git", "-C", bare, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("remote log: %v %s", err, out)
	}
	if got := strings.Count(strings.TrimSpace(string(out)), "\n") + 1; got < 2 {
		t.Errorf("remote should have >= 2 commits, log:\n%s", out)
	}

	// Environments must never reach the remote.
	if _, err := a.SaveEnvironment("prod", map[string]string{"tok": "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncNow(); err != nil {
		t.Fatal(err)
	}
	lsOut, _ := exec.Command("git", "-C", bare, "ls-tree", "-r", "--name-only", "HEAD").CombinedOutput()
	if strings.Contains(string(lsOut), "environments/") {
		t.Errorf("environments leaked to the remote:\n%s", lsOut)
	}
}
