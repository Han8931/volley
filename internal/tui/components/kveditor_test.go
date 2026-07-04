package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/model"
)

// rk builds a normal-mode rune keypress (its String() is the literal text).
func rk(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func seed(n int) KVEditor {
	e := NewKVEditor("Headers")
	rows := make([]model.KV, n)
	for i := range rows {
		rows[i] = model.KV{Key: "k", Value: "v", Enabled: true}
	}
	e.SetRows(rows)
	return e
}

func TestColumnMotions(t *testing.T) {
	e := seed(2)
	for _, k := range []string{"l", "$", "w", "e"} {
		e.col = 0
		if !e.UpdateNormal(rk(k)) || e.col != 1 {
			t.Errorf("%q should move to value column, col=%d", k, e.col)
		}
	}
	for _, k := range []string{"h", "0", "^", "b"} {
		e.col = 1
		if !e.UpdateNormal(rk(k)) || e.col != 0 {
			t.Errorf("%q should move to key column, col=%d", k, e.col)
		}
	}
}

func TestRowMotionsAndBounds(t *testing.T) {
	e := seed(3)
	// k at the top is a no-op (can't go above row 0).
	e.UpdateNormal(rk("k"))
	if e.cursor != 0 {
		t.Fatalf("k at top: cursor=%d, want 0", e.cursor)
	}
	// j walks down but stops at the last row.
	for i := 0; i < 5; i++ {
		e.UpdateNormal(rk("j"))
	}
	if e.cursor != 2 {
		t.Fatalf("j past end: cursor=%d, want 2", e.cursor)
	}
	// G jumps to bottom, gg to top.
	e.cursor = 0
	e.UpdateNormal(rk("G"))
	if e.cursor != 2 {
		t.Fatalf("G: cursor=%d, want 2", e.cursor)
	}
	e.UpdateNormal(rk("g"))
	e.UpdateNormal(rk("g"))
	if e.cursor != 0 {
		t.Fatalf("gg: cursor=%d, want 0", e.cursor)
	}
}

func TestPendingGCancels(t *testing.T) {
	e := seed(3)
	e.cursor = 2
	e.UpdateNormal(rk("g")) // arm pending g
	e.UpdateNormal(rk("j")) // a non-g key cancels gg and acts normally
	if e.cursor != 2 {
		t.Fatalf("g then j: cursor=%d, want unchanged 2 (j is already at bottom)", e.cursor)
	}
	// The pending state must be cleared: a lone g now, then a real gg.
	e.UpdateNormal(rk("g"))
	e.UpdateNormal(rk("g"))
	if e.cursor != 0 {
		t.Fatalf("fresh gg after cancel: cursor=%d, want 0", e.cursor)
	}
}

func TestDeleteDD(t *testing.T) {
	e := seed(3)
	e.cursor = 1
	e.UpdateNormal(rk("d"))
	e.UpdateNormal(rk("d"))
	if len(e.rows) != 2 {
		t.Fatalf("dd: len=%d, want 2", len(e.rows))
	}
	if e.cursor != 1 {
		t.Fatalf("dd mid-list: cursor=%d, want 1", e.cursor)
	}
	// Deleting the last row pulls the cursor back so it stays in range.
	e.cursor = 1
	e.UpdateNormal(rk("d"))
	e.UpdateNormal(rk("d"))
	if len(e.rows) != 1 || e.cursor != 0 {
		t.Fatalf("dd at end: len=%d cursor=%d, want len 1 cursor 0", len(e.rows), e.cursor)
	}
}

func TestDeleteDJ(t *testing.T) {
	// "d" then "j" also deletes the current row (the pendD path in case "j").
	e := seed(3)
	e.cursor = 0
	e.UpdateNormal(rk("d"))
	e.UpdateNormal(rk("j"))
	if len(e.rows) != 2 {
		t.Fatalf("dj: len=%d, want 2", len(e.rows))
	}
}

func TestCancelPendingClearsDelete(t *testing.T) {
	e := seed(2)
	e.cursor = 0
	e.UpdateNormal(rk("d")) // arm pending d
	e.CancelPending()       // owner steals the sequence
	e.UpdateNormal(rk("j")) // must now be a plain move, not a delete
	if len(e.rows) != 2 {
		t.Fatalf("CancelPending should cancel dd: len=%d, want 2", len(e.rows))
	}
	if e.cursor != 1 {
		t.Fatalf("j after CancelPending should move: cursor=%d, want 1", e.cursor)
	}
}

func TestOpenBelowAndAbove(t *testing.T) {
	e := seed(2)
	e.cursor = 0
	e.UpdateNormal(rk("o")) // append below, enter edit
	if len(e.rows) != 3 || e.cursor != 2 || e.col != 0 || !e.Editing() {
		t.Fatalf("o: len=%d cursor=%d col=%d editing=%v", len(e.rows), e.cursor, e.col, e.Editing())
	}
	if !e.rows[2].Enabled {
		t.Error("o should create an enabled row")
	}

	e = seed(3)
	e.commit() // leave the o-editing state clean for this case
	e.cursor = 1
	e.UpdateNormal(rk("O")) // insert above cursor
	if len(e.rows) != 4 || e.cursor != 1 || !e.Editing() {
		t.Fatalf("O: len=%d cursor=%d editing=%v", len(e.rows), e.cursor, e.Editing())
	}
	if e.rows[1].Key != "" || !e.rows[1].Enabled {
		t.Errorf("O should insert a blank enabled row at cursor, got %+v", e.rows[1])
	}
	if e.rows[2].Key != "k" {
		t.Error("O should have pushed the old row down")
	}
}

func TestInsertEntryPoints(t *testing.T) {
	// I positions on the key column, A on the value column; both begin editing.
	e := seed(1)
	e.col = 1
	e.UpdateNormal(rk("I"))
	if e.col != 0 || !e.Editing() {
		t.Errorf("I: col=%d editing=%v, want col 0 editing", e.col, e.Editing())
	}
	e.commit()
	e.col = 0
	e.UpdateNormal(rk("A"))
	if e.col != 1 || !e.Editing() {
		t.Errorf("A: col=%d editing=%v, want col 1 editing", e.col, e.Editing())
	}
}

func TestInsertOnEmptyCreatesRow(t *testing.T) {
	e := NewKVEditor("Query")
	if len(e.rows) != 0 {
		t.Fatal("new editor should start empty")
	}
	e.UpdateNormal(rk("i")) // must materialize a row so editing is possible
	if len(e.rows) != 1 || !e.Editing() {
		t.Fatalf("i on empty: len=%d editing=%v, want 1 row editing", len(e.rows), e.Editing())
	}
}

func TestToggleEnabled(t *testing.T) {
	e := seed(1) // starts enabled
	e.UpdateNormal(tea.KeyMsg{Type: tea.KeySpace})
	if e.rows[0].Enabled {
		t.Error("space should disable an enabled row")
	}
	e.UpdateNormal(tea.KeyMsg{Type: tea.KeySpace})
	if !e.rows[0].Enabled {
		t.Error("space should re-enable a disabled row")
	}
}

func TestEditCommitAndTabHop(t *testing.T) {
	e := seed(1)
	e.rows[0] = model.KV{Enabled: true} // clear key/value
	e.col = 0
	e.UpdateNormal(rk("i"))
	// Type into the key cell, then Tab to hop to the value cell and keep typing.
	e.UpdateEditing(rk("Host"))
	e.UpdateEditing(tea.KeyMsg{Type: tea.KeyTab})
	if e.rows[0].Key != "Host" {
		t.Fatalf("key after tab commit = %q, want Host", e.rows[0].Key)
	}
	if e.col != 1 || !e.Editing() {
		t.Fatalf("tab should hop to value column and keep editing: col=%d editing=%v", e.col, e.Editing())
	}
	e.UpdateEditing(rk("example.test"))
	e.UpdateEditing(tea.KeyMsg{Type: tea.KeyEnter})
	if e.rows[0].Value != "example.test" {
		t.Fatalf("value after enter commit = %q", e.rows[0].Value)
	}
	if e.Editing() {
		t.Error("enter should end editing")
	}
}

func TestEscCommits(t *testing.T) {
	e := seed(1)
	e.rows[0] = model.KV{Enabled: true}
	e.col = 0
	e.UpdateNormal(rk("i"))
	e.UpdateEditing(rk("abc"))
	e.UpdateEditing(tea.KeyMsg{Type: tea.KeyEsc})
	if e.rows[0].Key != "abc" {
		t.Errorf("esc should commit the typed value, key=%q", e.rows[0].Key)
	}
	if e.Editing() {
		t.Error("esc should end editing")
	}
}

func TestSetRowsResetsState(t *testing.T) {
	e := seed(3)
	e.cursor, e.col = 2, 1
	e.UpdateNormal(rk("i")) // enter editing
	e.SetRows([]model.KV{{Key: "a"}})
	if e.cursor != 0 || e.col != 0 || e.Editing() {
		t.Errorf("SetRows should reset cursor/col/editing: cursor=%d col=%d editing=%v", e.cursor, e.col, e.Editing())
	}
}

func TestHeadersAdaptation(t *testing.T) {
	e := NewKVEditor("Headers")
	e.SetRows([]model.KV{{Key: "Accept", Value: "application/json", Enabled: true}, {Key: "X-Off", Value: "no", Enabled: false}})
	hs := e.Headers()
	if len(hs) != 2 {
		t.Fatalf("Headers len=%d, want 2", len(hs))
	}
	if hs[0] != (model.Header{Name: "Accept", Value: "application/json", Enabled: true}) {
		t.Errorf("Headers[0] = %+v", hs[0])
	}
	if hs[1].Enabled {
		t.Error("disabled row should stay disabled through Headers()")
	}
}

func TestUnknownKeyNotConsumed(t *testing.T) {
	e := seed(1)
	if e.UpdateNormal(rk("z")) {
		t.Error("an unhandled key should not be consumed (so the owner can move focus)")
	}
}

func TestMotionsOnEmptyDoNotPanic(t *testing.T) {
	e := NewKVEditor("Headers")
	for _, k := range []string{"j", "k", "g", "g", "G", " ", "d", "d"} {
		e.UpdateNormal(rk(k))
	}
	if e.cursor != 0 || len(e.rows) != 0 {
		t.Errorf("empty editor motions changed state: cursor=%d len=%d", e.cursor, len(e.rows))
	}
}
