// Package codegen renders a built request as a runnable command in several
// CLI dialects — the GUI's "generate code" feature (Bruno-style). Like
// curl.Format, every generator expects the BUILT request: {{vars}} expanded,
// auth materialized into headers, query params folded into the URL.
package codegen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tabularasa/volley/internal/curl"
	"github.com/tabularasa/volley/internal/model"
)

// Formats lists the supported dialects in display order.
var Formats = []string{"curl", "wget", "httpie"}

// Generate renders req in the named format.
func Generate(format string, req model.Request) (string, error) {
	switch format {
	case "curl":
		return curl.Format(req), nil
	case "wget":
		return Wget(req), nil
	case "httpie":
		return HTTPie(req), nil
	default:
		return "", fmt.Errorf("unknown code format %q (want %s)", format, strings.Join(Formats, ", "))
	}
}

// Wget renders req as a multi-line wget command printing the response body
// to stdout.
func Wget(req model.Request) string {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	var b strings.Builder
	b.WriteString("wget -qO-")
	if method != "GET" {
		b.WriteString(" \\\n  --method=" + method)
	}
	for _, h := range req.Headers {
		if h.Enabled && h.Name != "" {
			b.WriteString(" \\\n  --header=" + shellQuote(h.Name+": "+h.Value))
		}
	}
	if req.Body != "" {
		b.WriteString(" \\\n  --body-data=" + shellQuote(req.Body))
	}
	if req.Timeout > 0 {
		b.WriteString(" \\\n  --timeout=" + strconv.Itoa(int(req.Timeout.Seconds()+0.999)))
	}
	b.WriteString(" \\\n  " + shellQuote(req.URL))
	return b.String()
}

// HTTPie renders req as a multi-line httpie command. The raw body is passed
// with --raw so JSON isn't re-encoded; headers use httpie's Name:value items.
func HTTPie(req model.Request) string {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	var b strings.Builder
	b.WriteString("http")
	if req.Timeout > 0 {
		b.WriteString(" --timeout=" + strconv.FormatFloat(req.Timeout.Seconds(), 'f', -1, 64))
	}
	if method != "GET" || req.Body != "" {
		b.WriteString(" " + method)
	}
	b.WriteString(" " + shellQuote(req.URL))
	// httpie header items keep a stable order for reproducible output.
	headers := make([]model.Header, 0, len(req.Headers))
	for _, h := range req.Headers {
		if h.Enabled && h.Name != "" {
			headers = append(headers, h)
		}
	}
	sort.SliceStable(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	for _, h := range headers {
		b.WriteString(" \\\n  " + shellQuote(h.Name+":"+h.Value))
	}
	if req.Body != "" {
		b.WriteString(" \\\n  --raw " + shellQuote(req.Body))
	}
	return b.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
