package collections

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

func TestStoreSaveListLoadDelete(t *testing.T) {
	s := Store{Root: t.TempDir()}
	req := model.Request{
		Method:  "POST",
		URL:     "https://example.test/users",
		Headers: []model.Header{{Name: "Accept", Value: "application/json", Enabled: true}},
		Query:   []model.KV{{Key: "page", Value: "1", Enabled: true}},
		Body:    `{"ok":true}`,
	}

	if err := s.Save("users/create", req); err != nil {
		t.Fatalf("Save: %v", err)
	}
	items, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !hasItem(items, "users/create", false) || !hasItem(items, "users", true) {
		t.Fatalf("items = %+v, want request users/create under group users", items)
	}
	got, err := s.Load("users/create")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Method != req.Method || got.URL != req.URL || got.Body != req.Body {
		t.Fatalf("loaded request = %+v, want %+v", got, req)
	}
	if err := s.Copy("users/create", "users/create-copy"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if _, err := s.Load("users/create-copy"); err != nil {
		t.Fatalf("Load copy: %v", err)
	}
	if err := s.Rename("users/create-copy", "users/create-renamed"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := s.Load("users/create-renamed"); err != nil {
		t.Fatalf("Load renamed: %v", err)
	}
	if err := s.Delete("users/create"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("users/create-renamed"); err != nil {
		t.Fatalf("Delete renamed: %v", err)
	}
	items, err = s.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items after delete = %+v, want empty", items)
	}
}

func TestStoreRoundTripAllFields(t *testing.T) {
	s := Store{Root: t.TempDir()}
	req := model.Request{
		Method:  "PUT",
		URL:     "https://example.test/a",
		Headers: []model.Header{{Name: "Authorization", Value: "Bearer x", Enabled: true}, {Name: "X-Off", Value: "no", Enabled: false}},
		Query:   []model.KV{{Key: "q", Value: "1", Enabled: true}},
		Body:    `{"k":"v"}`,
		Auth:    model.Auth{Type: model.AuthAPIKey, Key: "X-Key", Value: "{{secret}}", InQuery: true},
		Timeout: 12 * time.Second,
	}
	if err := s.Save("round/trip", req); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load("round/trip")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, req) {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", got, req)
	}
}

func TestStoreCollisionGuards(t *testing.T) {
	s := Store{Root: t.TempDir()}
	a := model.Request{Method: "GET", URL: "https://a.test"}
	b := model.Request{Method: "POST", URL: "https://b.test"}
	if err := s.Save("a", a); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("b", b); err != nil {
		t.Fatal(err)
	}

	if err := s.Copy("a", "b"); err == nil {
		t.Error("Copy onto existing name should fail")
	}
	if err := s.Rename("a", "b"); err == nil {
		t.Error("Rename onto existing name should fail")
	}
	// b must be untouched by the refused operations.
	if got, _ := s.Load("b"); got.URL != "https://b.test" {
		t.Errorf("b was clobbered: %+v", got)
	}
}

func TestStorePrunesEmptyDirs(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	if err := s.Save("deep/nested/req", model.NewRequest()); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("deep/nested/req"); err != nil {
		t.Fatal(err)
	}
	// The now-empty deep/ and deep/nested/ dirs should be gone.
	if _, err := os.Stat(filepath.Join(root, "deep")); !os.IsNotExist(err) {
		t.Errorf("empty parent dirs should be pruned, stat err = %v", err)
	}
	// Root itself must survive.
	if _, err := os.Stat(root); err != nil {
		t.Errorf("root should not be removed: %v", err)
	}
}

func hasItem(items []Item, name string, isDir bool) bool {
	for _, it := range items {
		if it.Name == name && it.IsDir == isDir {
			return true
		}
	}
	return false
}

func TestGroupsLifecycle(t *testing.T) {
	s := Store{Root: t.TempDir()}

	// An empty group persists and appears in List.
	if err := s.CreateGroup("APISet1"); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if items, _ := s.List(); !hasItem(items, "APISet1", true) {
		t.Fatalf("empty group not listed: %+v", items)
	}
	// Creating the same group again is a collision.
	if err := s.CreateGroup("APISet1"); err == nil {
		t.Error("duplicate CreateGroup should fail")
	}

	// A request saved into the group; deleting it leaves the group (it has a marker).
	if err := s.Save("APISet1/login", model.NewRequest()); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("APISet1/login"); err != nil {
		t.Fatal(err)
	}
	if items, _ := s.List(); !hasItem(items, "APISet1", true) {
		t.Error("explicit group should survive deleting its last request")
	}

	// Rename the group, then delete it entirely.
	if err := s.RenameGroup("APISet1", "APISet2"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if items, _ := s.List(); hasItem(items, "APISet1", true) || !hasItem(items, "APISet2", true) {
		t.Errorf("rename group failed: %+v", items)
	}
	if err := s.DeleteGroup("APISet2"); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if items, _ := s.List(); len(items) != 0 {
		t.Errorf("after DeleteGroup, items = %+v, want empty", items)
	}
}

func TestGroupRejectsUnsafeNames(t *testing.T) {
	s := Store{Root: t.TempDir()}
	for _, name := range []string{"", "..", "../evil", "/abs"} {
		if err := s.CreateGroup(name); err == nil {
			t.Errorf("CreateGroup(%q) should fail", name)
		}
	}
}

func TestStoreRejectsUnsafeNames(t *testing.T) {
	s := Store{Root: t.TempDir()}
	unsafe := []string{
		"", "   ", "/", "..", "../secret", "/tmp/secret",
		"a/../../etc/passwd", "foo/..", "./", ".",
	}
	for _, name := range unsafe {
		if err := s.Save(name, model.NewRequest()); err == nil {
			t.Errorf("Save(%q) succeeded, want error", name)
		}
	}
}
