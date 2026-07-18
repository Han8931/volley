package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/model"
)

func TestCopyButtonShownOnlyForCompletedResponse(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	if strings.Contains(stripANSI(m.viewResponseInner()), "copy") {
		t.Error("no copy button before a response exists")
	}
	m.hasResp = true
	m.resp = model.Response{Status: "200 OK", StatusCode: 200, Body: []byte(`{"ok":true}`)}
	m.respText = `{"ok":true}`
	m.vp.SetContent(m.respText)
	if !strings.Contains(stripANSI(m.viewResponseInner()), "copy") {
		t.Error("a completed response should show the copy button")
	}
	m.sending = true // a resend in flight replaces the button with the spinner
	if strings.Contains(stripANSI(m.viewResponseInner()), "copy") {
		t.Error("copy button must be hidden while a request is in flight")
	}
}

func TestCopyButtonClickCopiesResponse(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.hasResp = true
	m.resp = model.Response{Status: "200 OK", StatusCode: 200, Body: []byte("hello-body")}
	m.respText = "hello-body"
	m.vp.SetContent(m.respText)

	// Locating the button in the rendered view and clicking it also verifies the
	// copyButtonClicked geometry matches where respHeaderBar actually draws it.
	x, y := findInView(m, "⧉ copy")
	if x < 0 {
		t.Fatal("copy button not found in the rendered response header")
	}
	next, _ := m.handleClick(clickAt(x, y))
	got := next.(Model)
	// The clipboard may be unavailable in headless CI, so accept either the
	// success or the unavailable message — both prove the click routed to a yank.
	if !strings.Contains(got.statusMsg, "yanked") && !strings.Contains(got.statusMsg, "clipboard") {
		t.Fatalf("clicking copy should attempt a yank, status=%q", got.statusMsg)
	}
}

func TestRenderStatusSummaryTiers(t *testing.T) {
	resp := model.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Duration:   123 * time.Millisecond,
		Size:       1234,
	}
	// A generous budget shows status, timing, and size.
	full := stripANSI(renderStatusSummary(resp, 40))
	for _, want := range []string{"200 OK", "123ms", "1.2 kB"} {
		if !strings.Contains(full, want) {
			t.Errorf("full summary %q missing %q", full, want)
		}
	}
	// Tight enough to drop the size but keep the timing.
	noSize := stripANSI(renderStatusSummary(resp, len("200 OK · 123ms")))
	if strings.Contains(noSize, "kB") {
		t.Errorf("summary %q should have dropped the size", noSize)
	}
	if !strings.Contains(noSize, "123ms") {
		t.Errorf("summary %q should still show the timing", noSize)
	}
	// Tighter still: only the status code survives.
	codeOnly := stripANSI(renderStatusSummary(resp, len("200 OK")))
	if codeOnly != "200 OK" {
		t.Errorf("tight summary = %q, want just the status code", codeOnly)
	}
}

func TestResponseJSONSyntaxHighlight(t *testing.T) {
	plain := "{\n  \"ok\": true,\n  \"n\": 2\n}"
	highlighted := highlightResponseText(plain)
	if got := stripANSI(highlighted); got != plain {
		t.Fatalf("highlighting changed visible text:\n%s", got)
	}
	if got := highlightResponseText("hello"); got != "hello" {
		t.Fatalf("non-JSON response should not be highlighted, got %q", got)
	}
}

func TestSendingKeepsPreviousResponseVisible(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.hasResp = true
	m.resp = model.Response{Status: "200 OK", StatusCode: 200, Duration: 50 * time.Millisecond, Size: 99}
	m.respText = "previous body line"
	m.vp.SetContent(m.respText)
	m.sending = true

	inner := stripANSI(m.viewResponseInner())
	// The prior body is still on screen while the resend is in flight.
	if !strings.Contains(inner, "previous body line") {
		t.Errorf("previous response body should stay visible while sending:\n%s", inner)
	}
	// The header shows the sending indicator in place of the status readout.
	if !strings.Contains(inner, "sending…") {
		t.Errorf("header should show the sending indicator:\n%s", inner)
	}
	if strings.Contains(inner, "200 OK") {
		t.Errorf("the stale status should be replaced by the spinner while sending:\n%s", inner)
	}
}

func TestSendingWithoutPriorResponseCentersSpinner(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 120, Height: 40})
	m.sending = true // no prior response

	inner := stripANSI(m.viewResponseInner())
	if !strings.Contains(inner, "sending…") {
		t.Errorf("first send should still show the spinner:\n%s", inner)
	}
	if strings.Contains(inner, "Body") && strings.Contains(inner, "Headers") {
		t.Errorf("with no prior response there is no tab/body to show:\n%s", inner)
	}
}

func TestResponseHeaderCombinesTabsAndStatus(t *testing.T) {
	m := step(New(), tea.WindowSizeMsg{Width: 160, Height: 40})
	m.hasResp = true
	m.resp = model.Response{Status: "200 OK", StatusCode: 200, Duration: 123 * time.Millisecond, Size: 1234}
	m.respText = "body line"
	m.vp.SetContent(m.respText)

	lines := strings.Split(stripANSI(m.viewResponseInner()), "\n")

	// The tabs and the status share exactly one row; neither appears alone.
	var headerRows int
	for _, ln := range lines {
		tabs := strings.Contains(ln, "Body") && strings.Contains(ln, "Headers")
		status := strings.Contains(ln, "200 OK")
		switch {
		case tabs && status:
			headerRows++
		case tabs || status:
			t.Errorf("tabs and status must stay on one row, but this row splits them: %q", ln)
		}
	}
	if headerRows != 1 {
		t.Fatalf("want exactly one combined tab/status header row, got %d", headerRows)
	}
	// Status sits to the right of the tabs (flush-right corner).
	header := lines[0]
	if strings.Index(header, "200 OK") <= strings.Index(header, "Headers") {
		t.Errorf("status should be right of the tabs, got %q", header)
	}
}

func TestWrapLines(t *testing.T) {
	if got := wrapLines("abcdefgh", 3); got != "abc\ndef\ngh" {
		t.Errorf("wrapLines = %q", got)
	}
	if got := wrapLines("ab\ncd", 10); got != "ab\ncd" {
		t.Errorf("short lines must pass through, got %q", got)
	}
	if got := wrapLines("abcdef", 0); got != "abcdef" {
		t.Errorf("width 0 must be a no-op, got %q", got)
	}
}

// A budget too small for even the bare status yields nothing: a clipped
// "200 OK" would read as HTTP 20.
func TestStatusSummaryHidesWhenTooNarrow(t *testing.T) {
	resp := model.Response{Status: "200 OK", StatusCode: 200, Size: 10}
	if got := stripANSI(renderStatusSummary(resp, 3)); got != "" {
		t.Errorf("tiny budget should render nothing, got %q", got)
	}
	if got := stripANSI(renderStatusSummary(resp, 0)); got != "" {
		t.Errorf("zero budget should render nothing, got %q", got)
	}
}
