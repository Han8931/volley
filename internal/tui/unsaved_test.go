package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/model"
)

// isQuit reports whether cmd is Bubble Tea's Quit command.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func seededModel(t *testing.T) (Model, collections.Store) {
	t.Helper()
	store := collections.Store{Root: t.TempDir()}
	seed := model.NewRequest()
	seed.URL = "https://seed.test"
	other := model.NewRequest()
	other.URL = "https://other.test"
	if err := store.Save("seed", seed); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("other", other); err != nil {
		t.Fatal(err)
	}
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.collectionStore = store
	m.refreshCollections()
	return m, store
}

func TestCleanModelQuitsWithoutPrompt(t *testing.T) {
	base, _ := seededModel(t)
	if base.dirty() {
		t.Fatal("a freshly launched model must not read as dirty")
	}
	if _, cmd := base.guardedQuit(); !isQuit(cmd) {
		t.Error("quitting a clean model should quit immediately, not prompt")
	}
}

func TestEditingMarksDirty(t *testing.T) {
	base, _ := seededModel(t)
	m := base.loadSavedRequest("seed")
	if m.dirty() {
		t.Fatal("a just-loaded request must not be dirty")
	}
	m.url.SetText("https://seed.test/edited")
	if !m.dirty() {
		t.Fatal("editing the URL must mark the request dirty")
	}
}

func TestSwitchingRequestGuardsUnsavedEdits(t *testing.T) {
	base, store := seededModel(t)
	m := base.loadSavedRequest("seed")
	m.url.SetText("https://seed.test/edited")

	// Opening another request must NOT silently discard the edit; it arms the prompt.
	next, _ := m.guardedOpen("other")
	armed := next.(Model)
	if armed.pendingAction != pendingOpenRequest {
		t.Fatal("opening a request with unsaved edits should arm the save prompt")
	}
	if armed.url.Text() != "https://seed.test/edited" {
		t.Error("the other request must not load until the prompt is resolved")
	}

	// esc: stay put, edits intact.
	cancelled := step(armed, keyEsc)
	if cancelled.pendingAction != pendingNone {
		t.Error("esc should clear the pending prompt")
	}
	if cancelled.url.Text() != "https://seed.test/edited" {
		t.Error("esc must preserve the in-progress edit")
	}

	// n: discard and load the other request.
	discarded := step(armed, runes("n"))
	if discarded.url.Text() != "https://other.test" {
		t.Errorf("n should load the other request; url = %q", discarded.url.Text())
	}

	// y: save the edit to disk, then load the other request.
	saved := step(armed, runes("y"))
	if saved.url.Text() != "https://other.test" {
		t.Errorf("y should save then load the other request; url = %q", saved.url.Text())
	}
	reloaded, err := store.Load("seed")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.URL != "https://seed.test/edited" {
		t.Errorf("y should have persisted the seed edit; got %q", reloaded.URL)
	}
}

func TestQuitGuardsAndForceQuit(t *testing.T) {
	base, _ := seededModel(t)
	m := base.loadSavedRequest("seed")
	m.url.SetText("https://seed.test/edited")

	// A normal quit while dirty arms the prompt instead of quitting.
	q, cmd := m.guardedQuit()
	if isQuit(cmd) {
		t.Error("quitting with unsaved edits should prompt, not quit")
	}
	if q.(Model).pendingAction != pendingQuit {
		t.Error("dirty quit should arm the quit prompt")
	}

	// :q! force-quits, discarding edits.
	if _, c := m.executeCommand("q!"); !isQuit(c) {
		t.Error(":q! should force-quit even with unsaved edits")
	}
}

// The Vim "all" variants are single-buffer aliases: :qa guards, :qa! forces,
// :wqa saves-then-quits.
func TestQuitAllAliases(t *testing.T) {
	base, _ := seededModel(t)
	m := base.loadSavedRequest("seed")
	m.url.SetText("https://seed.test/edited")

	// :qa while dirty arms the prompt instead of quitting.
	q, cmd := m.executeCommand("qa")
	if isQuit(cmd) {
		t.Error(":qa with unsaved edits should prompt, not quit")
	}
	if q.(Model).pendingAction != pendingQuit {
		t.Error(":qa should arm the quit prompt when dirty")
	}

	// :qa! force-quits, discarding edits.
	if _, c := m.executeCommand("qa!"); !isQuit(c) {
		t.Error(":qa! should force-quit even with unsaved edits")
	}

	// :wqa saves the current request, then quits.
	if _, c := m.executeCommand("wqa"); !isQuit(c) {
		t.Fatal(":wqa should save and quit")
	}
	reloaded, err := m.collectionStore.Load("seed")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.URL != "https://seed.test/edited" {
		t.Errorf(":wqa should have persisted the edit; got %q", reloaded.URL)
	}
}
