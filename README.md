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

| Key       | Action                            |
|-----------|-----------------------------------|
| `h/j/k/l` | move focus between panes          |
| `i` / `a` | edit the focused field (URL bar)  |
| `esc`     | back to NORMAL mode               |
| `m`       | cycle HTTP method (URL focused)   |
| `q`       | quit                              |

## Roadmap

- [x] **Phase 1** — UI skeleton, panes, Vim modal core
- [ ] **Phase 2** — send request, render status/headers/pretty-JSON response
- [ ] **Phase 3** — request editor (headers / body / query)
- [ ] **Phase 4** — `:` command line, response search, yank, help overlay
- [ ] Collections (posting-compatible storage)
- [ ] Load testing (concurrency, RPS, p50/p95/p99, live charts)
