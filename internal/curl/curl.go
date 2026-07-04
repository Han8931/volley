// Package curl converts between a model.Request and a curl command line, so
// users can paste a curl command to fill a request (:import curl …) or copy the
// current request as a runnable curl command (:copy curl).
//
// Import handles the flags devs actually paste — method, headers, data, basic
// auth, common metadata headers, and --max-time — and reports the flags it had
// to ignore rather than failing. It is deliberately not a full curl parser.
package curl

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

// Parse turns a curl command line into a Request, plus a list of human-readable
// warnings for anything it recognized but could not fully honor (unknown flags,
// extra arguments). It errors only on input it cannot tokenize or that carries
// no command at all.
func Parse(s string) (model.Request, []string, error) {
	toks, err := tokenize(s)
	if err != nil {
		return model.Request{}, nil, err
	}
	if len(toks) > 0 && toks[0] == "curl" {
		toks = toks[1:]
	}
	if len(toks) == 0 {
		return model.Request{}, nil, fmt.Errorf("no curl command found")
	}

	req := model.Request{}
	var warnings []string
	methodSet, bodySet := false, false

	addHeader := func(v string) {
		name, value := splitHeader(v)
		req.Headers = append(req.Headers, model.Header{Name: name, Value: value, Enabled: true})
	}

	for i := 0; i < len(toks); i++ {
		flag, inline, hasInline := normalizeFlag(toks[i])
		// value fetches the flag's argument: an attached "--flag=v"/"-Xv" form,
		// otherwise the next token.
		value := func() (string, bool) {
			if hasInline {
				return inline, true
			}
			if i+1 < len(toks) {
				i++
				return toks[i], true
			}
			return "", false
		}

		switch {
		case flag == "-X" || flag == "--request":
			if v, ok := value(); ok {
				req.Method, methodSet = strings.ToUpper(v), true
			}
		case flag == "-H" || flag == "--header":
			if v, ok := value(); ok {
				addHeader(v)
			}
		case isDataFlag(flag):
			if v, ok := value(); ok {
				if bodySet {
					req.Body += "&" + v // curl concatenates repeated -d with '&'
				} else {
					req.Body, bodySet = v, true
				}
			}
		case flag == "--url":
			if v, ok := value(); ok {
				req.URL = v
			}
		case flag == "-u" || flag == "--user":
			if v, ok := value(); ok {
				req.Headers = append(req.Headers, model.Header{
					Name:    "Authorization",
					Value:   "Basic " + base64.StdEncoding.EncodeToString([]byte(v)),
					Enabled: true,
				})
			}
		case flag == "-b" || flag == "--cookie":
			if v, ok := value(); ok {
				req.Headers = append(req.Headers, model.Header{Name: "Cookie", Value: v, Enabled: true})
			}
		case flag == "-A" || flag == "--user-agent":
			if v, ok := value(); ok {
				req.Headers = append(req.Headers, model.Header{Name: "User-Agent", Value: v, Enabled: true})
			}
		case flag == "-e" || flag == "--referer":
			if v, ok := value(); ok {
				req.Headers = append(req.Headers, model.Header{Name: "Referer", Value: v, Enabled: true})
			}
		case flag == "-m" || flag == "--max-time":
			if v, ok := value(); ok {
				if d, err := parseSeconds(v); err == nil {
					req.Timeout = d
				} else {
					warnings = append(warnings, "bad --max-time value "+v)
				}
			}
		case isIgnoredFlag(flag):
			// Common flags that don't affect the modeled request (-s, -L, --compressed, …).
		case isUnsupportedValueFlag(flag):
			if _, ok := value(); !ok {
				warnings = append(warnings, "ignored flag "+flag+" (missing value)")
			} else {
				warnings = append(warnings, "ignored flag "+flag)
			}
		case strings.HasPrefix(toks[i], "-") && toks[i] != "-":
			warnings = append(warnings, "ignored flag "+flag)
		default:
			if req.URL == "" {
				req.URL = toks[i]
			} else {
				warnings = append(warnings, "ignored extra argument "+toks[i])
			}
		}
	}

	// curl defaults to POST when a body is present and no method was given.
	if !methodSet {
		if bodySet {
			req.Method = "POST"
		} else {
			req.Method = "GET"
		}
	}
	if req.URL == "" {
		return model.Request{}, nil, fmt.Errorf("no URL found in curl command")
	}
	sort.Strings(warnings)
	return req, warnings, nil
}

// Format renders req as a multi-line curl command suitable for copying to a
// shell. The URL is used verbatim, so the caller should fold any query params
// into it (and expand variables) beforehand; the Query slice is ignored here.
func Format(req model.Request) string {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	hasBody := req.Body != ""

	var b strings.Builder
	b.WriteString("curl")
	if method != "GET" || hasBody {
		b.WriteString(" -X " + method)
	}
	b.WriteString(" \\\n  " + shellQuote(req.URL))
	for _, h := range req.Headers {
		if h.Enabled && h.Name != "" {
			b.WriteString(" \\\n  -H " + shellQuote(h.Name+": "+h.Value))
		}
	}
	if hasBody {
		b.WriteString(" \\\n  --data " + shellQuote(req.Body))
	}
	if req.Timeout > 0 {
		b.WriteString(" \\\n  --max-time " + formatSeconds(req.Timeout))
	}
	return b.String()
}

func isDataFlag(flag string) bool {
	switch flag {
	case "-d", "--data", "--data-raw", "--data-ascii", "--data-binary", "--data-urlencode":
		return true
	}
	return false
}

// isIgnoredFlag reports flags that are safe to drop because they don't change
// the request Volley models. Combined boolean shorts (e.g. -sSL) are ignored
// when every letter is itself an ignorable boolean flag.
func isIgnoredFlag(flag string) bool {
	switch flag {
	case "--compressed", "--location", "--insecure", "--silent", "--show-error",
		"--verbose", "--fail", "--globoff", "--progress-bar", "--http1.1", "--http2":
		return true
	}
	if strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && len(flag) >= 2 {
		for _, c := range flag[1:] {
			if !strings.ContainsRune("sSvkgiILf#", c) {
				return false
			}
		}
		return true
	}
	return false
}

// isUnsupportedValueFlag reports common curl options that take an argument but
// do not change the request shape Volley currently models. Consuming their
// value prevents it from being mistaken for the URL.
func isUnsupportedValueFlag(flag string) bool {
	switch flag {
	case "--connect-timeout", "--proxy", "-x", "--proxy-user", "--resolve",
		"--cacert", "--cert", "--key", "--interface", "--limit-rate",
		"--retry", "--retry-delay", "--header-binary", "--request-target":
		return true
	}
	return false
}

// normalizeFlag splits an attached-value flag ("--data=x" or "-Xv") into its
// flag and inline value. Only short flags that take a value are split so that
// combined boolean shorts (-sSL) pass through untouched.
func normalizeFlag(t string) (flag, val string, hasInline bool) {
	if strings.HasPrefix(t, "--") {
		if eq := strings.IndexByte(t, '='); eq >= 0 {
			return t[:eq], t[eq+1:], true
		}
		return t, "", false
	}
	if strings.HasPrefix(t, "-") && len(t) > 2 && strings.ContainsRune("XHduAebm", rune(t[1])) {
		return t[:2], t[2:], true
	}
	return t, "", false
}

func splitHeader(h string) (name, value string) {
	if i := strings.IndexByte(h, ':'); i >= 0 {
		return strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+1:])
	}
	if strings.HasSuffix(h, ";") { // curl's "Header;" sends an empty-valued header
		return strings.TrimSpace(strings.TrimSuffix(h, ";")), ""
	}
	return strings.TrimSpace(h), ""
}

func parseSeconds(v string) (time.Duration, error) {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return 0, fmt.Errorf("invalid seconds")
	}
	return time.Duration(f * float64(time.Second)), nil
}

// formatSeconds renders a duration as curl's --max-time seconds, trimming a
// trailing ".0" so whole seconds read as "30" rather than "30.0".
func formatSeconds(d time.Duration) string {
	s := strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
	return s
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// tokenize splits a command line into shell-style words, honoring single quotes
// (literal), double quotes (with \-escapes), backslash escapes, and backslash
// line continuations.
func tokenize(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inToken := false
	r := []rune(s)

	flush := func() {
		if inToken {
			tokens = append(tokens, cur.String())
			cur.Reset()
			inToken = false
		}
	}

	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c == '\'':
			inToken = true
			i++
			for i < len(r) && r[i] != '\'' {
				cur.WriteRune(r[i])
				i++
			}
			if i >= len(r) {
				return nil, fmt.Errorf("unterminated single quote")
			}
		case c == '"':
			inToken = true
			i++
			for i < len(r) && r[i] != '"' {
				if r[i] == '\\' && i+1 < len(r) {
					if n := r[i+1]; n == '"' || n == '\\' || n == '$' || n == '`' {
						cur.WriteRune(n)
						i += 2
						continue
					}
				}
				cur.WriteRune(r[i])
				i++
			}
			if i >= len(r) {
				return nil, fmt.Errorf("unterminated double quote")
			}
		case c == '\\':
			if i+1 < len(r) {
				if r[i+1] == '\n' {
					i++ // line continuation → treat as whitespace
					continue
				}
				inToken = true
				cur.WriteRune(r[i+1])
				i++
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		default:
			inToken = true
			cur.WriteRune(c)
		}
	}
	flush()
	return tokens, nil
}
