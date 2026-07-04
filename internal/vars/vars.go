// Package vars resolves {{name}} placeholders in a request using a
// user-defined store with a fallback to process environment variables.
package vars

import (
	"os"
	"regexp"
	"sort"

	"github.com/tabularasa/volley/internal/model"
)

// placeholder matches {{ name }} with optional surrounding whitespace.
var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

// Store holds user-defined variables (e.g. via ":set token=abc").
type Store map[string]string

// New returns an empty store.
func New() Store { return Store{} }

// Set defines or overwrites a variable.
func (s Store) Set(name, value string) { s[name] = value }

// Expand replaces every {{name}} in text. Resolution order: the store, then
// the process environment. Unknown placeholders are left untouched so the
// user can see what failed to resolve.
func (s Store) Expand(text string) string {
	return placeholder.ReplaceAllStringFunc(text, func(match string) string {
		name := placeholder.FindStringSubmatch(match)[1]
		if v, ok := s[name]; ok {
			return v
		}
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return match
	})
}

// Unresolved returns the sorted, unique names of {{placeholders}} that remain
// in the parts of req that will actually be sent — its URL, body, enabled
// header names/values, and enabled query keys/values. Query rows are inspected
// directly rather than via the URL, since folding query params into the URL
// percent-encodes the braces (so {{x}} would hide as %7B%7Bx%7D%7D). An empty
// result means every variable resolved.
func Unresolved(req model.Request) []string {
	seen := map[string]struct{}{}
	collect := func(text string) {
		for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
			seen[m[1]] = struct{}{}
		}
	}
	collect(req.URL)
	collect(req.Body)
	for _, h := range req.Headers {
		if h.Enabled {
			collect(h.Name)
			collect(h.Value)
		}
	}
	for _, q := range req.Query {
		if q.Enabled {
			collect(q.Key)
			collect(q.Value)
		}
	}
	switch req.Auth.Type {
	case model.AuthBearer:
		collect(req.Auth.Token)
	case model.AuthBasic:
		collect(req.Auth.Username)
		collect(req.Auth.Password)
	case model.AuthAPIKey:
		collect(req.Auth.Key)
		collect(req.Auth.Value)
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Apply returns a copy of req with placeholders expanded across the URL,
// header names/values, query keys/values, and body.
func (s Store) Apply(req model.Request) model.Request {
	out := req
	out.URL = s.Expand(req.URL)
	out.Body = s.Expand(req.Body)

	out.Headers = make([]model.Header, len(req.Headers))
	for i, h := range req.Headers {
		out.Headers[i] = model.Header{
			Name:    s.Expand(h.Name),
			Value:   s.Expand(h.Value),
			Enabled: h.Enabled,
		}
	}

	out.Query = make([]model.KV, len(req.Query))
	for i, kv := range req.Query {
		out.Query[i] = model.KV{
			Key:     s.Expand(kv.Key),
			Value:   s.Expand(kv.Value),
			Enabled: kv.Enabled,
		}
	}

	out.Auth = req.Auth
	out.Auth.Token = s.Expand(req.Auth.Token)
	out.Auth.Username = s.Expand(req.Auth.Username)
	out.Auth.Password = s.Expand(req.Auth.Password)
	out.Auth.Key = s.Expand(req.Auth.Key)
	out.Auth.Value = s.Expand(req.Auth.Value)
	return out
}
