// Package httpx executes Volley requests against the network and adapts
// the result into model.Response. It is deliberately UI-agnostic so the
// same engine can later drive the load tester.
package httpx

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

// DefaultTimeout bounds a single request.
const DefaultTimeout = 30 * time.Second

// MaxResponseBytes caps how much of a response body we buffer into memory, so
// a huge or endless (streaming/chunked) response cannot exhaust memory or wedge
// the UI while it renders. Bodies larger than this are truncated and flagged.
const MaxResponseBytes = 10 << 20 // 10 MiB

// sharedClient is reused across every call so connections are pooled and kept
// alive between requests. A fresh http.Client per call (as this once did) opens
// a new TCP+TLS handshake every send and won't scale to the load tester, which
// fires many requests at one host. The per-request timeout and cancellation are
// applied via context rather than http.Client.Timeout, so one client can serve
// many concurrent requests, each with its own deadline and cancel signal.
var sharedClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// Do executes req and returns a populated Response. Transport/build errors
// are reported via Response.Err rather than a separate error return, so the
// UI has a single value to render (with timing) in every case. The caller's
// ctx is honored for cancellation; req.Timeout (or DefaultTimeout) is layered
// on as a per-request deadline.
func Do(ctx context.Context, req model.Request) model.Response {
	start := time.Now()

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}

	hr, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return model.Response{Err: err, Duration: time.Since(start)}
	}
	for _, h := range req.Headers {
		if h.Enabled && h.Name != "" {
			hr.Header.Set(h.Name, h.Value)
		}
	}

	resp, err := sharedClient.Do(hr)
	if err != nil {
		return model.Response{Err: err, Duration: time.Since(start)}
	}
	defer resp.Body.Close()

	// Read one byte past the cap so we can tell "exactly at the cap" from
	// "there was more we dropped".
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	dur := time.Since(start)
	if err != nil {
		return model.Response{Err: err, Duration: dur}
	}
	truncated := false
	if len(raw) > MaxResponseBytes {
		raw = raw[:MaxResponseBytes]
		truncated = true
	}

	var headers []model.Header
	for name, vals := range resp.Header {
		for _, v := range vals {
			headers = append(headers, model.Header{Name: name, Value: v, Enabled: true})
		}
	}

	return model.Response{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Proto:      resp.Proto,
		Headers:    headers,
		Body:       raw,
		Duration:   dur,
		Size:       int64(len(raw)),
		Truncated:  truncated,
	}
}

// DoLoad executes one load-test request and returns only the outcome needed by
// the load engine. Unlike Do, it does not retain response bodies or headers.
// The body is streamed to io.Discard so large responses do not multiply the
// load generator's memory use, and reading it fully preserves connection reuse.
func DoLoad(ctx context.Context, req model.Request) (int, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	hr, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return 0, err
	}
	for _, h := range req.Headers {
		if h.Enabled && h.Name != "" {
			hr.Header.Set(h.Name, h.Value)
		}
	}

	resp, err := sharedClient.Do(hr)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, nil
}
