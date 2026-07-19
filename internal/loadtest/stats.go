package loadtest

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Stats aggregates the outcomes of a run. It is written by worker goroutines
// and read by the UI through Snapshot, so all access is mutex-guarded.
type Stats struct {
	mu        sync.Mutex
	start     time.Time
	latencies []time.Duration // completed requests, in completion order
	errors    int
	canceled  int
	dropped   int
	inFlight  int
	buckets   []Bucket // per-second aggregates for the live chart
	done      bool
	elapsed   time.Duration // frozen at completion; live until then
}

// Bucket is one second of the run, for plotting achieved load over time.
type Bucket struct {
	Sent       int           // requests dispatched in this second
	Completed  int           // responses (ok or error) received in this second
	Errors     int           // completions that were transport errors or 5xx
	Canceled   int           // completions aborted through context cancellation
	SumLatency time.Duration // sum over Completed, for a mean
}

// MeanLatency is the average latency of completions in the bucket.
func (b Bucket) MeanLatency() time.Duration {
	if b.Completed == 0 {
		return 0
	}
	return b.SumLatency / time.Duration(b.Completed)
}

// Snapshot is a point-in-time copy of a run's aggregate results, safe to
// render while the run continues.
type Snapshot struct {
	Elapsed       time.Duration
	Done          bool
	Sent          int // dispatched to a worker
	Completed     int // responses/errors completed; excludes cancellations
	Errors        int // transport errors + 5xx responses
	Canceled      int // requests aborted by stopping/cancelling the run
	Dropped       int // scheduled sends that found no free worker
	InFlight      int
	AchievedRPS   float64 // completions per second of elapsed time
	P50, P95, P99 time.Duration
	Max           time.Duration
	Buckets       []Bucket
}

func newStats(start time.Time) *Stats {
	return &Stats{start: start}
}

// bucketAt grows the series to include the bucket for offset and returns it.
func (s *Stats) bucketAt(offset time.Duration) *Bucket {
	i := int(offset / time.Second)
	if i < 0 {
		i = 0
	}
	for len(s.buckets) <= i {
		s.buckets = append(s.buckets, Bucket{})
	}
	return &s.buckets[i]
}

// recordSent notes a request handed to a worker.
func (s *Stats) recordSent(offset time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight++
	s.bucketAt(offset).Sent++
}

// recordResult notes a completed request in the bucket where it completed.
// Context cancellation is kept separate from target failures so stopping a
// run does not make the service's error rate look worse.
func (s *Stats) recordResult(offset, latency time.Duration, status int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight--
	b := s.bucketAt(offset)
	if errors.Is(err, context.Canceled) {
		s.canceled++
		b.Canceled++
		return
	}
	s.latencies = append(s.latencies, latency)
	b.Completed++
	b.SumLatency += latency
	if err != nil || status >= 500 {
		s.errors++
		b.Errors++
	}
}

// recordDropped notes a scheduled request that found no free worker.
func (s *Stats) recordDropped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropped++
}

// finish freezes the elapsed clock at the run's true end.
func (s *Stats) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	s.elapsed = time.Since(s.start)
}

// Snapshot copies the current aggregates. Percentiles are computed over all
// completions so far — exact, not estimated, which is affordable at TUI-scale
// request counts.
func (s *Stats) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	elapsed := s.elapsed
	if !s.done {
		elapsed = time.Since(s.start)
	}
	snap := Snapshot{
		Elapsed:   elapsed,
		Done:      s.done,
		Completed: len(s.latencies),
		Errors:    s.errors,
		Canceled:  s.canceled,
		Dropped:   s.dropped,
		InFlight:  s.inFlight,
		Buckets:   append([]Bucket(nil), s.buckets...),
	}
	snap.Sent = snap.Completed + snap.Canceled + s.inFlight
	if secs := elapsed.Seconds(); secs > 0 {
		snap.AchievedRPS = float64(snap.Completed) / secs
	}
	if len(s.latencies) > 0 {
		sorted := append([]time.Duration(nil), s.latencies...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		snap.P50 = percentile(sorted, 0.50)
		snap.P95 = percentile(sorted, 0.95)
		snap.P99 = percentile(sorted, 0.99)
		snap.Max = sorted[len(sorted)-1]
	}
	return snap
}

// percentile returns the nearest-rank percentile of an ascending-sorted slice.
func percentile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(q*float64(len(sorted))+0.5) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
