package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/model"
)

func TestAuthTypeCycling(t *testing.T) {
	e := NewAuthEditor()
	e.SetFocused(true)
	if e.Auth().Type != model.AuthNone {
		t.Fatalf("initial type = %q, want none", e.Auth().Type)
	}
	// space on the Type row cycles forward through the types and wraps.
	want := []string{model.AuthBearer, model.AuthBasic, model.AuthAPIKey, model.AuthNone}
	for _, w := range want {
		e.UpdateNormal(rk(" "))
		if e.Auth().Type != w {
			t.Fatalf("after cycle, type = %q, want %q", e.Auth().Type, w)
		}
	}
	// h cycles backward.
	e.UpdateNormal(rk("h"))
	if e.Auth().Type != model.AuthAPIKey {
		t.Fatalf("h from none should wrap to apikey, got %q", e.Auth().Type)
	}
}

func TestAuthVisibleFieldsPerType(t *testing.T) {
	e := NewAuthEditor()
	cases := map[string]int{
		model.AuthNone:   1, // just the Type row
		model.AuthBearer: 2, // Type + Token
		model.AuthBasic:  3, // Type + Username + Password
		model.AuthAPIKey: 4, // Type + Key + Value + Add-to
	}
	for typ, n := range cases {
		e.SetAuth(model.Auth{Type: typ})
		if got := len(e.visibleFields()); got != n {
			t.Errorf("type %q: %d fields, want %d", typ, got, n)
		}
	}
}

func TestAuthEditTokenField(t *testing.T) {
	e := NewAuthEditor()
	e.SetFocused(true)
	e.SetAuth(model.Auth{Type: model.AuthBearer})

	e.UpdateNormal(rk("j")) // move to Token row
	e.UpdateNormal(rk("i")) // begin edit
	if !e.Editing() {
		t.Fatal("expected editing after 'i' on a text field")
	}
	for _, r := range "t0k" {
		e.UpdateEditing(rk(string(r)))
	}
	e.UpdateEditing(tea.KeyMsg{Type: tea.KeyEnter}) // commit
	if e.Editing() {
		t.Error("enter should commit and leave edit mode")
	}
	if e.Auth().Token != "t0k" {
		t.Errorf("token = %q, want t0k", e.Auth().Token)
	}
}

func TestAuthAPIKeyLocationToggle(t *testing.T) {
	e := NewAuthEditor()
	e.SetFocused(true)
	e.SetAuth(model.Auth{Type: model.AuthAPIKey})

	e.UpdateNormal(rk("G")) // jump to the last row ("Add to")
	if e.Auth().InQuery {
		t.Fatal("InQuery should start false")
	}
	e.UpdateNormal(rk(" "))
	if !e.Auth().InQuery {
		t.Error("space should toggle location to query")
	}
	e.UpdateNormal(rk("h")) // explicitly back to header
	if e.Auth().InQuery {
		t.Error("h should set location to header")
	}
}

// Changing to a type with fewer rows must not leave the cursor out of bounds.
func TestAuthCursorClampOnTypeChange(t *testing.T) {
	e := NewAuthEditor()
	e.SetFocused(true)
	e.SetAuth(model.Auth{Type: model.AuthAPIKey})
	e.UpdateNormal(rk("G")) // cursor at row 3
	// Cycle Type forward to None (1 row); cursor must clamp to 0.
	e.cursor = 0 // move back to the Type row to change it
	e.SetAuth(model.Auth{Type: model.AuthAPIKey})
	e.UpdateNormal(rk("G")) // cursor -> 3 again
	// Directly drive a type change while cursor is high by putting cursor on Type.
	e.cursor = 0
	e.UpdateNormal(rk("h")) // apikey -> basic (3 rows), still fine
	e.UpdateNormal(rk("h")) // basic -> bearer (2 rows)
	e.UpdateNormal(rk("h")) // bearer -> none (1 row)
	if e.cursor >= len(e.visibleFields()) {
		t.Errorf("cursor %d out of bounds for %d fields", e.cursor, len(e.visibleFields()))
	}
}
