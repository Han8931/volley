// Package build turns an editable request into the final wire request:
// {{vars}} expanded through the caller's resolver, the auth helper
// materialized into a header or query param, and query rows folded into the
// URL. The TUI and the GUI share it so a request sends identically in both.
package build

import (
	"net/url"

	"github.com/tabularasa/volley/internal/model"
	"github.com/tabularasa/volley/internal/vars"
)

// Final composes the full pipeline over a raw editable request.
func Final(req model.Request, resolver vars.Layered) model.Request {
	out := resolver.Apply(req)
	out = out.ApplyAuth()
	out.URL = AppendQuery(out.URL, out.Query)
	return out
}

// AppendQuery merges enabled query rows into base's query string.
func AppendQuery(base string, kvs []model.KV) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for _, kv := range kvs {
		if kv.Enabled && kv.Key != "" {
			q.Add(kv.Key, kv.Value)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
