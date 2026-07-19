package loadtest

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func ramp0to100over10s() Profile {
	return Profile{
		Name: "ramp",
		Points: []Point{
			{At: 0, RPS: 0},
			{At: Duration(10 * time.Second), RPS: 100},
		},
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    Profile
		ok   bool
	}{
		{"valid ramp", ramp0to100over10s(), true},
		{"one point", Profile{Points: []Point{{At: 0, RPS: 5}}}, false},
		{"first not at zero", Profile{Points: []Point{
			{At: Duration(time.Second), RPS: 1}, {At: Duration(2 * time.Second), RPS: 1}}}, false},
		{"negative rate", Profile{Points: []Point{
			{At: 0, RPS: -1}, {At: Duration(time.Second), RPS: 1}}}, false},
		{"decreasing offsets", Profile{Points: []Point{
			{At: 0, RPS: 1}, {At: Duration(5 * time.Second), RPS: 1}, {At: Duration(2 * time.Second), RPS: 1}}}, false},
		{"zero duration", Profile{Points: []Point{{At: 0, RPS: 1}, {At: 0, RPS: 2}}}, false},
		{"vertical jump allowed", Spike(1, 10, time.Second, time.Second, time.Second), true},
	} {
		err := tc.p.Validate()
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
	for _, p := range DefaultProfiles() {
		if err := p.Validate(); err != nil {
			t.Errorf("default profile %q invalid: %v", p.Name, err)
		}
	}
}

func TestTargetAt(t *testing.T) {
	ramp := ramp0to100over10s()
	for _, tc := range []struct {
		at   time.Duration
		want float64
	}{
		{-time.Second, 0},
		{0, 0},
		{5 * time.Second, 50},
		{10 * time.Second, 100},
		{20 * time.Second, 100}, // past the end: last value
	} {
		if got := ramp.TargetAt(tc.at); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("ramp.TargetAt(%v) = %v, want %v", tc.at, got, tc.want)
		}
	}

	// A vertical jump resolves to the post-jump rate at the jump instant.
	spike := Spike(5, 100, 10*time.Second, 5*time.Second, 10*time.Second)
	if got := spike.TargetAt(10 * time.Second); got != 100 {
		t.Errorf("at jump instant: %v, want 100", got)
	}
	if got := spike.TargetAt(10*time.Second - time.Millisecond); got != 5 {
		t.Errorf("just before jump: %v, want 5", got)
	}
	if got := spike.TargetAt(15 * time.Second); got != 5 {
		t.Errorf("at fall instant: %v, want 5", got)
	}
}

func TestExpectedArrivals(t *testing.T) {
	// Constant 10 RPS: exactly rate × time.
	c := Constant(10, 10*time.Second)
	for _, tc := range []struct {
		at   time.Duration
		want float64
	}{
		{0, 0},
		{time.Second, 10},
		{10 * time.Second, 100},
		{time.Minute, 100}, // clamped at the end of the profile
	} {
		if got := c.ExpectedArrivals(tc.at); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("constant.ExpectedArrivals(%v) = %v, want %v", tc.at, got, tc.want)
		}
	}

	// Ramp 0→100 over 10s: triangle area = 500; half-time area = 125.
	ramp := ramp0to100over10s()
	if got := ramp.ExpectedArrivals(10 * time.Second); math.Abs(got-500) > 1e-9 {
		t.Errorf("ramp full = %v, want 500", got)
	}
	if got := ramp.ExpectedArrivals(5 * time.Second); math.Abs(got-125) > 1e-9 {
		t.Errorf("ramp half = %v, want 125", got)
	}

	// Spike: baseline 5×10s + peak 100×5s + baseline 5×10s = 50+500+50.
	spike := Spike(5, 100, 10*time.Second, 5*time.Second, 10*time.Second)
	if got := spike.ExpectedArrivals(25 * time.Second); math.Abs(got-600) > 1e-9 {
		t.Errorf("spike total = %v, want 600", got)
	}
}

func TestProfileJSONRoundTrip(t *testing.T) {
	orig := Spike(5, 100, 20*time.Second, 10*time.Second, 20*time.Second)
	orig.MaxWorkers = 32
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	// Durations must serialize human-readably, not as nanosecond ints.
	if want := `"at":"20s"`; !contains(string(b), want) {
		t.Errorf("JSON should contain %s, got %s", want, b)
	}
	var back Profile
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Points) != len(orig.Points) || back.Name != orig.Name || back.MaxWorkers != 32 {
		t.Fatalf("round trip mismatch: %+v vs %+v", back, orig)
	}
	for i := range back.Points {
		if back.Points[i] != orig.Points[i] {
			t.Errorf("point %d: %+v != %+v", i, back.Points[i], orig.Points[i])
		}
	}
	// Hand-written JSON with a duration string parses too.
	var hand Profile
	src := `{"name":"x","points":[{"at":"0s","rps":1},{"at":"1m30s","rps":2}]}`
	if err := json.Unmarshal([]byte(src), &hand); err != nil {
		t.Fatal(err)
	}
	if got := hand.Duration(); got != 90*time.Second {
		t.Errorf("hand-written duration = %v, want 90s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
