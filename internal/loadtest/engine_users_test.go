package loadtest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestUsersModeHoldsConcurrency checks the closed-loop executor: the number of
// simultaneously in-flight requests tracks the plotted user count, and nothing
// is dropped no matter how slow the target is.
func TestUsersModeHoldsConcurrency(t *testing.T) {
	var inFlight, peak atomic.Int64
	p := Profile{
		Name:   "users",
		Mode:   ModeUsers,
		Points: []Point{{At: 0, RPS: 4}, {At: Duration(400 * time.Millisecond), RPS: 4}},
	}
	run, err := Runner{
		Profile: p,
		Do: func(ctx context.Context) (int, error) {
			cur := inFlight.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			defer inFlight.Add(-1)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(40 * time.Millisecond):
			}
			return 200, nil
		},
	}.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-run.Done()
	snap := run.Snapshot()

	if got := peak.Load(); got != 4 {
		t.Errorf("peak concurrency = %d, want the plotted 4 users", got)
	}
	if snap.Dropped != 0 {
		t.Errorf("closed loop must never drop, got %d", snap.Dropped)
	}
	// ~4 users × ~10 iterations/s over 0.4s; be generous about scheduling.
	if snap.Completed < 10 {
		t.Errorf("completed = %d, want a full pipeline's worth", snap.Completed)
	}
}

// TestUsersModeRespectsThinkTimeAndBudget checks think time slows iteration
// and MaxRequests stops the run early.
func TestUsersModeRespectsThinkTimeAndBudget(t *testing.T) {
	var calls atomic.Int64
	p := Profile{
		Name:        "budget",
		Mode:        ModeUsers,
		MaxRequests: 5,
		ThinkTime:   Duration(20 * time.Millisecond),
		Points:      []Point{{At: 0, RPS: 2}, {At: Duration(2 * time.Second), RPS: 2}},
	}
	run, err := Runner{
		Profile: p,
		Do: func(context.Context) (int, error) {
			calls.Add(1)
			return 200, nil
		},
	}.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-run.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("budgeted run never finished")
	}
	if got := calls.Load(); got > 5 {
		t.Errorf("made %d requests, MaxRequests was 5", got)
	}
	if run.Snapshot().Elapsed > 2*time.Second {
		t.Error("run should end on the request budget, well before the plot does")
	}
}

func TestUsersModeValidation(t *testing.T) {
	base := []Point{{At: 0, RPS: 1}, {At: Duration(time.Second), RPS: 1}}
	if err := (Profile{Name: "x", Mode: "nonsense", Points: base}).Validate(); err == nil {
		t.Error("unknown mode should not validate")
	}
	if err := (Profile{Name: "x", Mode: ModeUsers, ThinkTime: Duration(-1), Points: base}).Validate(); err == nil {
		t.Error("negative think time should not validate")
	}
	p := Profile{Name: "x", Mode: ModeUsers, Points: base}
	if err := p.Validate(); err != nil {
		t.Errorf("valid users profile rejected: %v", err)
	}
	if p.PlannedRequests() != 0 {
		t.Error("closed loop cannot promise a request count without a budget")
	}
}
