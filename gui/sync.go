package main

// sync.go — git-based sync for the volley config dir (collections, load
// profiles), Bruno-style: the store directory is a git repo pushed to a
// remote the user owns. Shells out to the system git so the user's existing
// credentials (ssh keys, credential helpers) just work.
//
// environments/ and loadresults/ are gitignored by default: environments
// hold tokens, and results are machine-local run data.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const syncIgnore = `# volley sync — secrets and machine-local data stay out of the repo
environments/
loadresults/
`

type SyncStateDTO struct {
	GitInstalled bool   `json:"gitInstalled"`
	Configured   bool   `json:"configured"` // the config dir is a git repo
	Remote       string `json:"remote"`     // origin URL, "" if none
	Branch       string `json:"branch"`
	Dirty        int    `json:"dirty"` // changed paths not yet committed
	Root         string `json:"root"`
}

type SyncReportDTO struct {
	Committed bool   `json:"committed"`
	Pushed    bool   `json:"pushed"`
	Detail    string `json:"detail"`
}

// git runs one git command against the sync root with a hard timeout, so a
// hung network push can't wedge a binding call forever.
func (a *App) git(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", a.syncRoot()}, args...)...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("git %s: %s", args[0], text)
	}
	return text, nil
}

// syncRoot is the volley config dir — the parent of the collections store.
func (a *App) syncRoot() string {
	return filepath.Dir(a.store.Root)
}

func (a *App) SyncStatus() SyncStateDTO {
	st := SyncStateDTO{Root: a.syncRoot()}
	if _, err := exec.LookPath("git"); err != nil {
		return st
	}
	st.GitInstalled = true
	if _, err := os.Stat(filepath.Join(a.syncRoot(), ".git")); err != nil {
		return st
	}
	st.Configured = true
	if url, err := a.git("remote", "get-url", "origin"); err == nil {
		st.Remote = url
	}
	if branch, err := a.git("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		st.Branch = branch
	}
	if out, err := a.git("status", "--porcelain"); err == nil && out != "" {
		st.Dirty = len(strings.Split(out, "\n"))
	}
	return st
}

// SyncSetup turns the config dir into a git repo (if it isn't yet) pointed
// at remote, writes the default .gitignore, and makes the initial commit.
func (a *App) SyncSetup(remote string) (SyncStateDTO, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return SyncStateDTO{}, errors.New("git is not installed (or not on PATH)")
	}
	root := a.syncRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return SyncStateDTO{}, err
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		if _, err := a.git("init", "-b", "main"); err != nil {
			// Older git without -b: fall back to plain init.
			if _, err2 := a.git("init"); err2 != nil {
				return SyncStateDTO{}, err
			}
		}
	}
	ignorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(ignorePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ignorePath, []byte(syncIgnore), 0o644); err != nil {
			return SyncStateDTO{}, err
		}
	}
	remote = strings.TrimSpace(remote)
	if remote != "" {
		if _, err := a.git("remote", "get-url", "origin"); err != nil {
			if _, err := a.git("remote", "add", "origin", remote); err != nil {
				return SyncStateDTO{}, err
			}
		} else if _, err := a.git("remote", "set-url", "origin", remote); err != nil {
			return SyncStateDTO{}, err
		}
	}
	if _, err := a.git("add", "-A"); err != nil {
		return SyncStateDTO{}, err
	}
	if out, _ := a.git("status", "--porcelain"); out != "" {
		if _, err := a.git("commit", "-m", "volley: initial sync"); err != nil {
			return SyncStateDTO{}, err
		}
	}
	return a.SyncStatus(), nil
}

// SyncNow commits local changes, rebases on the remote, and pushes — one
// button, like Bruno's git sync. A missing or empty remote branch is fine:
// the first push creates it.
func (a *App) SyncNow() (SyncReportDTO, error) {
	st := a.SyncStatus()
	if !st.GitInstalled {
		return SyncReportDTO{}, errors.New("git is not installed (or not on PATH)")
	}
	if !st.Configured {
		return SyncReportDTO{}, errors.New("sync is not set up yet — configure it in Settings")
	}
	report := SyncReportDTO{}
	if _, err := a.git("add", "-A"); err != nil {
		return report, err
	}
	if out, _ := a.git("status", "--porcelain"); out != "" {
		msg := "volley: sync " + time.Now().Format("2006-01-02 15:04:05")
		if _, err := a.git("commit", "-m", msg); err != nil {
			return report, err
		}
		report.Committed = true
	}
	if st.Remote == "" {
		report.Detail = "committed locally — no remote configured"
		return report, nil
	}
	// Pull first so a push never needs force. An unborn remote branch makes
	// pull fail with "couldn't find remote ref" — that's fine, push creates it.
	if out, err := a.git("pull", "--rebase", "origin", st.Branch); err != nil &&
		!strings.Contains(out, "couldn't find remote ref") {
		return report, err
	}
	if _, err := a.git("push", "-u", "origin", st.Branch); err != nil {
		return report, err
	}
	report.Pushed = true
	report.Detail = "synced with " + st.Remote
	return report, nil
}
