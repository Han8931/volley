package loadtest

import (
	"context"
	"errors"
	"sync"
	"time"
)

// paceTick is how often the pacer compares dispatched requests against the
// profile's expected-arrivals curve. 10ms keeps bursts imperceptible at TUI
// rates while staying cheap.
const paceTick = 10 * time.Millisecond

// DoFunc executes one request and reports its HTTP status (0 if none) and
// transport error. The engine measures latency around the call; the context
// is cancelled when the run stops.
type DoFunc func(ctx context.Context) (status int, err error)

// Runner configures a load run: which profile to follow and how to perform a
// single request.
type Runner struct {
	Profile Profile
	Do      DoFunc
}

// Run is a load test in progress. Poll Snapshot for live results; Done closes
// once every in-flight request has finished. When the profile completes
// naturally, in-flight requests are drained rather than cancelled — their
// latencies are the slow tail a load test exists to measure, and each is
// bounded by its own request timeout. Stop (or the parent context) cancels
// them instead.
type Run struct {
	stats  *Stats
	done   chan struct{}
	cancel context.CancelFunc
	start  time.Time
}

// Start validates the runner and begins the paced run. The run stops early
// when ctx is cancelled or Stop is called; either way in-flight requests are
// cancelled through their context.
func (r Runner) Start(ctx context.Context) (*Run, error) {
	if r.Do == nil {
		return nil, errors.New("loadtest: Runner.Do is nil")
	}
	if err := r.Profile.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	start := time.Now()
	run := &Run{
		stats:  newStats(start),
		done:   make(chan struct{}),
		cancel: cancel,
		start:  start,
	}
	go run.pace(ctx, r)
	return run, nil
}

// Snapshot returns the run's current aggregate results.
func (r *Run) Snapshot() Snapshot { return r.stats.Snapshot() }

// Done closes when the run has fully finished (profile complete or stopped,
// all workers drained).
func (r *Run) Done() <-chan struct{} { return r.done }

// Stop aborts the run: no more requests are scheduled and in-flight ones are
// cancelled. Done still closes only after workers drain.
func (r *Run) Stop() { r.cancel() }

// pace is the scheduler loop: every tick it advances a dispatch counter to the
// profile's expected-arrivals curve (self-correcting for drift) and hands each
// due request to a worker goroutine. A due request that finds no free worker
// slot is dropped and counted — the open-loop answer to a target that can't
// keep up within the concurrency cap.
func (run *Run) pace(ctx context.Context, r Runner) {
	// Deferred wind-down runs LIFO: drain the workers (with ctx still live, so
	// a naturally-completed run lets them finish), then freeze the elapsed
	// clock, release the context, and finally signal Done.
	defer close(run.done)
	defer run.cancel()
	defer run.stats.finish()
	var workers sync.WaitGroup
	defer workers.Wait()

	slots := make(chan struct{}, r.Profile.maxWorkers())
	total := r.Profile.Duration()
	ticker := time.NewTicker(paceTick)
	defer ticker.Stop()

	dispatched := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		elapsed := time.Since(run.start)
		due := int(r.Profile.ExpectedArrivals(elapsed))
		if r.Profile.MaxRequests > 0 && due > r.Profile.MaxRequests {
			due = r.Profile.MaxRequests
		}
		for dispatched < due {
			dispatched++
			select {
			case slots <- struct{}{}:
				workers.Add(1)
				go func(offset time.Duration) {
					defer workers.Done()
					defer func() { <-slots }()
					run.stats.recordSent(offset)
					begin := time.Now()
					status, err := r.Do(ctx)
					run.stats.recordResult(time.Since(run.start), time.Since(begin), status, err)
				}(elapsed)
			default:
				run.stats.recordDropped()
			}
		}
		if r.Profile.MaxRequests > 0 && dispatched >= r.Profile.MaxRequests {
			return
		}
		if elapsed >= total {
			return
		}
	}
}
