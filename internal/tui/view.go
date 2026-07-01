package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette — kept small and named so theming is a later, central change.
var (
	colAccent = lipgloss.Color("#7D56F4") // Volley violet
	colDim    = lipgloss.Color("#6C6C6C")
	colFg     = lipgloss.Color("#E5E5E5")
	colOK     = lipgloss.Color("#34D399")
	colMethod = lipgloss.Color("#F59E0B")
	colSel    = lipgloss.Color("#2A2440")
)

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

func (m Model) viewURLBar(l layout) string {
	method := lipgloss.NewStyle().Foreground(colMethod).Bold(true).
		Render(fmt.Sprintf(" %-6s", m.req.Method))

	urlView := m.url.View()
	if !m.url.Focused() && m.url.Value() == "" {
		urlView = lipgloss.NewStyle().Foreground(colDim).Render(m.url.Placeholder)
	}

	inner := lipgloss.JoinHorizontal(lipgloss.Left, method, " │ ", urlView)
	return m.paneStyle(focusURL, l.urlInnerW, 1).Render(inner)
}

func (m Model) viewMain(l layout) string {
	right := lipgloss.JoinVertical(lipgloss.Left,
		m.viewURLBar(l),
		m.viewBody(l),
	)
	if !m.collectionShown {
		return right
	}
	collections := m.paneStyle(focusCollection, l.collectionInnerW, l.collectionInnerH).
		Render(m.collectionPane.view())
	return lipgloss.JoinHorizontal(lipgloss.Top,
		collections, strings.Repeat(" ", l.gap), right)
}

func (m Model) viewBody(l layout) string {
	request := m.paneStyle(focusRequest, l.reqInnerW, l.bodyInnerH).Render(m.reqPane.view())
	response := m.paneStyle(focusResponse, l.respInnerW, l.bodyInnerH).Render(m.viewResponseInner())
	gap := strings.Repeat(" ", l.gap)

	return lipgloss.JoinHorizontal(lipgloss.Top, request, gap, response)
}

// viewResponseInner is the content placed inside the response pane: a status
// line on top and the scrollable body viewport below.
func (m Model) viewResponseInner() string {
	switch {
	case m.sending:
		return title("RESPONSE") + "\n\n" + m.spin.View() + dim(" sending…")
	case !m.hasResp:
		return title("RESPONSE") + "\n\n" +
			dim("Send a request with ") + keyHint("⏎") + dim(" to see the result here.")
	default:
		return renderStatusLine(m.resp) + "\n" + m.respTabBar() + "\n" + m.vp.View()
	}
}

// respTabBar renders the Body / Headers selector for the response pane.
func (m Model) respTabBar() string {
	names := []string{"Body", "Headers"}
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
	case insert:
		hints = " esc — vim normal mode in this field"
	case fieldNormal:
		hints = " vim: x dd dw cw C w b u p · esc — leave field"
	case m.pendingWindow:
		hints = " window: h/j/k/l pick a pane"
	case m.focus == focusURL:
		hints = " h/l method · [/] request tabs · j move · i edit URL · ⏎ send · ? help"
	case m.focus == focusCollection:
		hints = " tree: j/k move · o/l open/toggle · O/X expand/collapse all · p parent · m menu · dd del · R reload"
	case m.focus == focusRequest && m.reqPane.tab == tabBody:
		hints = " [/] tab · i edit body (Vim) · ^w/tab switch panes · ? help"
	case m.focus == focusRequest:
		hints = " [/] tab · j/k row · h/l cell · i edit · o add · dd del · ^w/tab panes · ? help"
	case m.focus == focusResponse:
		hints = " [/] body·headers · j/k scroll · / search · y yank · ^w/tab panes · ? help"
	}

	hintStyle := lipgloss.NewStyle().Foreground(colDim)
	if m.statusMsg != "" {
		hintStyle = hintStyle.Foreground(colOK)
	}
	hintW := m.width - lipgloss.Width(modeTag)
	if hintW < 0 {
		hintW = 0
	}
	hint := hintStyle.Width(hintW).Render(hints)

	return lipgloss.JoinHorizontal(lipgloss.Left, modeTag, hint)
}

func title(s string) string {
	return lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render(s)
}
func dim(s string) string     { return lipgloss.NewStyle().Foreground(colDim).Render(s) }
func keyHint(s string) string { return lipgloss.NewStyle().Foreground(colOK).Render(s) }
