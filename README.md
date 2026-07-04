# Volley

A Vim-centric TUI API client and load tester — a terminal alternative to
Postman, inspired by [posting](https://github.com/darrenburns/posting) but
built for Vim users and high-concurrency load testing.

**Status:** functional MVP — request/response, Vim URL/body editing, collections,
auth helpers, and curl import/export are in. Load testing is the next major
feature.

## Stack

Go · [Bubble Tea](https://github.com/charmbracelet/bubbletea) ·
[Lip Gloss](https://github.com/charmbracelet/lipgloss) ·
[Bubbles](https://github.com/charmbracelet/bubbles)

## Run

```sh
go run .          # or: go build -o volley . && ./volley
```

## Keys (so far)

| Key            | Action                                          |
|----------------|-------------------------------------------------|
| `ctrl+w` `h/j/k/l` | move focus between panes (Vim window nav)   |
| arrow keys     | mirror `h/j/k/l` — move **within** the focused pane |
| `tab` / `shift+tab` | cycle focus between panes                  |
| `:send` or SEND button | send request                            |
| **Method pane** | `j`/`k` or `↑`/`↓` cycle the HTTP method (`tab`/`ctrl+w` to reach it) |
| **URL bar**    | **types directly — just start typing, no `i` needed** |
| `tab`/`ctrl+w` | move to another pane                            |
| `esc` (URL)    | drop to NORMAL sub-mode with Vim edits (`x`, `w`, `b`, `C`, `dd`, `p`, `u`, …) |
| URL NORMAL     | `i/a/I/A`, motions/operators, undo/redo, paste; `esc` again releases to pane navigation |
| `,t`           | focus / edit the timeout field                   |
| `i` / `a`      | edit focused field / cell (headers, params, auth fields, body) |
| `esc`          | leave INSERT, back to NORMAL                     |
| `,n`           | show / hide collections tree                    |
| `q`            | quit (prompts if there are unsaved changes)     |
| **Collections pane (NerdTree)** |                                 |
| `j/k` · `gg`/`G` · `P` | move selection · first/last · jump to top |
| `enter`/`l`/`o` | open request or toggle group                     |
| `O` / `X`       | expand / collapse group **recursively**          |
| `h` · `p` · `x` | collapse group · jump to parent · close parent   |
| `,n` · `R`      | show / hide tree · reload from disk              |
| `m`             | open NerdTree-style menu (context-aware)         |
| `m a` · `m g`   | add request into group · **new group**           |
| `m r` · `m c`   | rename request/group · copy request              |
| `m d` · `dd`    | delete request or group (asks `y/n` to confirm)  |
| **Request pane** |                                               |
| `[` / `]` · `H`/`L` | previous / next tab (Headers · Body · Params · Auth) |
| **Headers / Params** |                                             |
| `j/k` · `gg`/`G` | move between rows · first/last row             |
| `h/l` · `0/$` · `b/w` | key/value cell                            |
| `i/a/enter` · `I/A` | edit current/key/value cell                  |
| `o/O`          | add row below/above · `dd`/`dj` delete · `space` toggle on/off |
| **Auth tab**   | `space`/`l`/`h` on Type cycles None/Bearer/Basic/API Key; edit fields with `i`/`enter` |
| API Key auth   | `space`/`h`/`l` on “Add to” switches Header vs Query param |
| **Body editor (Vim)** | `i`/`a`/`o` insert; `esc` → field-NORMAL; `esc` again leaves |
| in field-NORMAL | `x dd D C s r`, operators `d/c/y` + motions `w b e $ 0`, counts (`3x`), `u`/`ctrl+r`, `p`/`P` |
| `?`            | toggle keybindings help overlay                  |
| `:`            | command line (see below)                          |
| **Response pane** |                                              |
| `[` / `]`      | switch Body / Headers tab                         |
| `j/k`          | scroll · `gg`/`G` top/bottom · `^d`/`^u` half-page |
| `/` · `n`/`N`  | search · next / previous match                   |
| `y`            | yank response body to clipboard                  |

## Command line & variables

| Command            | Effect                                            |
|--------------------|---------------------------------------------------|
| `:new users/list`  | create/open a blank saved request                 |
| `:save users/list` | save the current request                          |
| `:save` / `:w`     | save back to the currently opened/created request |
| `:open users/list` | open a saved request                              |
| `:delete users/list` | delete a saved request                          |
| `:rename old new`  | rename a saved request                            |
| `:copy old new`    | copy a saved request                              |
| `:import curl …`   | fill the editor from a pasted curl command        |
| `:copy curl`       | copy the current request as a curl command        |
| `:mkgroup APISet1` | create a group (folder), even when empty          |
| `:rmgroup APISet1` | delete a group and everything under it            |
| `:rengroup old new`| rename a group                                    |
| `:ls`              | focus/refresh the collections tree                |
| `:method POST`     | set the HTTP method                               |
| `:set tok=abc123`  | define a variable usable as `{{tok}}`             |
| `:send`            | send the current request                          |
| `:timeout 10s`     | set the request timeout (or press `,t`)           |
| `:help`            | help overlay                                      |
| `:q` · `:q!`       | quit · quit discarding unsaved changes (`:qa` aliases work too) |
| `:wq` / `:x`       | save the current request, then quit (`:wqa`/`:xa` aliases work too) |

When a request has unsaved edits, switching to another request or quitting
prompts to **save** (`y`), **discard** (`n`), or **cancel** (`esc`) so your
changes are never silently lost. The method selector is its own focus target —
reach it with `tab`/`shift+tab` or the `ctrl+w` window motions. The timeout field
is edited inline with `,t` or `:timeout`. Arrow keys only move *within* the
focused pane, never between panes.

Saved requests are stored as JSON under `~/.config/volley/collections/`.
Groups are folders: slash-separated names like `APISet1/auth/login` nest a
request inside groups. Empty groups persist (they keep a `.keep` marker), while
folders created implicitly by saving are cleaned up when their last request goes.

`{{name}}` placeholders in the URL, headers, params, auth fields, and body are
expanded at send time — resolved from `:set` variables first, then process
environment variables (so `{{HOME}}`, secrets exported in your shell, etc. work).
Unresolved placeholders are surfaced in the status bar before the request is
sent.

## Roadmap

- [x] **Phase 1** — UI skeleton, panes, Vim modal core
- [x] **Phase 2** — send request, render status + pretty-JSON response, Vim scroll
- [x] **Phase 3** — request editor: tabbed headers / body / params / auth, Vim KV editor
- [x] **Phase 4** — `:` command line, `/` response search, `y` yank, `?` help overlay
- [x] **Vim editing engine** (`internal/vimtext`) — motions, operators, counts, undo/redo, in the body editor
- [x] **Collections** — save/open/rename/copy/delete, NerdTree-style tree pane (native JSON storage)
- [x] **Auth helpers** — Bearer, Basic, and API-key auth, injected at send time
- [x] **curl import/export** — `:import curl …` and `:copy curl`
- [ ] Load testing (concurrency, RPS, p50/p95/p99, live charts)

> Note: collections are stored as native JSON under `~/.config/volley/collections/`.
> Posting-format import/export is not implemented yet; curl import/export is supported.
