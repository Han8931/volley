// Package model holds Volley's core domain types shared across the TUI,
// the HTTP engine, and storage.
package model

import "time"

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

// Request is an editable HTTP request definition.
type Request struct {
	Method  string
	URL     string
	Headers []Header
	Query   []KV
	Body    string
	// Timeout bounds this request; zero means use the engine default.
	Timeout time.Duration
}

// NewRequest returns a sensible blank request.
func NewRequest() Request {
	return Request{Method: "GET"}
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
	Err        error
}

// Methods is the ordered set of HTTP methods the method picker cycles through.
var Methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
