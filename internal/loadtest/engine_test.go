package loadtest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// shortConstant is a fast profile for real-clock engine tests: rps for dur.
func shortConstant(rps float64, dur time.Duration) Profile {
	return Profile{
		Name:   "test",
		Points: []Point{{At: 0, RPS: rps}, {At: Duration(dur), RPS: rps}},
	}
}

func TestRunCompletesProfile(t *testing.T) {
	var calls atomic.Int64
	run, err := Runner{
		Profile: shortConstant(100, 300*time.Millisecond),
		Do: func(ctx context.Context) (int, error) {
			calls.Add(1)
			return 200, nil
		},
	}.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-run.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("run did not finish")
	}
	snap := run.Snapshot()
	if !snap.Done {
		t.Error("snapshot should report done")
	}
	// 100 RPS × 0.3s = 30 expected; allow generous scheduling slack.
	if got := calls.Load(); got < 20 || got > 40 {
		t.Errorf("calls = %d, want ≈30", got)
	}
	if snap.Completed != int(calls.Load()) {
		t.Errorf("Completed = %d, calls = %d", snap.Completed, calls.Load())
	}
	if snap.Errors != 0 || snap.Dropped != 0 || snap.InFlight != 0 {
		t.Errorf("clean run: %+v", snap)
	}
	if snap.P50 < 0 || snap.Max < snap.P50 {
		t.Errorf("percentiles inconsistent: %+v", snap)
	}
}

func TestRunCountsErrors(t *testing.T) {
	var n atomic.Int64
	run, err := Runner{
		Profile: shortConstant(50, 200*time.Millisecond),
		Do: func(ctx context.Context) (int, error) {
			if n.Add(1)%2 == 0 {
				return 500, nil // 5xx counts as an error even without a transport failure
			}
			return 0, errors.New("boom")
		},
	}.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-run.Done()
	snap := run.Snapshot()
	if snap.Completed == 0 || snap.Errors != snap.Completed {
		t.Errorf("every completion should be an error: %+v", snap)
	}
}

func TestStopCancelsInFlight(t *testing.T) {
	release := make(chan struct{})
	run, err := Runner{
		Profile: shortConstant(50, 5*time.Second),
		Do: func(ctx context.Context) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-release:
				return 200, nil
			}
		},
	}.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // let some requests get in flight
	run.Stop()
	select {
	case <-run.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Stop must drain and close Done without waiting the full profile")
	}
	if snap := run.Snapshot(); snap.InFlight != 0 || !snap.Done {
		t.Errorf("after stop: %+v", snap)
	}
}

func TestSaturatedWorkersDrop(t *testing.T) {
	p := shortConstant(100, 300*time.Millisecond)
	p.MaxWorkers = 1
	block := make(chan struct{})
	var entered atomic.Int64
	run, err := Runner{
		Profile: p,
		Do: func(ctx context.Context) (int, error) {
			entered.Add(1)
			select {
			case <-ctx.Done():
			case <-block:
			}
			return 200, nil
		},
	}.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Done drains in-flight requests on natural completion, so the blocked
	// worker must be released after the profile window has passed.
	time.Sleep(400 * time.Millisecond)
	close(block)
	select {
	case <-run.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("run did not drain after the worker was released")
	}
	snap := run.Snapshot()
	if snap.Dropped == 0 {
		t.Errorf("a saturated single worker must drop scheduled sends: %+v", snap)
	}
	if got := entered.Load(); got > 1 {
		t.Errorf("only one request can run with MaxWorkers=1, got %d", got)
	}
}

func TestStartValidates(t *testing.T) {
	if _, err := (Runner{Profile: shortConstant(1, time.Second)}).Start(context.Background()); err == nil {
		t.Error("nil Do must be rejected")
	}
	bad := Profile{Points: []Point{{At: 0, RPS: 1}}}
	if _, err := (Runner{Profile: bad, Do: func(context.Context) (int, error) { return 200, nil }}).Start(context.Background()); err == nil {
		t.Error("invalid profile must be rejected")
	}
}

func TestSnapshotBuckets(t *testing.T) {
	s := newStats(time.Now())
	s.recordSent(100 * time.Millisecond)
	s.recordResult(100*time.Millisecond, 20*time.Millisecond, 200, nil)
	s.recordSent(1500 * time.Millisecond)
	s.recordResult(1500*time.Millisecond, 40*time.Millisecond, 502, nil)
	s.recordDropped()
	s.finish()

	snap := s.Snapshot()
	if len(snap.Buckets) != 2 {
		t.Fatalf("buckets = %d, want 2", len(snap.Buckets))
	}
	if b := snap.Buckets[0]; b.Sent != 1 || b.Completed != 1 || b.Errors != 0 || b.MeanLatency() != 20*time.Millisecond {
		t.Errorf("bucket 0 = %+v", b)
	}
	if b := snap.Buckets[1]; b.Sent != 1 || b.Errors != 1 {
		t.Errorf("bucket 1 = %+v", b)
	}
	if snap.Errors != 1 || snap.Dropped != 1 || snap.Completed != 2 {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestPercentile(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	sorted := []time.Duration{ms(10), ms(20), ms(30), ms(40), ms(100)}
	if got := percentile(sorted, 0.50); got != ms(30) {
		t.Errorf("p50 = %v, want 30ms", got)
	}
	if got := percentile(sorted, 0.99); got != ms(100) {
		t.Errorf("p99 = %v, want 100ms", got)
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("empty percentile = %v", got)
	}
}
