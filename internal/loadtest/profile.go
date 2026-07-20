// Package loadtest implements Volley's load-testing engine: open-loop request
// pacing driven by a load profile — a piecewise-linear plot of target request
// rate (y, requests/second) over time (x). Profiles are plain JSON so they can
// be saved, shared, and hand-edited like collections; the engine is UI-agnostic
// and reports progress through pollable snapshots.
package loadtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Duration marshals as a human-readable string ("30s", "1m30s") in profile
// JSON, and accepts either that form or integer nanoseconds when parsing.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("bad duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("duration must be a string like \"30s\"")
	}
	*d = Duration(n)
	return nil
}

// Point is one vertex of the load plot: the target rate at an offset from the
// start of the run. The rate between points is linearly interpolated. Two
// points at the same offset form a vertical jump (for step/spike shapes); the
// later point wins at exactly that instant.
type Point struct {
	At  Duration `json:"at"`
	RPS float64  `json:"rps"`
}

// DefaultMaxWorkers caps concurrent in-flight requests when a profile does not
// set its own limit, so a slow target cannot make the engine spawn unbounded
// work (scheduled requests that find no free worker are counted as dropped).
const DefaultMaxWorkers = 64

// Execution modes. The shape means different things in each:
//
//	ModeRate  (default) — open loop. The plot is target requests/second;
//	                      arrivals are scheduled on a clock regardless of how
//	                      fast the target answers, so a slow target produces
//	                      drops. Answers "can it sustain N rps?".
//	ModeUsers            — closed loop. The plot is the number of concurrent
//	                      virtual users, each looping send → await response →
//	                      think → repeat. Throughput is an OUTCOME, so a slow
//	                      target simply completes fewer requests, never drops.
//	                      Answers "what happens with N people using it?".
const (
	ModeRate  = ""
	ModeUsers = "users"
)

// Profile is a named, saveable load shape.
type Profile struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Mode selects the executor; "" (rate) keeps every existing profile
	// behaving exactly as before.
	Mode string `json:"mode,omitempty"`
	// Points plot the target over time. The y value is requests/second in
	// rate mode and concurrent users in users mode.
	Points []Point `json:"points"`
	// MaxRequests stops scheduling after this many planned arrivals. Zero
	// means the shape's full duration determines the request count.
	MaxRequests int `json:"maxRequests,omitempty"`
	// MaxWorkers bounds concurrent in-flight requests; 0 means
	// DefaultMaxWorkers. In users mode it also caps the user count.
	MaxWorkers int `json:"maxWorkers,omitempty"`
	// ThinkTime is how long each virtual user waits between its requests
	// (users mode only); 0 means hammer with no pause.
	ThinkTime Duration `json:"thinkTime,omitempty"`
}

// Users reports whether this profile runs the closed-loop user executor.
func (p Profile) Users() bool { return p.Mode == ModeUsers }

// Validate reports whether the profile describes a runnable plot.
func (p Profile) Validate() error {
	if len(p.Points) < 2 {
		return errors.New("profile needs at least two points")
	}
	if p.Points[0].At != 0 {
		return errors.New("first point must be at 0s")
	}
	for i, pt := range p.Points {
		if pt.RPS < 0 {
			return fmt.Errorf("point %d: negative rate", i)
		}
		if i > 0 && pt.At < p.Points[i-1].At {
			return fmt.Errorf("point %d: offsets must not decrease", i)
		}
	}
	if p.Duration() <= 0 {
		return errors.New("profile duration must be positive")
	}
	if p.MaxWorkers < 0 {
		return errors.New("maxWorkers must not be negative")
	}
	if p.MaxRequests < 0 {
		return errors.New("maxRequests must not be negative")
	}
	if p.ThinkTime < 0 {
		return errors.New("thinkTime must not be negative")
	}
	if p.Mode != ModeRate && p.Mode != ModeUsers {
		return fmt.Errorf("unknown mode %q (want %q or %q)", p.Mode, ModeRate, ModeUsers)
	}
	return nil
}

// PlannedRequests is the number of arrivals this profile will attempt. A
// request limit caps the integral of the full shape; dropped arrivals still
// count because the engine is open-loop.
func (p Profile) PlannedRequests() int {
	// Closed loop: how many requests N users get through depends on the
	// target's latency, so there is no number to promise up front.
	if p.Users() {
		return p.MaxRequests
	}
	total := int(p.ExpectedArrivals(p.Duration()))
	if p.MaxRequests > 0 && p.MaxRequests < total {
		return p.MaxRequests
	}
	return total
}

// Duration is the total length of the run: the offset of the last point.
func (p Profile) Duration() time.Duration {
	if len(p.Points) == 0 {
		return 0
	}
	return time.Duration(p.Points[len(p.Points)-1].At)
}

// Peak is the highest target rate anywhere on the plot.
func (p Profile) Peak() float64 {
	peak := 0.0
	for _, pt := range p.Points {
		if pt.RPS > peak {
			peak = pt.RPS
		}
	}
	return peak
}

// maxWorkers resolves the effective concurrency cap.
func (p Profile) maxWorkers() int {
	if p.MaxWorkers > 0 {
		return p.MaxWorkers
	}
	return DefaultMaxWorkers
}

// TargetAt is the target rate at offset t: linear interpolation between the
// surrounding points. Before the start it is the first point's rate; at or
// past the end, the last point's. At a vertical jump the later value wins.
func (p Profile) TargetAt(t time.Duration) float64 {
	if len(p.Points) == 0 {
		return 0
	}
	if t <= time.Duration(p.Points[0].At) {
		return p.Points[0].RPS
	}
	for i := 1; i < len(p.Points); i++ {
		a, b := p.Points[i-1], p.Points[i]
		at, bt := time.Duration(a.At), time.Duration(b.At)
		if t > bt {
			continue
		}
		if t == bt {
			// Land exactly on a vertex: take the last point at this offset so
			// jumps resolve to their post-jump rate.
			for i+1 < len(p.Points) && time.Duration(p.Points[i+1].At) == bt {
				i++
			}
			return p.Points[i].RPS
		}
		if bt == at { // zero-width jump strictly before t: keep scanning
			continue
		}
		frac := float64(t-at) / float64(bt-at)
		return a.RPS + frac*(b.RPS-a.RPS)
	}
	return p.Points[len(p.Points)-1].RPS
}

// ExpectedArrivals is the number of requests the plot calls for by offset t:
// the integral of the rate curve from 0 to t (trapezoids; vertical jumps have
// zero width and add nothing). The pacer dispatches whenever the number sent
// so far falls behind this curve, which self-corrects timing drift.
func (p Profile) ExpectedArrivals(t time.Duration) float64 {
	if len(p.Points) < 2 || t <= 0 {
		return 0
	}
	if d := p.Duration(); t > d {
		t = d
	}
	total := 0.0
	for i := 1; i < len(p.Points); i++ {
		a, b := p.Points[i-1], p.Points[i]
		at, bt := time.Duration(a.At), time.Duration(b.At)
		if bt == at {
			continue
		}
		if t <= at {
			break
		}
		end, endRPS := bt, b.RPS
		if t < bt { // segment partially covered: cut it at t
			end = t
			frac := float64(t-at) / float64(bt-at)
			endRPS = a.RPS + frac*(b.RPS-a.RPS)
		}
		total += (a.RPS + endRPS) / 2 * (end - at).Seconds()
	}
	return total
}
