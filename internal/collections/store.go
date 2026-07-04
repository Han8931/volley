// Package collections persists saved Volley requests on disk.
package collections

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tabularasa/volley/internal/model"
)

// Item is one entry shown in the collections tree: a saved request (IsDir
// false) or a group/folder (IsDir true).
type Item struct {
	Name  string // slash-separated name without extension, e.g. "auth/login"
	Path  string // absolute file path
	IsDir bool   // true for a group (directory)
}

// Store stores requests as JSON files below Root. Groups are directories;
// an empty group keeps a marker file so it persists and stays visible.
type Store struct{ Root string }

// groupMarker keeps an otherwise-empty group directory alive on disk.
const groupMarker = ".keep"

// DefaultStore returns the user's Volley collections directory.
func DefaultStore() Store {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return Store{Root: filepath.Join(base, "volley", "collections")}
}

// List returns saved requests sorted by name.
func (s Store) List() ([]Item, error) {
	if _, err := os.Stat(s.Root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []Item
	err := filepath.WalkDir(s.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == s.Root {
			return nil
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			out = append(out, Item{Name: relSlash, Path: path, IsDir: true})
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil // skip markers and other files
		}
		out = append(out, Item{Name: strings.TrimSuffix(relSlash, ".json"), Path: path})
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save writes req under name. Name may include folders separated by '/'.
func (s Store) Save(name string, req model.Request) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(toStored(req), "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o644)
}

// Load reads a request by name.
func (s Store) Load(name string) (model.Request, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return model.Request{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return model.Request{}, err
	}
	var sr storedRequest
	if err := json.Unmarshal(b, &sr); err != nil {
		return model.Request{}, err
	}
	return sr.toModel(), nil
}

// Copy duplicates a saved request. It refuses to overwrite an existing name.
func (s Store) Copy(oldName, newName string) error {
	req, err := s.Load(oldName)
	if err != nil {
		return err
	}
	newPath, err := s.pathFor(newName)
	if err != nil {
		return err
	}
	if fileExists(newPath) {
		return fmt.Errorf("%q already exists", newName)
	}
	return s.Save(newName, req)
}

// Rename moves a saved request to a new name. It refuses to overwrite an
// existing different name, and prunes any directory left empty behind it.
func (s Store) Rename(oldName, newName string) error {
	oldPath, err := s.pathFor(oldName)
	if err != nil {
		return err
	}
	newPath, err := s.pathFor(newName)
	if err != nil {
		return err
	}
	if oldPath != newPath && fileExists(newPath) {
		return fmt.Errorf("%q already exists", newName)
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	s.pruneEmptyParents(oldPath)
	return nil
}

// Delete removes a saved request and prunes any directory left empty.
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

// safeRel validates name and returns a cleaned, root-relative path, rejecting
// empty, absolute, and traversal ("..") names.
func (s Store) safeRel(name string) (string, error) {
	name = strings.TrimSpace(name)
	if filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("invalid name: %s", name)
	}
	name = strings.Trim(name, "/")
	if name == "" {
		return "", fmt.Errorf("empty name")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid name: %s", name)
	}
	return clean, nil
}

// pathFor returns the JSON file path for a request name.
func (s Store) pathFor(name string) (string, error) {
	rel, err := s.safeRel(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Root, rel+".json"), nil
}

// dirFor returns the directory path for a group name.
func (s Store) dirFor(name string) (string, error) {
	rel, err := s.safeRel(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Root, rel), nil
}

// CreateGroup makes an (initially empty) group directory that persists.
func (s Store) CreateGroup(name string) error {
	dir, err := s.dirFor(name)
	if err != nil {
		return err
	}
	if fileExists(dir) {
		return fmt.Errorf("%q already exists", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, groupMarker), nil, 0o644)
}

// DeleteGroup removes a group and everything under it.
func (s Store) DeleteGroup(name string) error {
	dir, err := s.dirFor(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	s.pruneEmptyParents(dir)
	return nil
}

// RenameGroup moves a group (and its contents) to a new name.
func (s Store) RenameGroup(oldName, newName string) error {
	oldDir, err := s.dirFor(oldName)
	if err != nil {
		return err
	}
	newDir, err := s.dirFor(newName)
	if err != nil {
		return err
	}
	if oldDir != newDir && fileExists(newDir) {
		return fmt.Errorf("%q already exists", newName)
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}
	s.pruneEmptyParents(oldDir)
	return nil
}
