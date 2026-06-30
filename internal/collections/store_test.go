package collections

import (
	"testing"

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
	if len(items) != 1 || items[0].Name != "users/create" {
		t.Fatalf("items = %+v, want users/create", items)
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

func TestStoreRejectsUnsafeNames(t *testing.T) {
	s := Store{Root: t.TempDir()}
	for _, name := range []string{"", "../secret", "/tmp/secret"} {
		if err := s.Save(name, model.NewRequest()); err == nil {
			t.Fatalf("Save(%q) succeeded, want error", name)
		}
	}
}
