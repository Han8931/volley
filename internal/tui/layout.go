package tui

// layout holds the computed inner content sizes of each region for the
// current window. Kept in one place so View() and the resize handler agree.
type layout struct {
	gap              int
	methodInnerW     int // method selector pane inner width
	urlInnerW        int // url bar inner content width
	collectionInnerW int
	reqInnerW        int
	respInnerW       int
	collectionInnerH int  // tree pane height; spans URL + body area
	bodyInnerH       int  // inner height of the request pane / whole lower area
	respInnerH       int  // inner height of the response pane
	respViewportW    int  // scrollable body width inside the response pane padding
	respViewportH    int  // scrollable body height inside the response pane
	showTimeout      bool // whether the URL bar has room for the inline timeout readout
}

// minURLInputW is the URL input width below which the inline timeout readout is
// dropped from the top bar (it stays editable via t / :timeout).
const minURLInputW = 12

// collectionsMinWidth is the terminal width below which the collections tree is
// auto-hidden, handing its columns to the request/response panes. The user's
// show/hide preference is remembered and restored when the window widens.
const collectionsMinWidth = 90

// borderOverhead is the rendered width/height added by a bordered pane.
// Lip Gloss' Width/Height values include padding, but not borders.
const borderOverhead = 2

// methodPaneInnerW is the inner width of the method selector pane: the longest
// method label ("OPTIONS", 7) plus one cell of horizontal padding each side.
const methodPaneInnerW = 9

// applyLayout pushes computed sizes into child components. Keeping this in one
// place prevents border/padding accounting from drifting between Update and View.
// The bordered panes carry Padding(0,1), so children get two columns less than
// the pane's inner width — otherwise their content wraps inside the pane and
// pushes every row below it out of place.
func (m Model) applyLayout(l layout) Model {
	m.vp.Width = l.respViewportW
	m.vp.Height = l.respViewportH
	m.collectionPane.width = innerContentW(l.collectionInnerW)
	m.reqPane.setSize(innerContentW(l.reqInnerW), l.bodyInnerH)
	return m
}

// innerContentW is a bordered pane's usable content width: its inner width
// minus the one-cell horizontal padding on each side.
func innerContentW(paneInnerW int) int {
	w := paneInnerW - 2
	if w < 1 {
		return 1
	}
	return w
}

func (m Model) computeLayout() layout {
	gap := 1

	// Horizontal layout:
	//   ┌ collections ┐ ┌──── URL/method ────┐
	//   │             │ ├ request ┐ response ┤
	//   └─────────────┘ └─────────┴──────────┘
	// Width/Height below are Lip Gloss style sizes; borders add 2 cells.
	collTotalW := 0
	if m.collectionShown {
		if m.collectionWide {
			// NerdTree-style zoom: make the tree wide enough to inspect long saved
			// request names, while leaving a usable editor/response area on the right.
			collTotalW = m.width / 2
			if maxTree := m.width - 40; collTotalW > maxTree {
				collTotalW = maxTree
			}
		} else {
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
	// The top row is split into a fixed-width method selector and the URL bar.
	methodTotalW := methodPaneInnerW + borderOverhead
	urlTotalW := rightTotalW - methodTotalW - gap
	if urlTotalW < 8 {
		urlTotalW = 8
	}
	urlW := urlTotalW - borderOverhead
	// Show the inline timeout readout only when the URL bar can spare the room
	// for it and the TEST + SEND buttons while keeping the input usably wide.
	showTimeout := (urlW-2-1-len(sendButtonText)-1-len(testButtonText))-(1+timeoutReserve) >= minURLInputW
	bodyAvail := rightTotalW - gap - 2*borderOverhead
	if bodyAvail < 2 {
		bodyAvail = 2
	}
	reqW := bodyAvail / 2
	respW := bodyAvail - reqW

	// Vertical layout: the collections tree spans the full height from the top to
	// the status bar. The right-hand column leads with a blank row (aligning with
	// the tree's top border) then the tabline, the 3-row URL/method bar, and the
	// request/response panes — so bodyTopY already accounts for the tab strip.
	collectionH := m.height - 1 - borderOverhead
	if collectionH < 1 {
		collectionH = 1
	}
	bodyH := m.height - 1 - m.bodyTopY() - borderOverhead
	if bodyH < 6 {
		bodyH = 6
	}

	// The request and response panes are the same height now that the timeout
	// options bar is gone (timeout moved inline into the URL bar).
	respH := bodyH

	// Response pane reserves one header row (the tab bar, which also carries the
	// status + timing flush-right) above the viewport. Its bordered style also
	// has one column of horizontal padding on each side, so the viewport itself
	// must be two columns narrower than the pane's inner width; otherwise long
	// response lines can make the rendered box wider than the layout budget.
	vpW := respW - 2
	if vpW < 1 {
		vpW = 1
	}
	vpH := respH - 1
	if vpH < 1 {
		vpH = 1
	}

	return layout{
		gap:              gap,
		methodInnerW:     methodPaneInnerW,
		urlInnerW:        urlW,
		collectionInnerW: collW,
		reqInnerW:        reqW,
		respInnerW:       respW,
		collectionInnerH: collectionH,
		bodyInnerH:       bodyH,
		respInnerH:       respH,
		respViewportW:    vpW,
		respViewportH:    vpH,
		showTimeout:      showTimeout,
	}
}
