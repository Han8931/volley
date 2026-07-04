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
	collectionInnerH int // tree pane height; spans URL + body area
	bodyInnerH       int // inner height of the request pane / whole lower area
	respInnerH       int // inner height of the response pane
	respViewportH    int // scrollable body height inside the response pane
	showTimeout      bool // whether the URL bar has room for the inline timeout readout
}

// minURLInputW is the URL input width below which the inline timeout readout is
// dropped from the top bar (it stays editable via t / :timeout).
const minURLInputW = 12

// collectionsMinWidth is the terminal width below which the collections tree is
// auto-hidden, handing its columns to the request/response panes. The user's
// show/hide preference is remembered and restored when the window widens.
const collectionsMinWidth = 60

// borderOverhead is the rendered width/height added by a bordered pane.
// Lip Gloss' Width/Height values include padding, but not borders.
const borderOverhead = 2

// methodPaneInnerW is the inner width of the method selector pane: the longest
// method label ("OPTIONS", 7) plus one cell of horizontal padding each side.
const methodPaneInnerW = 9

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
	// The top row is split into a fixed-width method selector and the URL bar.
	methodTotalW := methodPaneInnerW + borderOverhead
	urlTotalW := rightTotalW - methodTotalW - gap
	if urlTotalW < 8 {
		urlTotalW = 8
	}
	urlW := urlTotalW - borderOverhead
	// Show the inline timeout readout only when the URL bar can spare the room
	// for it and the SEND button while keeping the input usably wide.
	showTimeout := (urlW - 2 - 1 - len(sendButtonText)) - (1 + timeoutReserve) >= minURLInputW
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

	// The request and response panes are the same height now that the timeout
	// options bar is gone (timeout moved inline into the URL bar).
	respH := bodyH

	// Response pane reserves: status line (1) + tab bar (1) for the viewport.
	vpH := respH - 2
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
		respViewportH:    vpH,
		showTimeout:      showTimeout,
	}
}
