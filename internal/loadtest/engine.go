package loadtest

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
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
	if r.Profile.Users() {
		go run.paceUsers(ctx, r)
	} else {
		go run.pace(ctx, r)
	}
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

// paceUsers is the closed-loop scheduler: it keeps the pool of virtual users
// matched to the plotted user count, and each user loops send → await → think
// on its own. Nothing is ever dropped here — a slow target just means each
// user completes fewer iterations, which is the whole point of the model.
func (run *Run) paceUsers(ctx context.Context, r Runner) {
	defer close(run.done)
	defer run.cancel()
	defer run.stats.finish()
	var users sync.WaitGroup
	defer users.Wait()

	// Retiring a user closes its stop channel rather than cancelling its
	// context, so a user that is mid-request finishes it instead of logging a
	// cancellation. Run-level ctx cancellation still aborts immediately.
	var pool []chan struct{}
	defer func() {
		for _, stop := range pool {
			close(stop)
		}
	}()

	var sent atomic.Int64
	total := r.Profile.Duration()
	cap := r.Profile.maxWorkers()
	ticker := time.NewTicker(paceTick)
	defer ticker.Stop()

	for {
		elapsed := time.Since(run.start)
		want := int(math.Round(r.Profile.TargetAt(elapsed)))
		if want < 0 {
			want = 0
		}
		if want > cap {
			want = cap
		}
		for len(pool) < want {
			stop := make(chan struct{})
			pool = append(pool, stop)
			users.Add(1)
			go run.virtualUser(ctx, stop, r, &users, &sent)
		}
		for len(pool) > want {
			last := len(pool) - 1
			close(pool[last])
			pool = pool[:last]
		}
		if elapsed >= total {
			return
		}
		if r.Profile.MaxRequests > 0 && int(sent.Load()) >= r.Profile.MaxRequests {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// virtualUser is one simulated person: request, wait for the answer, think,
// repeat until retired (stop), cancelled (ctx), or the request budget is out.
func (run *Run) virtualUser(ctx context.Context, stop <-chan struct{}, r Runner, wg *sync.WaitGroup, sent *atomic.Int64) {
	defer wg.Done()
	think := time.Duration(r.Profile.ThinkTime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		default:
		}
		if r.Profile.MaxRequests > 0 && int(sent.Add(1)) > r.Profile.MaxRequests {
			return
		}
		run.stats.recordSent(time.Since(run.start))
		begin := time.Now()
		status, err := r.Do(ctx)
		run.stats.recordResult(time.Since(run.start), time.Since(begin), status, err)
		if think <= 0 {
			continue
		}
		timer := time.NewTimer(think)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
