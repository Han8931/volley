package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
