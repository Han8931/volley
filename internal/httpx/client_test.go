package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

func TestDo_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Errorf("header X-Test = %q, want yes", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req := model.Request{
		Method:  "GET",
		URL:     srv.URL,
		Headers: []model.Header{{Name: "X-Test", Value: "yes", Enabled: true}},
	}
	resp := Do(context.Background(), req)

	if resp.Err != nil {
		t.Fatalf("unexpected error: %v", resp.Err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Errorf("body = %q", resp.Body)
	}
	if resp.Size != int64(len(resp.Body)) {
		t.Errorf("size = %d, want %d", resp.Size, len(resp.Body))
	}
	if resp.Duration <= 0 {
		t.Errorf("duration not recorded")
	}
}

func TestDo_BadURL(t *testing.T) {
	resp := Do(context.Background(), model.Request{Method: "GET", URL: "://nope"})
	if resp.Err == nil {
		t.Fatal("expected error for malformed URL")
	}
}

func TestDo_TruncatesLargeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write more than the cap so the read is forced to truncate.
		chunk := make([]byte, 1<<20)
		for i := 0; i < (MaxResponseBytes/len(chunk))+2; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	defer srv.Close()

	resp := Do(context.Background(), model.Request{Method: "GET", URL: srv.URL})
	if resp.Err != nil {
		t.Fatalf("unexpected error: %v", resp.Err)
	}
	if !resp.Truncated {
		t.Errorf("Truncated = false, want true for oversized body")
	}
	if got := int64(len(resp.Body)); got != MaxResponseBytes {
		t.Errorf("body length = %d, want cap %d", got, MaxResponseBytes)
	}
	if resp.Size != MaxResponseBytes {
		t.Errorf("Size = %d, want cap %d", resp.Size, MaxResponseBytes)
	}
}

func TestDo_TimeoutHonored(t *testing.T) {
	// The server hangs until the client's request context is done, so the only
	// way Do returns is by hitting the per-request timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	start := time.Now()
	resp := Do(context.Background(), model.Request{
		Method:  "GET",
		URL:     srv.URL,
		Timeout: 50 * time.Millisecond,
	})
	if resp.Err == nil {
		t.Fatal("expected a timeout error from a hanging server")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Do took %v; per-request timeout was not honored", elapsed)
	}
}

func TestDo_ParentContextCancel(t *testing.T) {
	// A long per-request timeout means the caller-cancellable context is the
	// only thing that can end this request — exercising the esc-to-abort path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	resp := Do(ctx, model.Request{Method: "GET", URL: srv.URL, Timeout: 30 * time.Second})
	if resp.Err == nil {
		t.Fatal("expected an error when the caller's context is cancelled")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Do took %v; caller cancellation was not honored", elapsed)
	}
}

func TestDo_SmallBodyNotTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	resp := Do(context.Background(), model.Request{Method: "GET", URL: srv.URL})
	if resp.Truncated {
		t.Errorf("Truncated = true, want false for small body")
	}
	if string(resp.Body) != "hello" {
		t.Errorf("body = %q, want hello", resp.Body)
	}
}

func TestDoLoadDrainsLargeBody(t *testing.T) {
	const chunks = 12
	chunk := make([]byte, 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		for i := 0; i < chunks; i++ {
			if _, err := w.Write(chunk); err != nil {
				t.Errorf("write response: %v", err)
				return
			}
		}
	}))
	defer srv.Close()

	status, err := DoLoad(context.Background(), model.Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("DoLoad: %v", err)
	}
	if status != http.StatusAccepted {
		t.Errorf("status = %d, want %d", status, http.StatusAccepted)
	}
}

func TestDoLoadParentContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := DoLoad(ctx, model.Request{Method: "GET", URL: srv.URL})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context cancellation", err)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0 before response", status)
	}
}
