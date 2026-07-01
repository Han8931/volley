package tui

// layout holds the computed inner content sizes of each region for the
// current window. Kept in one place so View() and the resize handler agree.
type layout struct {
	gap              int
	urlInnerW        int // url bar inner content width
	collectionInnerW int
	reqInnerW        int
	respInnerW       int
	collectionInnerH int // tree pane height; spans URL + body area
	bodyInnerH       int // inner height of the request pane / whole lower area
	respInnerH       int // inner height of the response pane, after options bar
	respViewportH    int // scrollable body height inside the response pane
}

// borderOverhead is the rendered width/height added by a bordered pane.
// Lip Gloss' Width/Height values include padding, but not borders.
const borderOverhead = 2

func (m Model) computeLayout() layout {
	gap := 1

	// Horizontal layout:
	//   ┌ collections ┐ ┌──── URL/method ────┐
	//   │             │ ├ request ┐ response ┤
	//   └─────────────┘ └─────────┴──────────┘
	// Width/Height below are Lip Gloss style sizes; borders add 2 cells.
	collTotalW := 0
	if m.collectionShown {
		collTotalW = m.width / 5
		if collTotalW < 22 && m.width >= 90 {
			collTotalW = 22
		}
		if collTotalW < 14 {
			collTotalW = 14
		}
		if collTotalW > m.width-20 {
			collTotalW = m.width / 3
		}
	}
	collW := collTotalW - borderOverhead
	if collW < 0 {
		collW = 0
	}

	rightGap := 0
	if m.collectionShown {
		rightGap = gap
	}
	rightTotalW := m.width - collTotalW - rightGap
	if rightTotalW < 10 {
		rightTotalW = 10
	}
	urlW := rightTotalW - borderOverhead
	bodyAvail := rightTotalW - gap - 2*borderOverhead
	if bodyAvail < 2 {
		bodyAvail = 2
	}
	reqW := bodyAvail / 2
	respW := bodyAvail - reqW

	// Vertical layout: collections spans from the top to the status bar.
	// Right side uses a 3-row URL/method bar above request/response.
	collectionH := m.height - 1 - borderOverhead
	if collectionH < 1 {
		collectionH = 1
	}
	bodyH := m.height - 1 - 3 - borderOverhead
	if bodyH < 6 {
		bodyH = 6
	}

	// The response column has a 3-row bordered options bar above the response,
	// reducing only the response pane, not the URL bar or request pane.
	respH := bodyH - 3
	if respH < 3 {
		respH = 3
	}

	// Response pane reserves: status line (1) + tab bar (1) for the viewport.
	vpH := respH - 2
	if vpH < 1 {
		vpH = 1
	}

	return layout{
		gap:              gap,
		urlInnerW:        urlW,
		collectionInnerW: collW,
		reqInnerW:        reqW,
		respInnerW:       respW,
		collectionInnerH: collectionH,
		bodyInnerH:       bodyH,
		respInnerH:       respH,
		respViewportH:    vpH,
	}
}
