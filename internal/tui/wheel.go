package tui

import "time"

// wheelArrowWindow is how long after a wheel notch we treat vertical navigation
// keys as terminal-injected (alternate-scroll) rather than real presses. Some
// terminals emit the synthetic keys just BEFORE the wheel mouse event, so a
// notch's keys are only covered by the PREVIOUS notch's window — the window
// therefore has to span the gap between notches during a continuous scroll.
// It stays well under human reaction time, so intentional keys a beat after
// scrolling still register.
const wheelArrowWindow = 250 * time.Millisecond

// wheelSuppressor swallows terminal-injected vertical navigation keys that can
// accompany mouse-wheel events in some terminals. It is time-window based rather
// than count based because terminals disagree on how many keys one notch emits.
type wheelSuppressor struct {
	active  bool
	armedAt time.Time
}

func (w *wheelSuppressor) Arm() {
	w.active = true
	w.armedAt = time.Now()
}

func (w *wheelSuppressor) ShouldSuppress(key string) bool {
	if !w.active {
		return false
	}
	if time.Since(w.armedAt) > wheelArrowWindow {
		w.active = false
		return false
	}
	// Most terminals send arrows (wheel down -> Down), but some send mixed
	// vertical arrows during high-resolution/touchpad scrolling. A few send
	// normal-mode-equivalent j/k bytes. Suppress vertical navigation while the
	// wheel burst is active.
	if key == "up" || key == "down" || key == "j" || key == "k" {
		return true
	}
	// A different key within the window is a real, intentional press: stop
	// suppressing so it and later navigation keys aren't eaten.
	w.active = false
	return false
}
