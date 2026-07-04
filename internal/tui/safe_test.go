package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSafeModel_NormalUpdatePassesThrough(t *testing.T) {
	s := Program()
	next, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	sm, ok := next.(safeModel)
	if !ok {
		t.Fatalf("Update returned %T, want safeModel", next)
	}
	inner, ok := sm.m.(Model)
	if !ok {
		t.Fatalf("inner model is %T, want Model", sm.m)
	}
	if inner.width != 100 || inner.height != 40 {
		t.Errorf("window size not applied: %dx%d", inner.width, inner.height)
	}
}

// boomModel panics from every lifecycle method, standing in for a model with a
// latent crash so we can exercise the recovery paths.
type boomModel struct{}

func (boomModel) Init() tea.Cmd                       { return nil }
func (boomModel) Update(tea.Msg) (tea.Model, tea.Cmd) { panic("boom in update") }
func (boomModel) View() string                        { panic("boom in view") }

func TestSafeModel_RecoversFromUpdatePanic(t *testing.T) {
	s := safeModel{m: boomModel{}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped safeModel.Update: %v", r)
		}
	}()

	next, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Errorf("expected nil cmd after recovered panic, got %v", cmd)
	}
	if _, ok := next.(safeModel); !ok {
		t.Fatalf("Update returned %T, want safeModel (wrapper must survive)", next)
	}
}

func TestSafeModel_RecoversFromViewPanic(t *testing.T) {
	s := safeModel{m: boomModel{}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped safeModel.View: %v", r)
		}
	}()

	if got := s.View(); got == "" {
		t.Error("View returned empty string after recovered panic, want a message")
	}
}
