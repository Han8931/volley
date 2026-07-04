package tui

import "github.com/charmbracelet/lipgloss"

// helpSections is the content of the "?" overlay.
var helpSections = []struct {
	title string
	keys  [][2]string
}{
	{"Global", [][2]string{
		{"ctrl+w  h/j/k/l", "move focus between panes (Vim windows)"},
		{"ctrl+w  ↑/↓/←/→", "move focus between panes (arrows too)"},
		{"tab / shift+tab", "cycle focus (reading order)"},
		{"arrows", "move within the focused pane (like h/j/k/l)"},
		{"⏎", "send request"},
		{":", "command line"},
		{"?", "toggle this help"},
		{",n", "show / hide collections tree"},
		{"q / :q", "quit (prompts if unsaved)"},
	}},
	{"Method pane", [][2]string{
		{"j / k  ·  ↑ / ↓", "cycle the HTTP method"},
		{"tab / ^w", "reach it from the URL bar"},
	}},
	{"URL bar (types directly)", [][2]string{
		{"type", "edit the URL — no i needed"},
		{"⏎", "send request"},
		{"tab / ^w", "move to another pane"},
		{"esc", "NORMAL sub-mode (shortcuts below)"},
		{"t", "edit inline timeout (NORMAL)"},
	}},
	{"Collections / NerdTree", [][2]string{
		{"j / k  ·  gg / G", "move selection  ·  P jump to top"},
		{"o / enter / l", "open request or toggle group"},
		{"O / X", "expand / collapse group recursively"},
		{"h  ·  p", "collapse group / jump to parent"},
		{"x", "close parent group"},
		{",n  ·  R", "show / hide tree  ·  reload from disk"},
		{"m a  ·  m g", "add request into group  ·  new group"},
		{"m r  ·  m c", "rename (request or group) · copy request"},
		{"m d  ·  dd", "delete request or group (with confirm)"},
	}},
	{"Request pane", [][2]string{
		{"H / L  ·  [ / ]", "switch tab (Headers·Body·Params)"},
		{"j / k  ·  gg / G", "move rows  ·  first / last row"},
		{"h/l  0/$  b/w", "key / value cell"},
		{"i/a/enter", "edit current cell"},
		{"I / A", "edit key / value cell"},
		{"o / O", "add row below / above"},
		{"dd / dj", "delete row  ·  space toggle"},
	}},
	{"Body editor (Vim)", [][2]string{
		{"i a I A o O", "enter INSERT  ·  esc → NORMAL → leave"},
		{"x  dd  D  C  s", "delete/change  ·  r replace char"},
		{"d/c/y + w b e $ 0", "operator + motion (e.g. dw, c$, 3x)"},
		{"w b e  gg G", "word / document motions"},
		{"u  ctrl+r  ·  p P", "undo / redo  ·  paste"},
	}},
	{"Response pane", [][2]string{
		{"[ / ]", "switch Body / Headers tab"},
		{"p", "toggle raw / pretty JSON body"},
		{"j / k", "scroll  ·  gg / G top / bottom"},
		{"ctrl+d / ctrl+u", "half-page scroll"},
		{"/", "search  ·  n / N next / prev"},
		{"y", "yank body to clipboard"},
	}},
	{"Command line", [][2]string{
		{":save users/list", "save current request"},
		{":open users/list", "open saved request"},
		{":delete users/list", "delete saved request"},
		{":rename old new", "rename saved request"},
		{":copy old new", "copy saved request"},
		{":import curl …", "fill request from a pasted curl command"},
		{":copy curl", "copy current request as a curl command"},
		{":method POST", "set HTTP method"},
		{":set tok=abc", "define a {{tok}} variable"},
		{":timeout 10s", "set request timeout"},
		{":q  ·  :q!", "quit  ·  quit discarding edits"},
		{":wq  ·  :x", "save current request, then quit"},
		{":help", "help overlay"},
	}},
}

func (m Model) helpView() string {
	keyStyle := lipgloss.NewStyle().Foreground(colOK).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E5E5"))
	headStyle := lipgloss.NewStyle().Foreground(colAccent).Bold(true).MarginTop(1)

	var rows []string
	rows = append(rows, lipgloss.NewStyle().Foreground(colAccent).Bold(true).
		Render("Volley — keybindings"))

	for _, sec := range helpSections {
		rows = append(rows, headStyle.Render(sec.title))
		for _, kv := range sec.keys {
			rows = append(rows,
				"  "+keyStyle.Width(18).Render(kv[0])+descStyle.Render(kv[1]))
		}
	}
	rows = append(rows, lipgloss.NewStyle().Foreground(colDim).MarginTop(1).
		Render("press any key to close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 3).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
