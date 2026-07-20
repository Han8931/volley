package main

// loadtest.go — GUI bindings for shaped load testing, the same engine and
// stores the TUI drives. The front-end starts a run, then polls PollLoadTest;
// summarizing and auto-saving on completion happen exactly once, Go-side.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tabularasa/volley/internal/build"
	"github.com/tabularasa/volley/internal/httpx"
	"github.com/tabularasa/volley/internal/loadtest"
)

// loadState is the App's in-flight load test. Wails calls bindings from
// multiple goroutines, so all access goes through mu.
type loadState struct {
	mu        sync.Mutex
	run       *loadtest.Run
	profile   loadtest.Profile
	method    string
	url       string
	startedAt time.Time
	stopped   bool
	summary   *loadtest.Summary
	rendered  string
	savedAs   string
	saveErr   string
}

type PointDTO struct {
	AtMS int64   `json:"atMs"`
	RPS  float64 `json:"rps"`
}

type ProfileDTO struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Mode        string     `json:"mode"` // "" = rate (rps), "users" = closed loop
	ThinkTimeMS int64      `json:"thinkTimeMs,omitempty"`
	Points      []PointDTO `json:"points"`
	MaxRequests int        `json:"maxRequests,omitempty"`
	MaxWorkers  int        `json:"maxWorkers,omitempty"`
	// Derived, for the picker's preview and the confirm step.
	PeakRPS    float64 `json:"peakRps"`
	DurationMS int64   `json:"durationMs"`
	Planned    int     `json:"planned"`
}

func fromProfile(p loadtest.Profile) ProfileDTO {
	d := ProfileDTO{
		Name:        p.Name,
		Description: p.Description,
		Mode:        p.Mode,
		ThinkTimeMS: time.Duration(p.ThinkTime).Milliseconds(),
		Points:      []PointDTO{},
		MaxRequests: p.MaxRequests,
		MaxWorkers:  p.MaxWorkers,
		PeakRPS:     p.Peak(),
		DurationMS:  p.Duration().Milliseconds(),
		Planned:     p.PlannedRequests(),
	}
	for _, pt := range p.Points {
		d.Points = append(d.Points, PointDTO{AtMS: time.Duration(pt.At).Milliseconds(), RPS: pt.RPS})
	}
	return d
}

func toProfile(d ProfileDTO) loadtest.Profile {
	p := loadtest.Profile{
		Name:        d.Name,
		Description: d.Description,
		Mode:        d.Mode,
		ThinkTime:   loadtest.Duration(time.Duration(d.ThinkTimeMS) * time.Millisecond),
		MaxRequests: d.MaxRequests,
		MaxWorkers:  d.MaxWorkers,
	}
	for _, pt := range d.Points {
		p.Points = append(p.Points, loadtest.Point{
			At:  loadtest.Duration(time.Duration(pt.AtMS) * time.Millisecond),
			RPS: pt.RPS,
		})
	}
	return p
}

// ListProfiles seeds the built-in shapes on first use, then lists everything,
// exactly like the TUI's picker.
func (a *App) ListProfiles() ([]ProfileDTO, error) {
	if err := a.loadStore.EnsureDefaults(); err != nil {
		return nil, err
	}
	profiles, err := a.loadStore.List()
	if err != nil {
		return nil, err
	}
	out := []ProfileDTO{}
	for _, p := range profiles {
		out = append(out, fromProfile(p))
	}
	return out, nil
}

// SaveProfile validates and stores a profile shape (the GUI's :loadedit).
func (a *App) SaveProfile(name string, d ProfileDTO) error {
	return a.loadStore.Save(name, toProfile(d))
}

// DeleteProfile removes a stored profile.
func (a *App) DeleteProfile(name string) error {
	return a.loadStore.Delete(name)
}

// BucketDTO is one second of the run, for the charts.
type BucketDTO struct {
	Completed     int     `json:"completed"`
	MeanLatencyMS float64 `json:"meanLatencyMs"`
}

// LoadRunDTO is the full poll result: the live snapshot, plus — once done —
// the k6-style analysis (structured and pre-rendered) and where it was saved.
type LoadRunDTO struct {
	Running bool `json:"running"` // a run exists (live or finished-but-shown)
	Done    bool `json:"done"`

	Profile    ProfileDTO `json:"profile"`
	ElapsedMS  int64      `json:"elapsedMs"`
	Sent       int        `json:"sent"`
	Completed  int        `json:"completed"`
	OK         int        `json:"ok"`
	Errors     int        `json:"errors"`
	Canceled   int        `json:"canceled"`
	Dropped    int        `json:"dropped"`
	InFlight   int        `json:"inFlight"`
	MaxWorkers int        `json:"maxWorkers"`

	AchievedRPS  float64 `json:"achievedRps"`
	TargetNowRPS float64 `json:"targetNowRps"`

	P50MS  float64 `json:"p50Ms"`
	P90MS  float64 `json:"p90Ms"`
	P95MS  float64 `json:"p95Ms"`
	P99MS  float64 `json:"p99Ms"`
	MaxMS  float64 `json:"maxMs"`
	MeanMS float64 `json:"meanMs"`

	Buckets []BucketDTO `json:"buckets"`

	Mode        string            `json:"mode"` // "" = rate, "users" = closed loop
	Stopped     bool              `json:"stopped"`
	Summary     *loadtest.Summary `json:"summary,omitempty"`
	SummaryText string            `json:"summaryText,omitempty"`
	SavedAs     string            `json:"savedAs,omitempty"`
	SaveError   string            `json:"saveError,omitempty"`
}

// ResultDTO is one stored run summary, flattened to chartable numbers (the
// Summary's own JSON uses duration strings) plus the rendered k6-style text.
type ResultDTO struct {
	File      string  `json:"file"`
	Profile   string  `json:"profile"`
	Mode      string  `json:"mode"`
	Method    string  `json:"method"`
	URL       string  `json:"url"`
	StartedAt string  `json:"startedAt"` // RFC 3339
	ElapsedMS int64   `json:"elapsedMs"`
	Stopped   bool    `json:"stopped"`
	Planned   int     `json:"planned"`
	Sent      int     `json:"sent"`
	Completed int     `json:"completed"`
	OK        int     `json:"ok"`
	Errors    int     `json:"errors"`
	Canceled  int     `json:"canceled"`
	Dropped   int     `json:"dropped"`
	PeakRPS   float64 `json:"peakRps"`
	Achieved  float64 `json:"achievedRps"`
	ErrorRate float64 `json:"errorRate"` // percent
	P50MS     float64 `json:"p50Ms"`
	P90MS     float64 `json:"p90Ms"`
	P95MS     float64 `json:"p95Ms"`
	P99MS     float64 `json:"p99Ms"`
	MaxMS     float64 `json:"maxMs"`
	Text      string  `json:"text"`
}

// ListResults returns every saved run, newest first — the results browser's
// backing data.
func (a *App) ListResults() ([]ResultDTO, error) {
	summaries, err := a.resultStore.List()
	if err != nil {
		return nil, err
	}
	ms := func(d loadtest.Duration) float64 { return float64(time.Duration(d)) / float64(time.Millisecond) }
	out := []ResultDTO{}
	for _, s := range summaries {
		out = append(out, ResultDTO{
			File:      a.resultStore.FileName(s),
			Profile:   s.Profile,
			Mode:      s.Mode,
			Method:    s.Method,
			URL:       s.URL,
			StartedAt: s.StartedAt.Format(time.RFC3339),
			ElapsedMS: time.Duration(s.Elapsed).Milliseconds(),
			Stopped:   s.Stopped,
			Planned:   s.Planned,
			Sent:      s.Sent,
			Completed: s.Completed,
			OK:        s.OK,
			Errors:    s.Errors,
			Canceled:  s.Canceled,
			Dropped:   s.Dropped,
			PeakRPS:   s.TargetPeakRPS,
			Achieved:  s.AchievedRPS,
			ErrorRate: s.ErrorRate() * 100,
			P50MS:     ms(s.LatencyP50),
			P90MS:     ms(s.LatencyP90),
			P95MS:     ms(s.LatencyP95),
			P99MS:     ms(s.LatencyP99),
			MaxMS:     ms(s.LatencyMax),
			Text:      s.Render(),
		})
	}
	return out, nil
}

// DeleteResult removes a stored run by file name.
func (a *App) DeleteResult(file string) error {
	return a.resultStore.Delete(file)
}

// StartLoadTest builds the target request and begins a paced run. The
// front-end owns the confirm step; this refuses only impossible states.
func (a *App) StartLoadTest(profileName string, d RequestDTO) error {
	p, err := a.loadStore.Load(profileName)
	if err != nil {
		return err
	}
	built := build.Final(toRequest(d), a.resolver())
	if built.URL == "" {
		return errors.New("cannot load test: URL is empty")
	}

	a.load.mu.Lock()
	defer a.load.mu.Unlock()
	if a.load.run != nil && !a.load.run.Snapshot().Done {
		return errors.New("load test already running — stop it first")
	}
	run, err := loadtest.Runner{
		Profile: p,
		Do: func(ctx context.Context) (int, error) {
			return httpx.DoLoad(ctx, built)
		},
	}.Start(context.Background())
	if err != nil {
		return err
	}
	a.load.run = run
	a.load.profile = p
	a.load.method = built.Method
	a.load.url = built.URL
	a.load.startedAt = time.Now()
	a.load.stopped = false
	a.load.summary = nil
	a.load.rendered = ""
	a.load.savedAs = ""
	a.load.saveErr = ""
	return nil
}

// StopLoadTest cancels the in-flight run; the next poll reports it done (and
// its summary marked stopped), matching the TUI's esc.
func (a *App) StopLoadTest() {
	a.load.mu.Lock()
	defer a.load.mu.Unlock()
	if a.load.run != nil {
		a.load.stopped = true
		a.load.run.Stop()
	}
}

// DismissLoadTest drops a finished run so polls report idle again.
func (a *App) DismissLoadTest() {
	a.load.mu.Lock()
	defer a.load.mu.Unlock()
	if a.load.run != nil && a.load.run.Snapshot().Done {
		a.load.run = nil
	}
}

// PollLoadTest reports the run's current state. On the first poll after
// completion it summarizes and auto-saves the result — once.
func (a *App) PollLoadTest() LoadRunDTO {
	a.load.mu.Lock()
	defer a.load.mu.Unlock()

	if a.load.run == nil {
		return LoadRunDTO{}
	}
	snap := a.load.run.Snapshot()
	p := a.load.profile

	if snap.Done && a.load.summary == nil {
		s := loadtest.Summarize(p, a.load.method, a.load.url, a.load.startedAt, snap, a.load.stopped)
		a.load.summary = &s
		a.load.rendered = s.Render()
		if name, err := a.resultStore.Save(s); err != nil {
			a.load.saveErr = err.Error()
		} else {
			a.load.savedAs = name
		}
	}

	workers := p.MaxWorkers
	if workers <= 0 {
		workers = loadtest.DefaultMaxWorkers
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	out := LoadRunDTO{
		Running:      true,
		Done:         snap.Done,
		Profile:      fromProfile(p),
		ElapsedMS:    snap.Elapsed.Milliseconds(),
		Sent:         snap.Sent,
		Completed:    snap.Completed,
		OK:           snap.Completed - snap.Errors,
		Errors:       snap.Errors,
		Canceled:     snap.Canceled,
		Dropped:      snap.Dropped,
		InFlight:     snap.InFlight,
		MaxWorkers:   workers,
		AchievedRPS:  snap.AchievedRPS,
		TargetNowRPS: p.TargetAt(snap.Elapsed),
		P50MS:        ms(snap.P50),
		P90MS:        ms(snap.P90),
		P95MS:        ms(snap.P95),
		P99MS:        ms(snap.P99),
		MaxMS:        ms(snap.Max),
		MeanMS:       ms(snap.Mean),
		Buckets:      []BucketDTO{},
		Mode:         p.Mode,
		Stopped:      a.load.stopped,
		Summary:      a.load.summary,
		SummaryText:  a.load.rendered,
		SavedAs:      a.load.savedAs,
		SaveError:    a.load.saveErr,
	}
	for _, b := range snap.Buckets {
		out.Buckets = append(out.Buckets, BucketDTO{
			Completed:     b.Completed,
			MeanLatencyMS: ms(b.MeanLatency()),
		})
	}
	return out
}
