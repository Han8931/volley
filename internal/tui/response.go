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
	sizeStr := humanize.Bytes(uint64(resp.Size))
	if resp.Truncated {
		sizeStr += "+ (truncated)"
	}
	meta := lipgloss.NewStyle().Foreground(colDim).Render(fmt.Sprintf(
		" · %s · %s",
		resp.Duration.Round(time.Millisecond),
		sizeStr,
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
