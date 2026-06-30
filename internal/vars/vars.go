// Package vars resolves {{name}} placeholders in a request using a
// user-defined store with a fallback to process environment variables.
package vars

import (
	"os"
	"regexp"

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

// Apply returns a copy of req with placeholders expanded across the URL,
// header names/values, query values, and body.
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
	return out
}
