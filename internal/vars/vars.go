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
func (s Store) Expand(text string) string { return Layered{s}.Expand(text) }

// Layered resolves placeholders through an ordered list of scopes — the first
// scope that defines a name wins (e.g. session :set overrides, then the active
// environment) — with the process environment as the final fallback. Unknown
// placeholders are left untouched, matching Store.Expand.
type Layered []map[string]string

// Expand replaces every {{name}} in text using the layered resolution order.
func (l Layered) Expand(text string) string {
	return placeholder.ReplaceAllStringFunc(text, func(match string) string {
		name := placeholder.FindStringSubmatch(match)[1]
		for _, scope := range l {
			if v, ok := scope[name]; ok {
				return v
			}
		}
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return match
	})
}

// Lookup reports the value name resolves to and whether any scope (or the
// process environment) defines it.
func (l Layered) Lookup(name string) (string, bool) {
	for _, scope := range l {
		if v, ok := scope[name]; ok {
			return v, true
		}
	}
	return os.LookupEnv(name)
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
func (s Store) Apply(req model.Request) model.Request { return Layered{s}.Apply(req) }

// Apply returns a copy of req with placeholders expanded across the URL,
// header names/values, query keys/values, and body, using the layered
// resolution order.
func (l Layered) Apply(req model.Request) model.Request {
	out := req
	out.URL = l.Expand(req.URL)
	out.Body = l.Expand(req.Body)

	out.Headers = make([]model.Header, len(req.Headers))
	for i, h := range req.Headers {
		out.Headers[i] = model.Header{
			Name:    l.Expand(h.Name),
			Value:   l.Expand(h.Value),
			Enabled: h.Enabled,
		}
	}

	out.Query = make([]model.KV, len(req.Query))
	for i, kv := range req.Query {
		out.Query[i] = model.KV{
			Key:     l.Expand(kv.Key),
			Value:   l.Expand(kv.Value),
			Enabled: kv.Enabled,
		}
	}

	out.Auth = req.Auth
	out.Auth.Token = l.Expand(req.Auth.Token)
	out.Auth.Username = l.Expand(req.Auth.Username)
	out.Auth.Password = l.Expand(req.Auth.Password)
	out.Auth.Key = l.Expand(req.Auth.Key)
	out.Auth.Value = l.Expand(req.Auth.Value)
	return out
}
