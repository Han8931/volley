package tui

// layout holds the computed inner content sizes of each region for the
// current window. Kept in one place so View() and the resize handler agree.
type layout struct {
	gap           int
	urlInnerW     int // url bar inner content width
	reqInnerW     int
	respInnerW    int
	bodyInnerH    int // inner height of the request/response panes
	respViewportH int // scrollable body height inside the response pane
}

// paneOverhead is border (2) + horizontal padding (2) per pane.
const paneOverhead = 4

func (m Model) computeLayout() layout {
	gap := 1

	// Two side-by-side panes plus the gap must fit the window width.
	avail := m.width - 2*paneOverhead - gap
	if avail < 2 {
		avail = 2
	}
	reqW := avail / 2
	respW := avail - reqW

	// Vertical: url bar (inner 1 + border 2 = 3) + body panes + status (1).
	bodyH := m.height - 3 - 1 - 2
	if bodyH < 3 {
		bodyH = 3
	}

	// Response pane reserves: status line (1) + blank (1) for the viewport.
	vpH := bodyH - 2
	if vpH < 1 {
		vpH = 1
	}

	return layout{
		gap:           gap,
		urlInnerW:     m.width - paneOverhead,
		reqInnerW:     reqW,
		respInnerW:    respW,
		bodyInnerH:    bodyH,
		respViewportH: vpH,
	}
}
