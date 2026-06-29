package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"

	"github.com/tabularasa/volley/internal/model"
)

// statusColor maps an HTTP status class to a palette color.
func statusColor(code int) lipgloss.Color {
	switch {
	case code >= 200 && code < 300:
		return colOK
	case code >= 300 && code < 400:
		return lipgloss.Color("#38BDF8") // cyan
	case code >= 400 && code < 500:
		return colMethod // amber
	case code >= 500:
		return lipgloss.Color("#F87171") // red
	default:
		return colDim
	}
}

// renderStatusLine is the one-line summary shown atop the response pane.
func renderStatusLine(resp model.Response) string {
	if resp.Err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true).
			Render("✗ " + resp.Err.Error())
	}
	status := lipgloss.NewStyle().Foreground(statusColor(resp.StatusCode)).Bold(true).
		Render(resp.Status)
	meta := lipgloss.NewStyle().Foreground(colDim).Render(fmt.Sprintf(
		" · %s · %s",
		resp.Duration.Round(time.Millisecond),
		humanize.Bytes(uint64(resp.Size)),
	))
	return status + meta
}

// renderResponseBody returns the scrollable body content, pretty-printing
// JSON when the payload looks like JSON.
func renderResponseBody(resp model.Response, width int) string {
	if resp.Err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).
			Render("Request failed:\n\n" + resp.Err.Error())
	}
	body := resp.Body
	if pretty, ok := prettyJSON(body); ok {
		body = pretty
	}
	out := string(body)
	if out == "" {
		return dim("(empty response body)")
	}
	return out
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

// headerSummary renders response headers as a compact block (used later by
// a Headers tab; kept here so response rendering lives together).
func headerSummary(resp model.Response) string {
	var b strings.Builder
	for _, h := range resp.Headers {
		b.WriteString(dim(h.Name+": ") + h.Value + "\n")
	}
	return b.String()
}
