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

// Store persists load profiles as JSON files below Root, mirroring the
// collections store's conventions: slash-separated names map to nested
// directories, files are git-friendly indented JSON, and writes are atomic.
// The file name is the canonical profile name; the JSON "name" field is
// rewritten to match on load so the two can never drift apart.
type Store struct{ Root string }

// DefaultStore returns the user's Volley load-profiles directory, beside the
// collections directory.
func DefaultStore() Store {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return Store{Root: filepath.Join(base, "volley", "loadprofiles")}
}

// EnsureDefaults seeds the built-in shapes as editable files on first run —
// when the profiles directory does not exist yet. An existing directory is
// left untouched, so deleting or editing a default sticks.
func (s Store) EnsureDefaults() error {
	if _, err := os.Stat(s.Root); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	for _, p := range DefaultProfiles() {
		if err := s.Save(p.Name, p); err != nil {
			return err
		}
	}
	return nil
}

// List returns every stored profile sorted by name. A corrupt or invalid file
// is skipped rather than failing the whole listing, matching how the
// collections tree tolerates one bad entry.
func (s Store) List() ([]Profile, error) {
	if _, err := os.Stat(s.Root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []Profile
	err := filepath.WalkDir(s.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		if p, err := s.Load(name); err == nil {
			out = append(out, p)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save validates and writes p under name (which becomes the profile's Name).
func (s Store) Save(name string, p Profile) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	p.Name = strings.Trim(strings.TrimSpace(name), "/")
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o644)
}

// Load reads and validates a profile by name.
func (s Store) Load(name string) (Profile, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return Profile{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return Profile{}, fmt.Errorf("%s: %w", name, err)
	}
	p.Name = strings.Trim(strings.TrimSpace(name), "/")
	if err := p.Validate(); err != nil {
		return Profile{}, fmt.Errorf("%s: %w", name, err)
	}
	return p, nil
}

// Delete removes a stored profile and prunes any directory left empty.
func (s Store) Delete(name string) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	s.pruneEmptyParents(path)
	return nil
}

// Rename moves a profile to a new name, refusing to overwrite a different
// existing one.
func (s Store) Rename(oldName, newName string) error {
	oldPath, err := s.pathFor(oldName)
	if err != nil {
		return err
	}
	newPath, err := s.pathFor(newName)
	if err != nil {
		return err
	}
	if oldPath != newPath {
		if _, err := os.Stat(newPath); err == nil {
			return fmt.Errorf("%q already exists", newName)
		}
	}
	// Rewrite through Load/Save so the stored "name" field follows the file.
	p, err := s.Load(oldName)
	if err != nil {
		return err
	}
	if err := s.Save(newName, p); err != nil {
		return err
	}
	if oldPath != newPath {
		if err := os.Remove(oldPath); err != nil {
			return err
		}
		s.pruneEmptyParents(oldPath)
	}
	return nil
}

// pruneEmptyParents removes now-empty parent directories of start, walking up
// until (but never removing) Root.
func (s Store) pruneEmptyParents(start string) {
	dir := filepath.Dir(start)
	for strings.HasPrefix(dir, s.Root+string(filepath.Separator)) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// pathFor validates name and returns its JSON file path, rejecting empty,
// absolute, and traversal ("..") names.
func (s Store) pathFor(name string) (string, error) {
	name = strings.TrimSpace(name)
	if filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("invalid profile name: %s", name)
	}
	name = strings.Trim(name, "/")
	if name == "" {
		return "", errors.New("empty profile name")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid profile name: %s", name)
	}
	return filepath.Join(s.Root, clean+".json"), nil
}

// writeFileAtomic lands data at path via a temp file + rename so a crash or
// concurrent read never sees a half-written profile.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".volley-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	return nil
}
