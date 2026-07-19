# Volley

**Volley is a Vim-first terminal API client**: collections, tabs, auth helpers,
variables, curl import/export, pretty responses, and keyboard-native request
editing — all in one fast TUI.

It is built for people who live in the terminal and want a Postman/Bruno-style
workflow without leaving Vim muscle memory — plus built-in load testing driven
by editable load-shape profiles.

## Why Volley?

- **Vim-native workflow** — normal/insert modes, `hjkl`, `ctrl+w` pane movement,
  operators/motions in URL and body editors, and `:` commands.
- **Collections that feel like NerdTree** — folders, marks, recursive expand /
  collapse, context menu, request tabs, and tree-click tab opening.
- **Fast request editing** — Headers, Body, Params, and Auth tabs with Vim-like
  table navigation and a raw body editor.
- **Useful response viewer** — status/timing/size, pretty/raw JSON toggle,
  JSON syntax highlighting, search, yank, and selectable text.
- **Git-friendly storage** — saved requests are plain JSON files in your user
  config directory.
- **No account, no cloud, no browser** — just a terminal binary.

## Status

Functional MVP. Volley currently supports request/response editing, collections,
request tabs, auth helpers, variables, curl import/export, JSON response
highlighting, Vim-style navigation, and load testing with shaped profiles
(constant / ramp / spike / step / sawtooth), live charts, and p50/p95/p99.

## Tech stack

Go · [Bubble Tea](https://github.com/charmbracelet/bubbletea) ·
[Lip Gloss](https://github.com/charmbracelet/lipgloss) ·
[Bubbles](https://github.com/charmbracelet/bubbles)

## Run

```sh
go run .

# or build a local binary
go build -o volley .
./volley
```

Volley starts in **NORMAL mode** focused on the collections tree, so you can pick
a saved request immediately.

## Quick workflow

```text
j/k                 move through the collection tree
enter or click       open a request as a tab
,g then number       jump directly to a pane
ctrl+w h/j/k/l       move between panes
enter (URL bar)      send the request (:send works from anywhere)
p                   toggle raw/pretty JSON response
/                   search the response
:save name           save the current request
```

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `ctrl+w h/j/k/l` | move focus between panes; from Method/URL, `ctrl+w j` jumps to Body |
| `,g` | show numbered pane hints, then press the target number |
| `tab` / `shift+tab` | cycle focus between panes |
| arrow keys | mirror `h/j/k/l` inside the focused pane |
| `?` | help overlay (`j/k` scroll it; any other key closes) |
| `:` | command line |
| `q` | quit, prompting if there are unsaved changes |

### Collections tree

| Key | Action |
|-----|--------|
| `j/k` · `gg`/`G` · `P` | move selection · first/last · jump to top |
| click request | open request as a tab |
| `enter` / `l` / `o` | open request or toggle group |
| `O` / `X` | expand / collapse recursively |
| `A` | widen/narrow tree |
| `space` | mark/unmark request, then move down |
| `T` | open marked requests as tabs |
| `H` / `L` | switch open request tabs |
| `h` · `p` · `x` | collapse group · jump to parent · close parent |
| `,n` · `R` | show/hide tree · reload from disk |
| `m` | NerdTree-style context menu |
| `m a` · `m g` | add request · new group |
| `m r` · `m c` | rename · copy |
| `m d` · `dd` | delete request/group with confirmation |

### Method and URL

| Key | Action |
|-----|--------|
| `r` / `R` in Method pane | cycle HTTP method forward / back |
| `i` / `a` in URL | edit the URL |
| `enter` | send the request (Method or URL pane, INSERT or NORMAL) |
| URL NORMAL | Vim motions/operators, undo/redo, paste |
| `,t` | edit timeout inline |

### Request editor

| Key | Action |
|-----|--------|
| `[` / `]` | previous / next request sub-tab: Headers · Body · Params · Auth |
| `H` / `L` | switch open request tabs when tabs are open |
| `j/k` · `gg`/`G` | move rows in Headers/Params/Auth |
| `h/l` · `0/$` · `b/w` | move between key/value cells |
| `i/a/enter` · `I/A` | edit current/key/value cell |
| `o/O` | add row below/above |
| `dd` / `dj` | delete row |
| `space` | toggle row enabled/disabled |
| Body tab | `i/a/o` insert, `esc` to Vim-normal, `esc` again leaves |

### Response pane

| Key | Action |
|-----|--------|
| `[` / `]` | switch Body / Headers tab |
| `p` | toggle raw / pretty JSON body |
| `j/k` · `gg`/`G` | scroll · top/bottom |
| `ctrl+d` / `ctrl+u` | half-page scroll |
| `/` · `n`/`N` | search · next/previous match |
| `y` | yank response body to clipboard |

## Commands

| Command | Effect |
|---------|--------|
| `:new users/list` | create/open a blank saved request |
| `:save users/list` | save current request |
| `:save` / `:w` | save back to the current request |
| `:open users/list` | open a saved request |
| `:delete users/list` | delete a saved request |
| `:rename old new` | rename a saved request |
| `:copy old new` | copy a saved request |
| `:import curl …` | import a pasted curl command |
| `:copy curl` | copy current request as curl |
| `:editor` / `:editor name` | edit current or named saved request as JSON in `$VISUAL` / `$EDITOR` |
| `:mkgroup APISet1` | create a group/folder |
| `:rmgroup APISet1` | delete a group and all requests under it |
| `:rengroup old new` | rename a group |
| `:ls` | focus/refresh collections tree |
| `:method POST` | set HTTP method |
| `:set tok=abc123` | define a `{{tok}}` variable |
| `:send` | send current request |
| `:timeout 10s` | set request timeout |
| `:tabnew name` / `:tabe name` | open saved request as a tab |
| `:tabnext` / `:tabprevious` | switch request tabs |
| `:tabclose` / `:tabonly` | close active tab / close all other tabs |
| `:help` | help overlay |
| `:q` · `:q!` | quit · force quit discarding unsaved changes |
| `:wq` / `:x` | save unsaved changes in every tab, then quit |

Each tab is a live in-memory buffer: switching tabs preserves that tab's unsaved
edits (no save-first prompt, no disk reload), and a tab with unsaved changes
shows a `●` marker in the tabline. Closing a dirty tab asks for confirmation
before discarding. Unsaved edits are still guarded when opening another request
into the current tab, or when quitting — dirty background tabs included.

## Load testing

Press the **TEST** button (or `:loadtest`, alias `:lt`) to pick a load profile —
a plot of target request rate over time. The picker previews each shape as a
sparkline with its peak rate, duration, and total request count. After a y/n
confirmation showing exactly what will be fired at which URL, the response pane
becomes a live run view: ok/error/dropped counters, achieved vs. target RPS,
p50/p95/p99/max latency, and target + achieved charts.

| Key / command | Action |
|---------------|--------|
| `TEST` button · `:loadtest` | open the profile picker (`j/k` · `⏎` run · `esc` cancel) |
| `:loadtest <name>` / `:lt <name>` | run a named profile directly |
| `esc` | stop a running test (in-flight requests are cancelled) |
| `esc` / `T` on the results | close the results / run the same profile again |

Profiles are plain JSON in `loadprofiles/` beside your collections; the five
default shapes (constant, ramp-up, spike, step, sawtooth) are written there on
first use so you can edit them or copy them into your own. A profile is a list
of `{at, rps}` points — linear between points, and two points at the same
offset make a vertical jump:

```json
{
  "name": "spike",
  "points": [
    {"at": "0s",  "rps": 5},
    {"at": "20s", "rps": 5},
    {"at": "20s", "rps": 100},
    {"at": "30s", "rps": 100},
    {"at": "30s", "rps": 5},
    {"at": "50s", "rps": 5}
  ]
}
```

`maxWorkers` (default 64) caps concurrent in-flight requests; scheduled sends
that find no free worker are counted as **dropped** — the signal that the
target can't keep up at the plotted rate. Errors are transport failures plus
5xx responses.

## Storage and variables

Saved requests live under Volley's user config directory:

- Linux: `~/.config/volley/collections/`
- macOS: `~/Library/Application Support/volley/collections/`
- Windows: `%AppData%\\volley\\collections\\`

Groups are folders. Slash-separated names like `APISet1/auth/login` create nested
folders. Empty groups persist with a `.keep` marker.

`{{name}}` placeholders in URLs, headers, params, auth fields, and bodies are
expanded at send time. Volley resolves `:set` variables first, then process
environment variables. Unresolved placeholders are shown in the status bar before
sending.

## Roadmap

- [x] UI skeleton, panes, Vim modal core
- [x] Send request, render status + pretty JSON response, Vim scrolling
- [x] Request editor: Headers / Body / Params / Auth
- [x] Command line, response search, yank, help overlay
- [x] Collections: save/open/rename/copy/delete, NerdTree-style tree pane
- [x] Auth helpers: Bearer, Basic, API key
- [x] curl import/export
- [x] Request tabs (per-tab in-memory buffers with their own dirty state)
- [x] JSON syntax highlighting for responses
- [x] Load testing: shaped RPS profiles, worker cap, p50/p95/p99, live charts
- [ ] Tab session persistence across restarts
- [ ] Environments and persisted variable scopes
- [ ] In-TUI load-shape editor (edit points without leaving Volley)
- [ ] Load-test result history / export

> Note: collections are stored as native JSON. Posting/Postman/Bruno collection
> import/export is not implemented yet; curl import/export is supported.
