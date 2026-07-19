package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tabularasa/volley/internal/httpx"
	"github.com/tabularasa/volley/internal/model"
)

// TestEngineDrivesHTTPX exercises the wiring the TUI uses: the engine's DoFunc
// wrapping httpx.DoLoad against a live server, pooled connections and all.
func TestEngineDrivesHTTPX(t *testing.T) {
	const serverDelay = 5 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(serverDelay)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req := model.Request{Method: "GET", URL: srv.URL, Timeout: 2 * time.Second}
	run, err := Runner{
		Profile: shortConstant(50, 300*time.Millisecond),
		Do: func(ctx context.Context) (int, error) {
			return httpx.DoLoad(ctx, req)
		},
	}.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-run.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("run did not finish")
	}
	snap := run.Snapshot()
	if snap.Completed < 5 {
		t.Fatalf("expected a paced batch of completions, got %+v", snap)
	}
	if snap.Errors != 0 || snap.Dropped != 0 {
		t.Errorf("local server should serve cleanly: %+v", snap)
	}
	if snap.P50 < serverDelay {
		t.Errorf("P50 %v cannot be below the server's %v delay", snap.P50, serverDelay)
	}
	if len(snap.Buckets) == 0 || snap.Buckets[0].Completed == 0 {
		t.Errorf("first-second bucket should hold completions: %+v", snap.Buckets)
	}
}
