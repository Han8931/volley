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

// Do executes req and returns a populated Response. Transport/build errors
// are reported via Response.Err rather than a separate error return, so the
// UI has a single value to render (with timing) in every case.
func Do(ctx context.Context, req model.Request) model.Response {
	start := time.Now()

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

	client := &http.Client{Timeout: DefaultTimeout}
	resp, err := client.Do(hr)
	if err != nil {
		return model.Response{Err: err, Duration: time.Since(start)}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	dur := time.Since(start)
	if err != nil {
		return model.Response{Err: err, Duration: dur}
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
	}
}
