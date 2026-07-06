# Volley

**Volley is a Vim-first terminal API client**: collections, tabs, auth helpers,
variables, curl import/export, pretty responses, and keyboard-native request
editing — all in one fast TUI.

It is built for people who live in the terminal and want a Postman/Bruno-style
workflow without leaving Vim muscle memory. Load testing is planned as Volley's
next major differentiator.

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
highlighting, and Vim-style navigation. Load testing is not implemented yet.

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
:send                send the request
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
| `?` | help overlay |
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
| `r` in Method pane | cycle HTTP method |
| `i` / `a` in URL | edit the URL |
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
| `:wq` / `:x` | save current request, then quit |

Unsaved edits are guarded when opening another request or quitting. Closing a
dirty active tab asks for confirmation before discarding changes.

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
- [x] Request tabs
- [x] JSON syntax highlighting for responses
- [ ] Tab persistence and fuller per-tab dirty state
- [ ] Environments and persisted variable scopes
- [ ] Load testing: concurrency, RPS, p50/p95/p99, live charts

> Note: collections are stored as native JSON. Posting/Postman/Bruno collection
> import/export is not implemented yet; curl import/export is supported.
