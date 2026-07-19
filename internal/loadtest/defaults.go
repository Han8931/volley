package loadtest

import "time"

// The built-in load shapes. They ship in-binary so a fresh install has
// something to run immediately; saving one under a new name (stage 2) is how
// users derive their own. Rates are deliberately modest defaults — shapes are
// meant to be opened and re-scaled, not fired blindly.

// Constant holds a flat rate for the whole run.
func Constant(rps float64, d time.Duration) Profile {
	return Profile{
		Name:        "constant",
		Description: "steady rate for the full duration",
		Points: []Point{
			{At: 0, RPS: rps},
			{At: Duration(d), RPS: rps},
		},
	}
}

// Ramp grows linearly from 0 to peak over d.
func Ramp(peak float64, d time.Duration) Profile {
	return Profile{
		Name:        "ramp-up",
		Description: "linear growth from zero to peak",
		Points: []Point{
			{At: 0, RPS: 0},
			{At: Duration(d), RPS: peak},
		},
	}
}

// Spike holds a baseline, jumps instantly to peak for spike, then falls back
// to the baseline for the remainder.
func Spike(baseline, peak float64, lead, spike, tail time.Duration) Profile {
	t1 := lead
	t2 := lead + spike
	end := lead + spike + tail
	return Profile{
		Name:        "spike",
		Description: "baseline, sudden burst, recovery",
		Points: []Point{
			{At: 0, RPS: baseline},
			{At: Duration(t1), RPS: baseline},
			{At: Duration(t1), RPS: peak}, // vertical jump
			{At: Duration(t2), RPS: peak},
			{At: Duration(t2), RPS: baseline}, // vertical fall
			{At: Duration(end), RPS: baseline},
		},
	}
}

// Step doubles the rate from start in equal-length plateaus.
func Step(start float64, steps int, plateau time.Duration) Profile {
	p := Profile{
		Name:        "step",
		Description: "doubling plateaus to find the breaking point",
	}
	rate := start
	at := time.Duration(0)
	p.Points = append(p.Points, Point{At: 0, RPS: rate})
	for i := 0; i < steps; i++ {
		at += plateau
		p.Points = append(p.Points, Point{At: Duration(at), RPS: rate})
		if i < steps-1 {
			rate *= 2
			p.Points = append(p.Points, Point{At: Duration(at), RPS: rate})
		}
	}
	return p
}

// Sawtooth repeats a rise-to-peak-then-drop cycle.
func Sawtooth(peak float64, cycles int, cycleLen time.Duration) Profile {
	p := Profile{
		Name:        "sawtooth",
		Description: "repeated ramp and reset",
	}
	at := time.Duration(0)
	p.Points = append(p.Points, Point{At: 0, RPS: 0})
	for i := 0; i < cycles; i++ {
		at += cycleLen
		p.Points = append(p.Points,
			Point{At: Duration(at), RPS: peak},
			Point{At: Duration(at), RPS: 0}) // instant reset
	}
	return p
}

// DefaultProfiles are the shapes offered out of the box.
func DefaultProfiles() []Profile {
	return []Profile{
		Constant(20, 30*time.Second),
		Ramp(50, time.Minute),
		Spike(5, 100, 20*time.Second, 10*time.Second, 20*time.Second),
		Step(10, 4, 15*time.Second),
		Sawtooth(40, 3, 20*time.Second),
	}
}
