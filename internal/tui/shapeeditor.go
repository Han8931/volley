package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/loadtest"
)

// This file is the dedicated shape-editing mode: an interactive load-profile
// editor living in the response pane. Points are selected and nudged with
// vim-style keys against a live chart, so building "your own test" never
// requires leaving Volley — E still drops to $EDITOR for raw-JSON precision.

// shapeRateStep / shapeRateStepBig are the j/k and J/K rate increments;
// shapeTimeStep / shapeTimeStepBig are the H/L and </> time increments.
const (
	shapeRateStep    = 1
	shapeRateStepBig = 10
	shapeTimeStep    = time.Second
	shapeTimeStepBig = 10 * time.Second
)

// openShapeEditor enters the mode on name with p's shape as the working copy.
func (m Model) openShapeEditor(name string, p loadtest.Profile) Model {
	m.loadPicker = false
	m.loadRun = nil // stale results yield the pane to the editor
	m.shapeEdit = true
	m.shapeName = name
	m.shapeBase = p
	m.shapePoints = append([]loadtest.Point(nil), p.Points...)
	m.shapeBaseline = append([]loadtest.Point(nil), p.Points...)
	m.shapeSel = 0
	m.shapeConfirmDiscard = false
	m.statusMsg = "shape edit: h/l point · j/k rate · H/L time · a/x add/del · w save · ⏎ save+run · esc exit"
	return m
}

// shapeDirty reports whether the working points differ from the loaded shape.
func (m Model) shapeDirty() bool {
	if len(m.shapePoints) != len(m.shapeBaseline) {
		return true
	}
	for i := range m.shapePoints {
		if m.shapePoints[i] != m.shapeBaseline[i] {
			return true
		}
	}
	return false
}

// shapeProfile assembles the working copy into a profile, carrying the base's
// description and worker cap through untouched.
func (m Model) shapeProfile() loadtest.Profile {
	return loadtest.Profile{
		Name:        m.shapeName,
		Description: m.shapeBase.Description,
		Points:      m.shapePoints,
		MaxWorkers:  m.shapeBase.MaxWorkers,
	}
}

// updateShapeEditor handles all keys while the mode is active.
func (m Model) updateShapeEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.shapeConfirmDiscard {
		m.shapeConfirmDiscard = false
		if msg.String() == "y" {
			m.shapeEdit = false
			m.statusMsg = "shape discarded"
			return m, nil
		}
		m.statusMsg = "kept editing — w saves"
		return m, nil
	}

	i := m.shapeSel
	pts := m.shapePoints
	switch msg.String() {
	case "l", "right":
		if i < len(pts)-1 {
			m.shapeSel++
		}
	case "h", "left":
		if i > 0 {
			m.shapeSel--
		}
	case "k", "up":
		m = m.adjustShapeRate(i, shapeRateStep)
	case "j", "down":
		m = m.adjustShapeRate(i, -shapeRateStep)
	case "K":
		m = m.adjustShapeRate(i, shapeRateStepBig)
	case "J":
		m = m.adjustShapeRate(i, -shapeRateStepBig)
	case "L":
		m = m.adjustShapeTime(i, shapeTimeStep)
	case "H":
		m = m.adjustShapeTime(i, -shapeTimeStep)
	case ">":
		m = m.adjustShapeTime(i, shapeTimeStepBig)
	case "<":
		m = m.adjustShapeTime(i, -shapeTimeStepBig)
	case "a":
		m = m.addShapePoint(i)
	case "x", "d":
		m = m.deleteShapePoint(i)
	case "w":
		return m.saveShape(false)
	case "enter":
		return m.saveShape(true)
	case "E":
		// Raw-JSON fallback: hand the working copy to $EDITOR; its result path
		// saves and reopens the picker, so leave the mode now.
		next, cmd := m.editLoadProfile(m.shapeName, m.shapeProfile())
		nm := next.(Model)
		if cmd != nil {
			nm.shapeEdit = false
		}
		return nm, cmd
	case "esc", "q":
		if m.shapeDirty() {
			m.shapeConfirmDiscard = true
			m.statusMsg = "discard unsaved shape changes? (y/n)"
			return m, nil
		}
		m.shapeEdit = false
		m.statusMsg = ""
	}
	return m, nil
}

// adjustShapeRate nudges point i's rate by delta, clamped at zero.
func (m Model) adjustShapeRate(i int, delta float64) Model {
	pts := append([]loadtest.Point(nil), m.shapePoints...)
	pts[i].RPS += delta
	if pts[i].RPS < 0 {
		pts[i].RPS = 0
	}
	m.shapePoints = pts
	return m
}

// adjustShapeTime nudges point i's offset by delta. The first point is pinned
// at 0; the others clamp between their neighbours (landing exactly on one
// makes a vertical jump). The last point may always extend the duration.
func (m Model) adjustShapeTime(i int, delta time.Duration) Model {
	if i == 0 {
		m.statusMsg = "the first point stays at 0s"
		return m
	}
	pts := append([]loadtest.Point(nil), m.shapePoints...)
	at := time.Duration(pts[i].At) + delta
	if lo := time.Duration(pts[i-1].At); at < lo {
		at = lo
	}
	if i < len(pts)-1 {
		if hi := time.Duration(pts[i+1].At); at > hi {
			at = hi
		}
	}
	pts[i].At = loadtest.Duration(at)
	m.shapePoints = pts
	return m
}

// addShapePoint inserts a point after i — midway to the next point, or 10s
// past the end when i is the last — and selects it.
func (m Model) addShapePoint(i int) Model {
	pts := append([]loadtest.Point(nil), m.shapePoints...)
	var np loadtest.Point
	if i == len(pts)-1 {
		np = loadtest.Point{At: pts[i].At + loadtest.Duration(shapeTimeStepBig), RPS: pts[i].RPS}
	} else {
		mid := (time.Duration(pts[i].At) + time.Duration(pts[i+1].At)) / 2
		np = loadtest.Point{At: loadtest.Duration(mid), RPS: (pts[i].RPS + pts[i+1].RPS) / 2}
	}
	pts = append(pts[:i+1], append([]loadtest.Point{np}, pts[i+1:]...)...)
	m.shapePoints = pts
	m.shapeSel = i + 1
	return m
}

// deleteShapePoint removes point i; the first point and a two-point minimum
// are protected so the shape stays a runnable plot.
func (m Model) deleteShapePoint(i int) Model {
	if len(m.shapePoints) <= 2 {
		m.statusMsg = "a shape needs at least two points"
		return m
	}
	if i == 0 {
		m.statusMsg = "the first point can't be deleted"
		return m
	}
	pts := append([]loadtest.Point(nil), m.shapePoints...)
	pts = append(pts[:i], pts[i+1:]...)
	m.shapePoints = pts
	if m.shapeSel >= len(pts) {
		m.shapeSel = len(pts) - 1
	}
	return m
}

// saveShape validates and persists the working copy. With run set, a
// successful save flows straight into the pre-run confirmation.
func (m Model) saveShape(run bool) (tea.Model, tea.Cmd) {
	p := m.shapeProfile()
	if err := m.loadStore.Save(m.shapeName, p); err != nil {
		m.statusMsg = "save failed: " + err.Error()
		return m, nil
	}
	m.shapeBaseline = append([]loadtest.Point(nil), m.shapePoints...)
	if run {
		m.shapeEdit = false
		return m.confirmLoadTest(p), nil
	}
	m.statusMsg = "saved load profile " + m.shapeName
	return m, nil
}

// --- rendering -------------------------------------------------------------

// shapeChartRows is the height of the editor's chart.
const shapeChartRows = 8

// viewShapeEditor renders the mode into the response pane.
func (m Model) viewShapeEditor() string {
	width := m.vp.Width
	p := m.shapeProfile()
	sel := m.shapePoints[m.shapeSel]

	dirtyMark := ""
	if m.shapeDirty() {
		dirtyMark = lipgloss.NewStyle().Foreground(colMethod).Bold(true).Render(" [+]")
	}
	head := title("SHAPE ") + lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(m.shapeName) + dirtyMark

	chartW := width - 2
	if chartW > 60 {
		chartW = 60
	}
	if chartW < 10 {
		chartW = 10
	}

	readout := fmt.Sprintf("point %d/%d · at %s · %.0f rps",
		m.shapeSel+1, len(m.shapePoints), formatRunDuration(time.Duration(sel.At)), sel.RPS)
	summary := dim(fmt.Sprintf("peak %.0f rps · %s · %d req total",
		p.Peak(), formatRunDuration(p.Duration()), int(p.ExpectedArrivals(p.Duration()))))

	lines := []string{head, ""}
	lines = append(lines, renderShapeChart(m.shapePoints, m.shapeSel, chartW, shapeChartRows)...)
	lines = append(lines,
		"",
		lipgloss.NewStyle().Foreground(colOK).Render(readout),
		summary,
		"",
		dim("h/l point · j/k rate ±1 · J/K ±10 · H/L time ±1s · </> ±10s"),
		dim("a add · x delete · w save · ⏎ save + run · E raw JSON · esc exit"),
	)
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(lines, "\n"))
}

// renderShapeChart draws the shape as filled columns with point markers: ◆ for
// points, a highlighted ● for the selected one.
func renderShapeChart(pts []loadtest.Point, sel, width, rows int) []string {
	p := loadtest.Profile{Points: pts}
	dur := p.Duration()
	max := p.Peak()

	// Per-column bar heights (in cells) sampled across the duration.
	heights := make([]int, width)
	for c := 0; c < width; c++ {
		var t time.Duration
		if width > 1 {
			t = time.Duration(float64(dur) * float64(c) / float64(width-1))
		}
		if max > 0 {
			heights[c] = int(p.TargetAt(t)/max*float64(rows) + 0.5)
		}
	}

	// Marker positions: column and row (from the bottom) per point.
	type marker struct{ col, row int }
	markers := make([]marker, len(pts))
	for i, pt := range pts {
		col := 0
		if dur > 0 {
			col = int(float64(time.Duration(pt.At)) / float64(dur) * float64(width-1))
		}
		row := 0
		if max > 0 {
			row = int(pt.RPS/max*float64(rows) + 0.5)
		}
		if row > rows-1 {
			row = rows - 1
		}
		markers[i] = marker{col: col, row: row}
	}

	barStyle := lipgloss.NewStyle().Foreground(colMarked)
	pointStyle := lipgloss.NewStyle().Foreground(colFg).Bold(true)
	selStyle := lipgloss.NewStyle().Foreground(colOK).Bold(true)

	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		fromBottom := rows - 1 - r
		var b strings.Builder
		for c := 0; c < width; c++ {
			cell, style := " ", barStyle
			if heights[c] > fromBottom {
				cell = "█"
			}
			// The selected point's marker wins the cell over other markers.
			for i := len(markers) - 1; i >= 0; i-- {
				if markers[i].col == c && markers[i].row == fromBottom {
					if i == sel {
						cell, style = "●", selStyle
						break
					}
					cell, style = "◆", pointStyle
				}
			}
			if cell == " " {
				b.WriteString(" ")
			} else {
				b.WriteString(style.Render(cell))
			}
		}
		out[r] = b.String()
	}
	return out
}
