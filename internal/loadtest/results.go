package loadtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ResultStore persists run summaries as JSON files below Root — one file per
// finished run, named <profile>-<timestamp>.json, newest discoverable by
// name. Files are flat: profile names containing slashes are folded so a
// result never lands outside Root.
type ResultStore struct{ Root string }

// DefaultResultStore returns the user's Volley load-results directory,
// beside collections and loadprofiles.
func DefaultResultStore() ResultStore {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return ResultStore{Root: filepath.Join(base, "volley", "loadresults")}
}

// FileName is the deterministic name Save stores s under.
func (rs ResultStore) FileName(s Summary) string {
	return fmt.Sprintf("%s-%s.json",
		sanitizeResultName(s.Profile), s.StartedAt.Format("20060102-150405"))
}

// Delete removes a stored result by its file name (as returned by Save or
// FileName). Names with path separators are rejected — results are flat.
func (rs ResultStore) Delete(name string) error {
	if name != filepath.Base(name) || name == "." || name == "" {
		return fmt.Errorf("invalid result name: %s", name)
	}
	return os.Remove(filepath.Join(rs.Root, name))
}

// Save writes s as an indented JSON file and returns the file name it chose.
func (rs ResultStore) Save(s Summary) (string, error) {
	if err := os.MkdirAll(rs.Root, 0o755); err != nil {
		return "", err
	}
	name := rs.FileName(s)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(filepath.Join(rs.Root, name), append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// List returns every stored summary, newest first. A corrupt file is skipped
// rather than failing the listing, matching the profile store's tolerance.
func (rs ResultStore) List() ([]Summary, error) {
	entries, err := os.ReadDir(rs.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(rs.Root, e.Name()))
		if err != nil {
			continue
		}
		var s Summary
		if err := json.Unmarshal(b, &s); err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

// Latest returns the newest stored summary for profile, or false when none
// exists — the natural baseline for a "compare with previous run".
func (rs ResultStore) Latest(profile string) (Summary, bool) {
	all, err := rs.List()
	if err != nil {
		return Summary{}, false
	}
	for _, s := range all {
		if s.Profile == profile {
			return s, true
		}
	}
	return Summary{}, false
}

// sanitizeResultName folds path separators and blanks so a profile name is
// always a safe flat file-name component.
func sanitizeResultName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, string(filepath.Separator), "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "..", "-")
	if name == "" {
		name = "run"
	}
	return name
}
