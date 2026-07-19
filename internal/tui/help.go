package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

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
		{"⏎ / :send", "send the request (⏎ from the Method/URL panes)"},
		{":", "command line"},
		{"?", "toggle this help"},
		{",n  ·  ,g", "show / hide collections tree  ·  numbered pane jump"},
		{"q / :q", "quit (prompts if unsaved)"},
	}},
	{"Tabs (open saved requests)", [][2]string{
		{"T (in tree)", "open marked requests (or the one under the cursor) as tabs — adds to the open set"},
		{"H / L  ·  click", "switch tabs — each keeps its own edits (● marks unsaved)"},
		{"click ✕  ·  ctrl+w q", "close a tab with the mouse  ·  close the active tab"},
		{":tabnew <name>  ·  :tabonly", "open a saved request in a tab  ·  close all others"},
	}},
	{"Method pane", [][2]string{
		{"r / R", "cycle the HTTP method forward / back"},
		{"⏎", "send the request"},
		{"tab / ^w", "reach it from the URL bar"},
	}},
	{"URL bar", [][2]string{
		{"i / a  ·  click", "edit the URL"},
		{"⏎", "send the request (INSERT or NORMAL)"},
		{"tab / ^w", "move to another pane"},
		{"NORMAL", "Vim edits (x w b C dd p u …)"},
		{",t", "edit inline timeout (leader)"},
	}},
	{"Collections / NerdTree", [][2]string{
		{"j / k  ·  gg / G", "move selection  ·  P jump to top"},
		{"o / enter / l", "open request or toggle group"},
		{"O / X  ·  A", "expand/collapse recursively  ·  widen tree"},
		{"space", "mark/unmark request, then move down"},
		{"T", "open marked requests as tabs"},
		{"h  ·  p", "collapse group / jump to parent"},
		{"x", "close parent group"},
		{",n  ·  R", "show / hide tree  ·  reload from disk"},
		{"m a  ·  m g", "add request into group  ·  new group"},
		{"m r  ·  m c", "rename (request or group) · copy request"},
		{"m d  ·  dd", "delete request or group (with confirm)"},
	}},
	{"Request pane", [][2]string{
		{"[ / ]", "switch tab (Headers·Body·Params·Auth)"},
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
		{"[ ]", "switch Body / Headers tab"},
		{"p", "toggle raw / pretty JSON body"},
		{"j / k", "scroll  ·  gg / G top / bottom"},
		{"ctrl+d / ctrl+u", "half-page scroll"},
		{"/", "search  ·  n / N next / prev"},
		{"y  ·  ⧉ copy", "yank body to clipboard (key or click the header button)"},
	}},
	{"Load testing", [][2]string{
		{"TEST  ·  :loadtest", "pick a load profile (shape preview), confirm, run"},
		{":loadtest <name>", "run a named profile against the current request"},
		{":loadnew <name>", "create your own shape (optionally from a template profile)"},
		{":loadedit <name>", "reshape a saved profile in the shape editor"},
		{":loadeditor <name>", "edit a profile's raw JSON in $VISUAL / $EDITOR"},
		{"e / E / n (in picker)", "edit shape · edit JSON · start a new profile"},
		{"esc", "stop a running test / close finished results"},
		{"T (in results)", "run the same profile again"},
		{"y · ⧉ copy · drag", "copy the analysis (key or button) · drag-select any run text"},
		{"results", "finished runs print a k6-style analysis, auto-saved to loadresults/"},
		{"profiles", "JSON files in config dir loadprofiles/ — edit or add your own"},
	}},
	{"Shape editor (:loadnew / :loadedit)", [][2]string{
		{"h / l", "select previous / next point on the plot"},
		{"j / k  ·  J / K", "rate −/+1 · −/+10 rps"},
		{"[ / ] · H / L · < / >", "time −/+100ms · −/+1s · −/+10s"},
		{"- / +", "decrease / increase the request limit by one"},
		{"C / c", "decrease / increase the worker cap by one"},
		{"a  ·  x", "add a point after the selection · delete it"},
		{"w  ·  ⏎", "save · save and run it"},
		{"E", "open the raw JSON in $EDITOR instead"},
		{"esc", "leave (asks before discarding unsaved changes)"},
	}},
	{"Command line", [][2]string{
		{"↑ / ↓", "previous / next command (restores the current draft)"},
		{"Tab", "complete commands, saved requests, groups, profiles, methods"},
		{":save users/list", "save current request"},
		{":open users/list", "open saved request"},
		{":delete users/list", "delete saved request"},
		{":rename old new", "rename saved request"},
		{":copy old new", "copy saved request"},
		{":import curl …", "fill request from a pasted curl command"},
		{":copy curl", "copy current request as a curl command"},
		{":editor [name]", "edit current or named request in $EDITOR"},
		{":method POST", "set HTTP method"},
		{":set tok=abc", "define a {{tok}} variable (bare :set lists names)"},
		{":env [name]", "switch environment · bare :env lists · off deactivates"},
		{":envnew · :envedit · :envrm", "create, edit ($EDITOR), delete an environment"},
		{":timeout 10s", "set request timeout"},
		{":q  ·  :q!", "quit  ·  quit discarding edits (:qa aliases too)"},
		{":wq  ·  :x", "save current request, then quit (:wqa too)"},
		{":help", "help overlay"},
	}},
}

// helpRows renders the overlay's content lines (title + every section). The
// blank separator rows are literal lines rather than style margins so the
// scroll window can slice them.
func helpRows() []string {
	keyStyle := lipgloss.NewStyle().Foreground(colOK).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colFg)
	headStyle := lipgloss.NewStyle().Foreground(colAccent).Bold(true)

	rows := []string{lipgloss.NewStyle().Foreground(colAccent).Bold(true).
		Render("Volley — keybindings")}
	for _, sec := range helpSections {
		rows = append(rows, "", headStyle.Render(sec.title))
		for _, kv := range sec.keys {
			rows = append(rows,
				"  "+keyStyle.Width(18).Render(kv[0])+descStyle.Render(kv[1]))
		}
	}
	return rows
}

// helpBoxOverhead is the rows the overlay chrome consumes around the content:
// the border (2), the vertical padding (2), and the footer block (blank + line).
const helpBoxOverhead = 6

// helpPageRows is how many content rows fit in the overlay at the current
// terminal height; helpMaxScroll is the largest useful helpScroll value.
func (m Model) helpPageRows() int {
	page := m.height - helpBoxOverhead
	if page < 1 {
		return 1
	}
	return page
}

func (m Model) helpMaxScroll() int {
	max := len(helpRows()) - m.helpPageRows()
	if max < 0 {
		return 0
	}
	return max
}

// helpView draws the overlay. Content taller than the terminal scrolls (j/k,
// ctrl+d/u, g/G) instead of being clipped off-screen.
func (m Model) helpView() string {
	rows := helpRows()
	page := m.helpPageRows()
	scroll := clampInt(m.helpScroll, 0, m.helpMaxScroll())

	footer := "press any key to close"
	if len(rows) > page {
		pos := "top"
		switch {
		case scroll >= m.helpMaxScroll():
			pos = "end"
		case scroll > 0:
			pos = fmt.Sprintf("%d%%", scroll*100/m.helpMaxScroll())
		}
		footer = "j/k scroll · " + pos + " · any other key closes"
		end := scroll + page
		if end > len(rows) {
			end = len(rows)
		}
		rows = rows[scroll:end]
	}
	rows = append(rows, "", lipgloss.NewStyle().Foreground(colDim).Render(footer))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 3).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
