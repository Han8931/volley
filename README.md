# Volley

A Vim-centric TUI API client and load tester — a terminal alternative to
Postman, inspired by [posting](https://github.com/darrenburns/posting) but
built for Vim users and high-concurrency load testing.

**Status:** early development. Phase 1 (UI skeleton + modal editing) is in.

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
| arrow keys     | move focus directionally                        |
| `tab` / `shift+tab` | cycle focus                                |
| `⏎`            | send request                                    |
| `i` / `a`      | edit focused field / cell                       |
| `esc`          | leave INSERT, back to NORMAL                     |
| `h`/`l`         | previous / next HTTP method (URL focused)        |
| `m`            | next HTTP method (URL focused)                    |
| `,n`           | show / hide collections tree                    |
| `q`            | quit                                            |
| **Collections pane** |                                            |
| `j/k` · `gg`/`G` | move selection · first/last saved request       |
| `enter`/`l`/`o` | open request or toggle folder                    |
| `h`             | collapse folder                                  |
| `m`             | open NerdTree-style menu                         |
| `,n`            | show / hide tree pane                            |
| `m a`           | add/save current request                         |
| `m r` / `m c`   | rename / copy selected request                   |
| `m d` / `dd`/`dj` | delete selected request                        |
| **Request pane** |                                               |
| `[` / `]` · `H`/`L` | previous / next tab (Headers · Body · Query) |
| `j/k` · `gg`/`G` | move between rows · first/last row             |
| `h/l` · `0/$` · `b/w` | key/value cell                            |
| `i/a/enter` · `I/A` | edit current/key/value cell                  |
| `o/O`          | add row below/above · `dd`/`dj` delete · `space` toggle on/off |
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
| `:save users/list` | save the current request                          |
| `:open users/list` | open a saved request                              |
| `:delete users/list` | delete a saved request                          |
| `:rename old new`  | rename a saved request                            |
| `:copy old new`    | copy a saved request                              |
| `:ls`              | focus/refresh the collections tree                |
| `:method POST`     | set the HTTP method                               |
| `:set tok=abc123`  | define a variable usable as `{{tok}}`             |
| `:timeout 10s`     | set the request timeout                           |
| `:help` · `:q`     | help overlay · quit                               |

Saved requests are stored as JSON under `~/.config/volley/collections/`.
Use slash-separated names like `auth/login` to organize them into folders.

`{{name}}` placeholders in the URL, headers, query, and body are expanded at
send time — resolved from `:set` variables first, then process environment
variables (so `{{HOME}}`, secrets exported in your shell, etc. work).

## Roadmap

- [x] **Phase 1** — UI skeleton, panes, Vim modal core
- [x] **Phase 2** — send request, render status + pretty-JSON response, Vim scroll
- [x] **Phase 3** — request editor: tabbed headers / body / query, Vim KV editor
- [x] **Phase 4** — `:` command line, `/` response search, `y` yank, `?` help overlay
- [ ] Collections (posting-compatible storage)
- [ ] Load testing (concurrency, RPS, p50/p95/p99, live charts)
