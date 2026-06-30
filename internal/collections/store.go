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

// Item is one saved request shown in the collections tree.
type Item struct {
	Name string // slash-separated name without extension, e.g. "auth/login"
	Path string // absolute file path
}

// Store stores requests as JSON files below Root.
type Store struct{ Root string }

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
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		out = append(out, Item{Name: name, Path: path})
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
	b, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
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
	var req model.Request
	if err := json.Unmarshal(b, &req); err != nil {
		return model.Request{}, err
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	return req, nil
}

// Copy duplicates a saved request.
func (s Store) Copy(oldName, newName string) error {
	req, err := s.Load(oldName)
	if err != nil {
		return err
	}
	return s.Save(newName, req)
}

// Rename moves a saved request to a new name.
func (s Store) Rename(oldName, newName string) error {
	oldPath, err := s.pathFor(oldName)
	if err != nil {
		return err
	}
	newPath, err := s.pathFor(newName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

// Delete removes a saved request.
func (s Store) Delete(name string) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s Store) pathFor(name string) (string, error) {
	name = strings.TrimSpace(name)
	if filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("invalid request name: %s", name)
	}
	name = strings.Trim(name, "/")
	if name == "" {
		return "", fmt.Errorf("empty request name")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid request name: %s", name)
	}
	return filepath.Join(s.Root, clean+".json"), nil
}
