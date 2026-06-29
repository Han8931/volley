package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tabularasa/volley/internal/tui/keys"
)

// Palette — kept small and named so theming is a later, central change.
var (
	colAccent = lipgloss.Color("#7D56F4") // Volley violet
	colDim    = lipgloss.Color("#6C6C6C")
	colFg     = lipgloss.Color("#E5E5E5")
	colOK     = lipgloss.Color("#34D399")
	colMethod = lipgloss.Color("#F59E0B")
)

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return "starting volley…"
	}
	l := m.computeLayout()
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewURLBar(l),
		m.viewBody(l),
		m.viewStatusBar(),
	)
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
	if m.mode != keys.Insert && m.url.Value() == "" {
		urlView = lipgloss.NewStyle().Foreground(colDim).Render(m.url.Placeholder)
	}

	inner := lipgloss.JoinHorizontal(lipgloss.Left, method, " │ ", urlView)
	return m.paneStyle(focusURL, l.urlInnerW, 1).Render(inner)
}

func (m Model) viewBody(l layout) string {
	left := m.paneStyle(focusRequest, l.reqInnerW, l.bodyInnerH).Render(
		title("REQUEST") + "\n\n" +
			dim("Headers · Body · Query") + "\n" +
			dim("(editor lands in Phase 3)"),
	)
	right := m.paneStyle(focusResponse, l.respInnerW, l.bodyInnerH).Render(m.viewResponseInner())

	return lipgloss.JoinHorizontal(lipgloss.Top,
		left, strings.Repeat(" ", l.gap), right)
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
		return renderStatusLine(m.resp) + "\n" + m.vp.View()
	}
}

func (m Model) viewStatusBar() string {
	modeTag := lipgloss.NewStyle().
		Background(colAccent).Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).Padding(0, 1).Render(m.mode.String())

	hints := " h/j/k/l move · i edit url · m method · ⏎ send · q quit"
	switch {
	case m.mode == keys.Insert:
		hints = " esc normal mode"
	case m.focus == focusResponse:
		hints = " j/k scroll · gg/G top/bottom · ^d/^u half-page · h back · ⏎ resend"
	}

	hint := lipgloss.NewStyle().Foreground(colDim).
		Width(m.width - lipgloss.Width(modeTag)).
		Render(hints)

	return lipgloss.JoinHorizontal(lipgloss.Left, modeTag, hint)
}

func title(s string) string {
	return lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render(s)
}
func dim(s string) string { return lipgloss.NewStyle().Foreground(colDim).Render(s) }
func keyHint(s string) string { return lipgloss.NewStyle().Foreground(colOK).Render(s) }
