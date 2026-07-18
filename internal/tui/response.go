package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"

	"github.com/tabularasa/volley/internal/model"
)

// statusColor maps an HTTP status class to a palette color.
func statusColor(code int) lipgloss.TerminalColor {
	switch {
	case code >= 200 && code < 300:
		return colOK
	case code >= 300 && code < 400:
		return lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#38BDF8"} // cyan
	case code >= 400 && code < 500:
		return colMethod // amber
	case code >= 500:
		return colErr
	default:
		return colDim
	}
}

// colErr is the error/5xx red, adaptive like the rest of the palette.
var colErr = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}

// renderStatusSummary is the response status + timing shown in the response
// pane's header row. It fits within budget columns, shedding the size and then
// the timing when the pane is too narrow to hold the full summary.
func renderStatusSummary(resp model.Response, budget int) string {
	if budget < 1 {
		return "" // MaxWidth(0) is "no limit", so guard the degenerate budget here
	}
	clip := func(s string) string {
		return lipgloss.NewStyle().MaxWidth(budget).Render(s)
	}
	if resp.Err != nil {
		// The full error also appears in the body, so clipping here is safe.
		return clip(lipgloss.NewStyle().Foreground(colErr).Bold(true).
			Render("✗ " + resp.Err.Error()))
	}
	code := lipgloss.NewStyle().Foreground(statusColor(resp.StatusCode)).Bold(true).
		Render(resp.Status)
	dimStyle := lipgloss.NewStyle().Foreground(colDim)
	sizeStr := humanize.Bytes(uint64(resp.Size))
	if resp.Truncated {
		sizeStr += "+ (truncated)"
	}
	dur := resp.Duration.Round(time.Millisecond).String()

	// Widest first; pick the largest tier that fits so segments drop whole
	// rather than clipping mid-word. When even the bare status doesn't fit,
	// show nothing: a clipped "200 OK" reads as HTTP 20, which is worse than
	// an empty cell.
	for _, tier := range []string{
		code + dimStyle.Render(fmt.Sprintf(" · %s · %s", dur, sizeStr)),
		code + dimStyle.Render(" · "+dur),
		code,
	} {
		if lipgloss.Width(tier) <= budget {
			return tier
		}
	}
	return ""
}

// renderResponseBody returns the scrollable body content, soft-wrapped to the
// viewport width so long lines stay readable instead of being clipped at the
// pane edge. Unless raw is set, a JSON-looking payload is pretty-printed.
func renderResponseBody(resp model.Response, width int, raw bool) string {
	if resp.Err != nil {
		return lipgloss.NewStyle().Foreground(colErr).
			Render(wrapLines("Request failed:\n\n"+resp.Err.Error(), width))
	}
	body := resp.Body
	if !raw {
		if pretty, ok := prettyJSON(body); ok {
			body = pretty
		}
	}
	out := string(body)
	if out == "" {
		return dim("(empty response body)")
	}
	return wrapLines(out, width)
}

// wrapLines hard-wraps every line of plain (un-styled) text to at most width
// runes, preserving existing newlines. width <= 0 leaves the text untouched.
func wrapLines(s string, width int) string {
	if width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		r := []rune(line)
		for len(r) > width {
			out = append(out, string(r[:width]))
			r = r[width:]
		}
		out = append(out, string(r))
	}
	return strings.Join(out, "\n")
}

func highlightResponseText(text string) string {
	if !isLikelyJSONBody(text) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = highlightJSONLine(line)
	}
	return strings.Join(lines, "\n")
}

// prettyJSON indents b when it is valid JSON.
func prettyJSON(b []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil, false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trimmed, "", "  "); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// renderResponseHeaders renders response headers as a sorted, aligned block
// for the response pane's Headers tab.
func renderResponseHeaders(resp model.Response) string {
	if resp.Err != nil {
		return dim("(no headers — request failed)")
	}
	if len(resp.Headers) == 0 {
		return dim("(no response headers)")
	}
	hs := append([]model.Header(nil), resp.Headers...)
	sort.Slice(hs, func(i, j int) bool { return hs[i].Name < hs[j].Name })

	var b strings.Builder
	for _, h := range hs {
		name := lipgloss.NewStyle().Foreground(colMethod).Render(h.Name + ": ")
		b.WriteString(name + h.Value + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
