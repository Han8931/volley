// Package model holds Volley's core domain types shared across the TUI,
// the HTTP engine, and storage.
package model

import (
	"encoding/base64"
	"time"
)

// Header is a single HTTP header. Enabled lets the UI toggle a header
// off without deleting it (matching posting's behavior).
type Header struct {
	Name    string
	Value   string
	Enabled bool
}

// KV is a generic enabled-able key/value pair (query params, form fields).
type KV struct {
	Key     string
	Value   string
	Enabled bool
}

// Auth type discriminators for Auth.Type. The empty string means no auth
// helper is applied (the user may still hand-write an Authorization header).
const (
	AuthNone   = ""
	AuthBearer = "bearer"
	AuthBasic  = "basic"
	AuthAPIKey = "apikey"
)

// Auth is a request-level authentication helper. At send time it is
// materialized into the appropriate header (or query parameter) by
// Request.ApplyAuth, so the user never hand-writes the Authorization header.
// Field values may contain {{vars}}; callers expand them before ApplyAuth.
type Auth struct {
	Type string // one of AuthNone, AuthBearer, AuthBasic, AuthAPIKey

	Token string // Bearer

	Username string // Basic
	Password string // Basic

	Key     string // API key: header or query-param name
	Value   string // API key: value
	InQuery bool   // API key: place in the query string instead of a header
}

// Request is an editable HTTP request definition.
type Request struct {
	Method  string
	URL     string
	Headers []Header
	Query   []KV
	Body    string
	Auth    Auth
	// Timeout bounds this request; zero means use the engine default.
	Timeout time.Duration
}

// NewRequest returns a sensible blank request.
func NewRequest() Request {
	return Request{Method: "GET"}
}

// ApplyAuth returns a copy of r with its Auth materialized into a header or
// query parameter. It is a no-op when Type is AuthNone or the required fields
// are empty. Values are used verbatim, so expand {{vars}} first. The injected
// header is appended last, so — because httpx sets headers with Header.Set —
// it overrides a hand-written header of the same name.
func (r Request) ApplyAuth() Request {
	switch r.Auth.Type {
	case AuthBearer:
		if r.Auth.Token == "" {
			return r
		}
		r.Headers = appendHeader(r.Headers, "Authorization", "Bearer "+r.Auth.Token)
	case AuthBasic:
		if r.Auth.Username == "" && r.Auth.Password == "" {
			return r
		}
		enc := base64.StdEncoding.EncodeToString([]byte(r.Auth.Username + ":" + r.Auth.Password))
		r.Headers = appendHeader(r.Headers, "Authorization", "Basic "+enc)
	case AuthAPIKey:
		if r.Auth.Key == "" {
			return r
		}
		if r.Auth.InQuery {
			r.Query = appendKV(r.Query, r.Auth.Key, r.Auth.Value)
		} else {
			r.Headers = appendHeader(r.Headers, r.Auth.Key, r.Auth.Value)
		}
	}
	return r
}

// appendHeader returns a new slice with an enabled header appended, without
// aliasing the caller's backing array.
func appendHeader(hs []Header, name, value string) []Header {
	out := make([]Header, len(hs), len(hs)+1)
	copy(out, hs)
	return append(out, Header{Name: name, Value: value, Enabled: true})
}

// appendKV returns a new slice with an enabled key/value appended, without
// aliasing the caller's backing array.
func appendKV(kvs []KV, key, value string) []KV {
	out := make([]KV, len(kvs), len(kvs)+1)
	copy(out, kvs)
	return append(out, KV{Key: key, Value: value, Enabled: true})
}

// Response is the result of executing a Request.
type Response struct {
	Status     string
	StatusCode int
	Proto      string
	Headers    []Header
	Body       []byte
	Duration   time.Duration
	Size       int64
	// Truncated is true when the body exceeded the engine's read cap and only
	// the first Size bytes were kept.
	Truncated bool
	Err       error
}

// Methods is the ordered set of HTTP methods the method picker cycles through.
var Methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
