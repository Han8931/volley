package vars

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

// EnvStore persists named environments as JSON files below Root, following
// the collections and load-profile stores' conventions: one file per
// environment holding a flat {"name": "value"} object, git-friendly indented
// JSON, atomic writes. The file name is the environment's name.
type EnvStore struct{ Root string }

// DefaultEnvStore returns the user's Volley environments directory, beside
// the collections and load-profile directories.
func DefaultEnvStore() EnvStore {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return EnvStore{Root: filepath.Join(base, "volley", "environments")}
}

// List returns every stored environment name, sorted. A missing directory is
// an empty listing, not an error; a corrupt file is skipped like a corrupt
// collection entry.
func (s EnvStore) List() ([]string, error) {
	if _, err := os.Stat(s.Root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []string
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
		if _, err := s.Load(name); err == nil {
			out = append(out, name)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// Load reads an environment's variables by name.
func (s EnvStore) Load(name string) (map[string]string, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var vals map[string]string
	if err := json.Unmarshal(b, &vals); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if vals == nil {
		vals = map[string]string{}
	}
	return vals, nil
}

// Save writes an environment's variables under name. MarshalIndent sorts the
// keys, so the file diffs cleanly under version control.
func (s EnvStore) Save(name string, vals map[string]string) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if vals == nil {
		vals = map[string]string{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(vals, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

// Delete removes a stored environment.
func (s EnvStore) Delete(name string) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// pathFor validates name and returns its JSON file path, rejecting empty,
// absolute, and traversal ("..") names.
func (s EnvStore) pathFor(name string) (string, error) {
	name = strings.TrimSpace(name)
	if filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("invalid environment name: %s", name)
	}
	name = strings.Trim(name, "/")
	if name == "" {
		return "", errors.New("empty environment name")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid environment name: %s", name)
	}
	return filepath.Join(s.Root, clean+".json"), nil
}

// writeFileAtomic lands data at path via a temp file + rename so a crash or
// concurrent read never sees a half-written environment.
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
