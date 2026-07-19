package loadtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempStore(t *testing.T) Store {
	t.Helper()
	return Store{Root: filepath.Join(t.TempDir(), "loadprofiles")}
}

func TestEnsureDefaultsSeedsOnce(t *testing.T) {
	s := tempStore(t)
	if err := s.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(DefaultProfiles()) {
		t.Fatalf("seeded %d profiles, want %d", len(got), len(DefaultProfiles()))
	}
	// Deleting a default and re-running EnsureDefaults must NOT resurrect it:
	// the directory exists, so the user's curation wins.
	if err := s.Delete("spike"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	after, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(got)-1 {
		t.Errorf("deleted default came back: %d profiles", len(after))
	}
	for _, p := range after {
		if p.Name == "spike" {
			t.Error("spike should stay deleted")
		}
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	s := tempStore(t)
	orig := Constant(25, 45*time.Second)
	if err := s.Save("mine/steady", orig); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("mine/steady")
	if err != nil {
		t.Fatal(err)
	}
	// The file name is canonical: it overrides the constructor's name.
	if got.Name != "mine/steady" {
		t.Errorf("Name = %q, want mine/steady", got.Name)
	}
	if got.Duration() != 45*time.Second || got.Peak() != 25 {
		t.Errorf("shape mismatch: %+v", got)
	}
	// The stored file is human-editable JSON with duration strings.
	b, err := os.ReadFile(filepath.Join(s.Root, "mine", "steady.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"at": "45s"`) {
		t.Errorf("file should be hand-editable with duration strings:\n%s", b)
	}
}

func TestStoreRejectsInvalid(t *testing.T) {
	s := tempStore(t)
	if err := s.Save("bad", Profile{Points: []Point{{At: 0, RPS: 1}}}); err == nil {
		t.Error("saving an invalid profile must fail")
	}
	for _, name := range []string{"", "../escape", "/abs"} {
		if err := s.Save(name, Constant(1, time.Second)); err == nil {
			t.Errorf("name %q must be rejected", name)
		}
	}
	// A corrupt file is skipped by List but reported by Load.
	if err := s.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.Name == "broken" {
			t.Error("List must skip the corrupt file")
		}
	}
	if _, err := s.Load("broken"); err == nil {
		t.Error("Load must surface the corrupt file's error")
	}
}

func TestStoreRename(t *testing.T) {
	s := tempStore(t)
	if err := s.Save("a", Constant(1, time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("b", Constant(2, time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("a", "b"); err == nil {
		t.Error("rename must not overwrite an existing profile")
	}
	if err := s.Rename("a", "sub/c"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("sub/c")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "sub/c" || got.Peak() != 1 {
		t.Errorf("renamed profile = %+v", got)
	}
	if _, err := s.Load("a"); err == nil {
		t.Error("old name should be gone after rename")
	}
	// Deleting the moved profile prunes its now-empty folder.
	if err := s.Delete("sub/c"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "sub")); err == nil {
		t.Error("empty subfolder should be pruned")
	}
}

func TestListMissingRootIsEmpty(t *testing.T) {
	s := tempStore(t) // Root never created
	got, err := s.List()
	if err != nil || got != nil {
		t.Errorf("List on a missing root = %v, %v; want nil, nil", got, err)
	}
}
