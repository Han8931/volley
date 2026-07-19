package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette — kept small and named so theming is a later, central change. Every
// color is adaptive so the UI stays legible on light terminals too.
var (
	colAccent = lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#7D56F4"} // Volley violet
	colDim    = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#6C6C6C"}
	colFg     = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E5E5E5"}
	colOK     = lipgloss.AdaptiveColor{Light: "#059669", Dark: "#34D399"}
	colMethod = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#F59E0B"}
	colSel    = lipgloss.AdaptiveColor{Light: "#E9E3F8", Dark: "#2A2440"}
	colMarked = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#4B5563"} // multi-selected tree rows (ranger/lf-style block)
	// colSelFg pairs with the colSel / colMarked backgrounds (white would vanish
	// on their light-theme fills).
	colSelFg = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#FFFFFF"}
)

const sendButtonText = " SEND "

// testButtonText labels the clickable load-test launcher beside SEND.
const testButtonText = " TEST "

// copyButtonText labels the clickable copy affordance in the response header.
const copyButtonText = " ⧉ copy "

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return "starting volley…"
	}
	if m.showHelp {
		return m.helpView()
	}
	l := m.computeLayout()
	bottom := m.viewStatusBar()
	if m.cmdActive {
		bottom = m.viewCommandLine()
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewMain(l),
		bottom,
	)
}

// viewCommandLine renders the active ":" or "/" input across the bottom row.
func (m Model) viewCommandLine() string {
	prefix := lipgloss.NewStyle().Foreground(colAccent).Bold(true).
		Render(string(m.cmdKind))
	line := prefix + m.cmd.View()
	if ghost := m.commandGhost(); ghost != "" {
		line += lipgloss.NewStyle().Foreground(colDim).Italic(true).Render(ghost)
	}
	return lipgloss.NewStyle().Width(m.width).Render(line)
}

// paneStyle returns a bordered box, highlighted when focused.
func (m Model) paneStyle(f focus, w, h int) lipgloss.Style {
	border := colDim
	if m.focus == f {
		border = colAccent
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Width(w).
		Height(h).
		Padding(0, 1)
}

// renderURLField renders the URL bar's contents. When the bar is focused it
// draws a reverse-video block cursor at the buffer's cursor column (in both
// Insert and NORMAL modes), scrolling horizontally so the cursor stays visible;
// otherwise it shows the text, or a dim placeholder when empty. Text clipped
// off either edge is flagged with ‹ / › (focused) or a trailing … (unfocused),
// so a long URL never looks complete when it isn't. Markers replace an edge
// cell rather than adding one, so the field never exceeds its width.
func (m Model) renderURLField(width int) string {
	runes := []rune(m.url.Text())
	if m.focus != focusURL {
		if len(runes) == 0 {
			return lipgloss.NewStyle().Foreground(colDim).Render(truncateRunes(urlPlaceholder, width))
		}
		if width > 0 && len(runes) > width {
			// First width-1 runes plus a … marker for the dropped tail.
			return truncateRunes(m.url.Text(), width-1) + "…"
		}
		return m.url.Text()
	}

	_, col := m.url.Cursor()
	start := 0
	if width > 0 && col >= width {
		start = col - width + 1
	}
	end := len(runes)
	if width > 0 && end > start+width {
		end = start + width
	}

	window := append([]rune(nil), runes[start:end]...)
	cursorAt := col - start
	// Overlay edge markers for hidden text, but never clobber the cursor cell —
	// the block cursor itself already signals that edge.
	if start > 0 && len(window) > 0 && cursorAt != 0 {
		window[0] = '‹'
	}
	if end < len(runes) && len(window) > 0 && cursorAt != len(window)-1 {
		window[len(window)-1] = '›'
	}
	return renderCursorLine(string(window), cursorAt)
}

// viewMethodPane renders the standalone HTTP-method selector to the left of the
// URL bar. It cycles with r when focused.
func (m Model) viewMethodPane(l layout) string {
	method := fmt.Sprintf("%-7s", m.req.Method)
	label := lipgloss.NewStyle().Foreground(colMethod).Bold(true).Render(method)
	if m.focusHints {
		label = focusHintBadge("2") + " " + lipgloss.NewStyle().Foreground(colMethod).Bold(true).Render(strings.TrimSpace(m.req.Method))
	}
	return m.paneStyle(focusMethod, l.methodInnerW, 1).Render(label)
}

func (m Model) viewURLBar(l layout) string {
	urlW := urlInputWidth(l)
	urlView := m.renderURLField(urlW)

	// The right edge holds the SEND button then TEST at the far edge (primary
	// action first), preceded by the inline timeout readout when the bar has
	// room (or is actively being edited).
	right := m.sendButtonView() + " " + m.testButtonView()
	if l.showTimeout || m.timeoutInput.Focused() {
		right = m.timeoutSegView() + " " + right
	}
	if m.focusHints {
		urlView = focusHintBadge("3") + " " + urlView
	}
	space := urlContentWidth(l) - lipgloss.Width(urlView) - lipgloss.Width(right)
	if space < 1 {
		space = 1
	}
	inner := lipgloss.JoinHorizontal(lipgloss.Left, urlView, strings.Repeat(" ", space), right)
	return m.paneStyle(focusURL, l.urlInnerW, 1).Render(inner)
}

const (
	// timeoutValueW bounds the rendered width of the timeout value (both the
	// readout and the inline editor); timeoutReserve is the URL bar's budget for
	// the whole "timeout <value>" segment. Kept in sync with the timeout input's
	// CharLimit/Width in New().
	timeoutValueW  = 7
	timeoutReserve = len("timeout ") + timeoutValueW + 1 // label + value + margin
)

// timeoutSegView renders the inline timeout readout carried on the right of the
// URL bar. When the field is being edited (via t or :timeout) it shows the live
// input; otherwise the effective value, or the engine default when unset.
func (m Model) timeoutSegView() string {
	label := lipgloss.NewStyle().Foreground(colDim).Render("timeout ")
	if m.timeoutInput.Focused() {
		return label + m.timeoutInput.View()
	}
	val := formatTimeout(m.timeout)
	if m.timeout <= 0 {
		val = m.timeoutInput.Placeholder // engine default
	}
	return label + lipgloss.NewStyle().Foreground(colMethod).Render(truncateRunes(val, timeoutValueW))
}

func urlContentWidth(l layout) int {
	w := l.urlInnerW - 2 // account for horizontal pane padding
	if w < 1 {
		return 1
	}
	return w
}

func urlInputWidth(l layout) int {
	// The URL bar holds the input and the TEST + SEND buttons, plus the inline
	// timeout readout when there's room — each trailing item has a one-cell gap.
	w := urlContentWidth(l) - 1 - len(sendButtonText) - 1 - len(testButtonText)
	if l.showTimeout {
		w -= 1 + timeoutReserve
	}
	if w < 1 {
		return 1
	}
	return w
}

func (m Model) sendButtonView() string {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(colOK).Bold(true)
	if m.sending || m.loadRunning() || strings.TrimSpace(m.url.Text()) == "" {
		st = st.Foreground(colFg).Background(colDim)
	}
	return st.Render(sendButtonText)
}

// testButtonView renders the load-test launcher: amber so it reads as "hotter"
// than a single SEND, dimmed whenever a run can't start right now.
func (m Model) testButtonView() string {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(colMethod).Bold(true)
	if m.sending || m.loadRunning() || strings.TrimSpace(m.url.Text()) == "" {
		st = st.Foreground(colFg).Background(colDim)
	}
	return st.Render(testButtonText)
}

// copyButtonView renders the clickable "copy" pill shown on the right of the
// response header; copyButtonClicked hit-tests against the same rendered width.
func (m Model) copyButtonView() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).
		Background(colAccent).Bold(true).Render(copyButtonText)
}

func (m Model) viewMain(l layout) string {
	topBar := lipgloss.JoinHorizontal(lipgloss.Top,
		m.viewMethodPane(l),
		strings.Repeat(" ", l.gap),
		m.viewURLBar(l),
	)
	// When the tree is shown, lead the right column with a blank row matching the
	// tree's top border, so the tabline lines up with the tree's first content
	// row (COLLECTIONS) and the tree border stays the topmost element.
	rightParts := make([]string, 0, 4)
	if m.collectionShown {
		rightParts = append(rightParts, "")
	}
	rightParts = append(rightParts, m.viewOpenTabs(l), topBar, m.viewBody(l))
	right := lipgloss.JoinVertical(lipgloss.Left, rightParts...)
	if !m.collectionShown {
		return right
	}
	collections := m.paneStyle(focusCollection, l.collectionInnerW, l.collectionInnerH).
		Render(m.collectionPane.viewWithTitle(m.focusHintTitle(focusCollection, "COLLECTIONS")))
	return lipgloss.JoinHorizontal(lipgloss.Top,
		collections, strings.Repeat(" ", l.gap), right)
}

const (
	openTabLabelMaxW = 18
	// openTabGap is the blank cells rendered between adjacent tabs. openTabHit
	// (mouse hit-testing) advances by the same amount, so the two must agree.
	openTabGap = 1
	// openTabCloseGlyph is the per-tab close button. It occupies the two trailing
	// cells of a tab (the glyph plus one pad), which openTabHit treats as the
	// close hot-zone; clicking anywhere else on the tab switches to it.
	openTabCloseGlyph = "✕"
	openTabCloseZone  = 2
	// openTabDirtyGlyph marks a tab whose buffer has unsaved edits.
	openTabDirtyGlyph = "●"
)

// openTabLabel is the on-screen text of a single tab: the (truncated) name,
// prefixed with a dot when the tab has unsaved edits, plus a trailing close
// button, padded one cell each side. Shared by the renderer and the click
// hit-tester so their widths never drift.
func openTabLabel(name string, dirty bool) string {
	display := name
	if display == "" {
		display = "unsaved"
	}
	display = truncateMiddle(display, openTabLabelMaxW)
	if dirty {
		display = openTabDirtyGlyph + display
	}
	return " " + display + " " + openTabCloseGlyph + " "
}

// tabLabels renders every open tab's label (with its live dirty state), for the
// renderer and the click hit-tester to share so their widths never drift.
func (m Model) tabLabels() []string {
	labels := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		labels[i] = openTabLabel(t.name, m.tabDirty(i))
	}
	return labels
}

// tablineWidth is the total width of the tab strip: the right-hand column's
// full span (method pane + gap + URL bar). Shared by the renderer and the
// click hit-tester.
func tablineWidth(l layout) int {
	w := l.methodInnerW + borderOverhead + l.gap + l.urlInnerW + borderOverhead
	if w < 1 {
		return 1
	}
	return w
}

// tabStripFirst returns the index of the first tab drawn in the strip: 0 while
// everything fits, otherwise the smallest index that keeps the active tab's
// right edge inside width — so the active tab can never scroll out of view.
func tabStripFirst(labels []string, active, width int) int {
	first := 0
	for first < active {
		end := 0
		for i := first; i <= active; i++ {
			if i > first {
				end += openTabGap
			}
			end += lipgloss.Width(labels[i])
		}
		if end <= width {
			break
		}
		first++
	}
	return first
}

// viewOpenTabs renders the tree-opened request tabs as a tabline at the top of
// the right-hand column, beside the tree — like Bruno/Postman's tab strip. The
// bar has a solid fill so it reads as a distinct strip; the active tab is a
// bright accent block and inactive tabs are lighter gray blocks separated by a
// gap, so every tab is legible even on a black terminal. The strip is always
// present (a dim hint when no tabs are open) so the layout never shifts. When
// the tabs overflow, the strip scrolls so the active tab stays visible (the
// [n/N] counter in the status bar signals the clipped rest).
func (m Model) viewOpenTabs(l layout) string {
	width := tablineWidth(l)
	fill := lipgloss.NewStyle().Background(colSel)

	if len(m.tabs) == 0 {
		hint := " no open tabs — mark requests in the tree, then press T "
		return fill.Foreground(colDim).Width(width).MaxWidth(width).Render(hint)
	}

	active := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(colAccent).Bold(true)
	idle := lipgloss.NewStyle().Foreground(colFg).Background(colMarked)
	sep := fill.Render(strings.Repeat(" ", openTabGap))

	labels := m.tabLabels()
	first := tabStripFirst(labels, m.activeTab, width)

	var b strings.Builder
	used := 0
	for i := first; i < len(labels); i++ {
		if i > first {
			b.WriteString(sep)
			used += openTabGap
		}
		st := idle
		if i == m.activeTab {
			st = active
		}
		b.WriteString(st.Render(labels[i]))
		used += lipgloss.Width(labels[i])
	}
	if used < width { // pad the rest of the strip so the bar spans the full width
		b.WriteString(fill.Render(strings.Repeat(" ", width-used)))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(b.String())
}

func (m Model) viewBody(l layout) string {
	reqHint := ""
	if m.focusHints {
		reqHint = m.focusHintKey(focusRequest)
	}
	request := m.paneStyle(focusRequest, l.reqInnerW, l.bodyInnerH).Render(m.reqPane.view(reqHint))
	response := m.paneStyle(focusResponse, l.respInnerW, l.respInnerH).Render(m.viewResponseInner())
	gap := strings.Repeat(" ", l.gap)

	return lipgloss.JoinHorizontal(lipgloss.Top, request, gap, response)
}

// viewResponseInner is the content placed inside the response pane: a header
// row (Body/Headers tabs on the left, status + timing flush-right) above the
// scrollable body viewport.
func (m Model) viewResponseInner() string {
	switch {
	case m.shapeEdit:
		return m.viewShapeEditor()
	case m.loadPicker:
		return m.viewLoadPicker()
	case m.loadRun != nil:
		return m.viewLoadRun()
	case m.sending && !m.hasResp:
		// Nothing to keep on screen yet — show the spinner centered in the pane.
		return m.focusHintTitle(focusResponse, "RESPONSE") + "\n\n" + m.spin.View() + dim(" sending…")
	case !m.hasResp:
		return m.focusHintTitle(focusResponse, "RESPONSE") + "\n\n" +
			dim("Send a request with ") + keyHint("⏎") + dim(" to see the result here.")
	default:
		// A completed response, or a resend in flight: keep the previous body
		// visible and let the header carry the status (or the spinner while
		// sending). The viewport width already accounts for the pane's horizontal
		// Padding(0,1), so target it for a header flush with the body's edge.
		return m.respHeaderBar(m.vp.Width) + "\n" + m.vp.View()
	}
}

// respHeaderTabs is the header's left side: the Body/Headers selector, plus
// the numbered focus badge while ,g hints are up. Shared by the renderer and
// the copy-button visibility/hit-test logic so their widths agree.
func (m Model) respHeaderTabs() string {
	tabs := m.respTabBar()
	if m.focusHints {
		tabs = focusHintBadge(m.focusHintKey(focusResponse)) + " " + tabs
	}
	return tabs
}

// copyButtonShown reports whether the header has room for the copy pill beside
// the status summary. When the pane is too narrow for both, the status wins —
// it is the response's most important datum, and y still yanks.
func (m Model) copyButtonShown(width int) bool {
	if !m.hasResp || m.sending {
		return false
	}
	budget := width - lipgloss.Width(m.respHeaderTabs()) - lipgloss.Width(m.copyButtonView()) - 2
	return renderStatusSummary(m.resp, budget) != ""
}

// respHeaderBar lays the Body/Headers tabs against the left edge with the
// response status + timing pushed to the right, filling width. On a narrow
// pane the summary drops segments (then the copy button) rather than clipping
// mid-word, keeping the header to a single truthful row.
func (m Model) respHeaderBar(width int) string {
	tabs := m.respHeaderTabs()
	// The right side shows the spinner while a request is in flight, otherwise
	// the response status + timing followed by the clickable copy button.
	// Reserve at least one column of separation.
	var right string
	switch {
	case m.sending:
		right = m.spin.View() + dim(" sending…")
	case m.copyButtonShown(width):
		copyBtn := m.copyButtonView()
		reserved := lipgloss.Width(tabs) + lipgloss.Width(copyBtn) + 2
		right = renderStatusSummary(m.resp, width-reserved) + " " + copyBtn
	default:
		right = renderStatusSummary(m.resp, width-lipgloss.Width(tabs)-1)
	}
	gap := width - lipgloss.Width(tabs) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := tabs + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// respTabBar renders the Body / Headers selector for the response pane. The
// Body tab shows the active raw/pretty mode when the payload is JSON (i.e. when
// the p toggle is meaningful).
func (m Model) respTabBar() string {
	bodyName := "Body"
	if m.hasResp && m.respTab == 0 {
		if _, ok := prettyJSON(m.resp.Body); ok {
			if m.rawBody {
				bodyName = "Body · raw"
			} else {
				bodyName = "Body · pretty"
			}
		}
	}
	names := []string{bodyName, "Headers"}
	cells := make([]string, len(names))
	for i, n := range names {
		st := lipgloss.NewStyle().Padding(0, 1)
		if i == m.respTab {
			st = st.Foreground(lipgloss.Color("#FFFFFF")).Background(colAccent).Bold(true)
		} else {
			st = st.Foreground(colDim)
		}
		cells[i] = st.Render(n)
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, cells...)
}

func (m Model) viewStatusBar() string {
	editing := m.editing()
	insert := m.inInsert()
	fieldNormal := editing && !insert // captured field in Vim-normal mode
	label := "NORMAL"
	tagBG := colAccent
	if insert {
		label, tagBG = "INSERT", colOK
	}
	modeTag := lipgloss.NewStyle().
		Background(tagBG).Foreground(lipgloss.Color("#000000")).
		Bold(true).Padding(0, 1).Render(label)

	var hints string
	switch {
	case m.statusMsg != "":
		hints = " " + m.statusMsg
	case m.timeoutInput.Focused():
		hints = " timeout: e.g. 500ms, 10s, 2m · ⏎/esc apply · empty = default"
	case insert && m.focus == focusURL:
		hints = " type the URL · ⏎ send · tab/^w move panes · esc — NORMAL shortcuts"
	case insert:
		hints = " esc — vim normal mode in this field"
	case fieldNormal:
		hints = " vim: x dd dw cw C w b u p · esc — leave field"
	case m.pendingWindow:
		hints = " window: h/j/k/l pick a pane"
	case m.focusHints:
		hints = " jump: 1 tree · 2 method · 3 url · 4 request · 5 response · esc cancel"
	case m.shapeEdit:
		hints = " shape editor · controls shown below the chart"
	case m.loadRunning():
		hints = " load test running · esc stop"
	case m.loadRun != nil && m.focus == focusResponse:
		hints = " esc close results · T run again"
	case m.focus == focusURL:
		hints = " i edit URL · ,t timeout · ⏎ send · tab / ^w move panes · ? help"
	case m.focus == focusMethod:
		hints = " r/R change method · ⏎ send · tab / ^w move panes · ? help"
	case m.focus == focusCollection:
		hints = " tree: j/k move · o/l open/toggle · O/X expand/collapse all · p parent · m menu · dd del · R reload"
	case m.focus == focusRequest && m.reqPane.tab == tabBody:
		hints = " [/] tab · i edit body (Vim) · ^w/tab switch panes · ? help"
	case m.focus == focusRequest:
		hints = " [/] tab · j/k row · h/l cell · i edit · o add · dd del · ^w/tab panes · ? help"
	case m.focus == focusResponse:
		hints = " [/] body·headers · p raw/pretty · j/k scroll · / search · y yank · ^w panes · ?"
	}

	nameSeg := m.docNameSeg()

	hintStyle := lipgloss.NewStyle().Foreground(colDim)
	if m.statusMsg != "" {
		hintStyle = hintStyle.Foreground(colOK)
	}
	hintW := m.width - lipgloss.Width(modeTag) - lipgloss.Width(nameSeg)
	if hintW < 0 {
		hintW = 0
	}
	hint := hintStyle.Width(hintW).Render(truncateRunes(hints, hintW))

	bar := lipgloss.JoinHorizontal(lipgloss.Left, modeTag, nameSeg, hint)
	// Clamp so a long name on a very narrow terminal can't overflow the row.
	return lipgloss.NewStyle().MaxWidth(m.width).Render(bar)
}

// docNameSeg renders the vim-style document label for the status bar: the saved
// request name (or [No Name]) plus a [+] marker when there are unsaved edits.
// docNameMaxW caps the request name shown in the status bar so a long path
// can't crowd out the keybinding hints.
const docNameMaxW = 28

func (m Model) docNameSeg() string {
	name := m.currentName
	nameStyle := lipgloss.NewStyle().Foreground(colFg)
	if name == "" {
		name, nameStyle = "[No Name]", lipgloss.NewStyle().Foreground(colDim)
	}
	seg := nameStyle.Render(" " + truncateMiddle(name, docNameMaxW))
	if len(m.tabs) > 1 { // a lone tab needs no position counter
		seg += lipgloss.NewStyle().Foreground(colAccent).Render(fmt.Sprintf(" [%d/%d]", m.activeTab+1, len(m.tabs)))
	}
	if m.dirty() {
		seg += lipgloss.NewStyle().Foreground(colMethod).Bold(true).Render(" [+]")
	}
	return seg + "  "
}

// truncateMiddle shortens s to at most max runes, keeping the head and tail
// around a central ellipsis so both the leading group and the leaf name stay
// legible (e.g. "auth/very/long/…/login").
func truncateMiddle(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	keep := max - 1 // room for the ellipsis
	head := keep / 2
	tail := keep - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

func (m Model) focusHintTitle(f focus, label string) string {
	if !m.focusHints {
		return title(label)
	}
	return focusHintBadge(m.focusHintKey(f)) + " " + title(label)
}

func (m Model) focusHintKey(f focus) string {
	switch f {
	case focusCollection:
		return "1"
	case focusMethod:
		return "2"
	case focusURL:
		return "3"
	case focusRequest:
		return "4"
	case focusResponse:
		return "5"
	default:
		return ""
	}
}

func title(s string) string {
	return lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render(s)
}
func dim(s string) string     { return lipgloss.NewStyle().Foreground(colDim).Render(s) }
func keyHint(s string) string { return lipgloss.NewStyle().Foreground(colOK).Render(s) }

func focusHintBadge(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(colOK).Bold(true).Padding(0, 1).Render(s)
}
