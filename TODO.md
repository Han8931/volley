# Volley — Roadmap & UX Ideas

_Last updated: 2026-07-02_

Ideas to grow Volley from a functional MVP into a tool that a Postman/Bruno user
would happily switch to **in the terminal**. Each item notes **why** it matters
(with a Postman/Bruno reference) and a rough effort: **S** (hours), **M** (a day
or two), **L** (multi-day).

## Where Volley stands vs. Postman / Bruno

| Capability                      | Postman | Bruno | Volley (today) |
|---------------------------------|:-------:|:-----:|:--------------:|
| Send request / view response    |   ✅    |  ✅   | ✅ |
| Vim-native, terminal-first      |   ❌    |  ❌   | ✅ (unique) |
| Git-friendly, offline, no account |  ❌    |  ✅   | ✅ |
| Collections / folders           |   ✅    |  ✅   | ✅ |
| Variables                       |   ✅    |  ✅   | ⚠️ single in-memory scope |
| Environments (dev/stg/prod)     |   ✅    |  ✅   | ❌ |
| Auth helpers (Bearer/Basic/…)   |   ✅    |  ✅   | ❌ (manual header) |
| Body types (form/multipart/GQL) |   ✅    |  ✅   | ❌ (raw only) |
| Assertions / tests              |   ✅    |  ✅   | ❌ |
| Value extraction / chaining     |   ✅    |  ✅   | ❌ |
| curl import / export            |   ✅    |  ✅   | ❌ |
| OpenAPI / code-gen import       |   ✅    |  ✅   | ❌ |
| Request history                 |   ✅    |  ⚠️   | ❌ |
| Headless CLI runner (CI)        |  (Newman) | ✅ (`bru run`) | ❌ |
| **Load testing**                |   ❌    |  ❌   | 🎯 planned (unique) |

**Volley's wedge:** the only tool that is *Vim-native + git-friendly + an API
client **and** a load tester* in one terminal binary. Lean into that — parity on
the boring essentials (auth, envs, bodies), then differentiate hard on
keyboard-speed workflows and load testing.

---

## P0 — Essentials for daily use (close the parity gap)

These are the things whose absence makes a Postman user bounce.

- [ ] **Auth helpers** — a request-level auth type applied at send time (injects
      the right header) instead of hand-writing `Authorization`. Start with
      Bearer token, Basic (user/pass), and API-key (header or query). **Why:**
      table-stakes in both Postman & Bruno. **Effort: M.**
      _Touchpoints: `model.Request` (add `Auth`), `httpx.Do`, a request pane tab._
- [ ] **Environments** — multiple named variable sets (dev/staging/prod) that
      switch instantly (`:env prod`), persisted as files under
      `~/.config/volley/environments/*.json`. Show the active env in the status
      bar. **Why:** the single most-used Postman/Bruno feature; today's `:set`
      vars are one unnamed in-memory scope and vanish on exit. **Effort: M.**
      _Touchpoints: `internal/vars` (persistence + scopes), status bar._
- [ ] **Persist & scope variables** — resolution order: request → environment →
      collection → global → OS env. Persist non-secret vars. **Why:** Bruno/Postman
      scoping. **Effort: M.** _Builds on Environments._
      _Done: unresolved `{{name}}` placeholders are surfaced as a status-bar
      warning before send (`vars.Unresolved`), listing exactly which vars are
      still unbound. Persistence + scoping remain._
- [ ] **Body types + auto Content-Type** — pick a body mode: raw (JSON/text/XML),
      `x-www-form-urlencoded`, `multipart/form-data` (with file fields), and
      GraphQL (query + variables). Auto-set `Content-Type` unless overridden.
      **Why:** raw-only blocks form posts & uploads that both rivals support.
      **Effort: L.** _Touchpoints: request Body tab, `model.Request`, `httpx`._
- [ ] **curl import & export** — `:import curl` (paste a `curl …` → fills the
      request) and `yc` / `:copy curl` (copy the current request as a curl
      command). **Why:** devs paste curl constantly; highest QoL-per-effort win.
      **Effort: S–M.**
- [ ] **Value extraction / request chaining** — after a response,
      `:extract token = json .data.token` (JSONPath/GJSON) saves into the active
      environment so `{{token}}` works in the next request. **Why:** auth-then-call
      flows are the #1 reason people script Postman; this makes them one line.
      **Effort: M.** _Pairs with Environments + Tests._

## P1 — Differentiators & workflow speed

Where a Vim TUI can feel *faster* than the GUIs.

- [ ] **Load testing (the roadmap headline)** — reuse `httpx` (already
      UI-agnostic) behind a `loadx` package. Concrete sub-tasks:
  - [ ] Shared `http.Transport` with tuned `MaxIdleConnsPerHost` (a fresh
        `http.Client` per request won't scale — flagged in review). **S.**
  - [ ] Run config: concurrency, duration **or** N requests, target RPS. **M.**
  - [ ] Live TUI dashboard: RPS, in-flight, latency p50/p90/p99, status
        histogram, error rate, sparkline. **L.**
  - [ ] Summary report + export (JSON/CSV). **S.**
  - **Why:** neither Postman nor Bruno does this; k6/vegeta aren't interactive.
        This is Volley's signature feature.
- [ ] **Fuzzy quick-open (`ctrl+p`)** — Telescope-style incremental finder over
      all saved requests (and commands). **Why:** big trees are slow to `j/k`
      through; fuzzy jump is the Vim-user expectation. **Effort: M.**
- [ ] **Request history** — ring of recent sends (method, URL, status, time),
      re-run or promote to a saved request. Persisted. **Why:** Postman history
      panel; great for exploratory work. **Effort: M.**
- [ ] **Richer response views** — extend the Body/Headers tab bar (`respTabBar`)
      with **Raw ↔ Pretty** toggle, a **Cookies** tab (parsed `Set-Cookie`), and
      a **Timing** view (DNS / connect / TLS / TTFB via `httptrace`). Add
      syntax highlighting to the response body (reuse the JSON highlighter; add
      XML/HTML). **Why:** Postman's response inspector. **Effort: M.**
- [ ] **Save response to file / yank path** — `:w response.json`, and
      `:extract`-style copy of a JSONPath value to the clipboard. Complements the
      10 MiB read cap (offer "save full body"). **Why:** large payloads. **S.**
- [ ] **Assertions / tests (no JS)** — a terminal-friendly DSL per request:
      `status == 200`, `header Content-Type ~ json`, `json .ok == true`,
      `time < 500ms`. Show a **Tests** tab with pass/fail. **Why:** Postman/Bruno
      tests without embedding a JS engine. **Effort: M–L.**
- [ ] **Headless CLI runner** — `volley run auth/login` (and
      `volley run --dir APISet1`) executes saved requests + assertions, exits
      non-zero on failure, `--json` output. **Why:** `bru run` / Newman for CI;
      makes collections executable. **Effort: M.** _Reuses the engine + Tests._

## P2 — Polish, interop & power features

- [ ] **Config file + theming** — `~/.config/volley/config.toml` for palette
      (the colors are hardcoded with a "theming is a later, central change"
      note), default timeout, redirect policy, keybindings. Ship a light and a
      dark theme. **Why:** GUIs are themeable; TUIs should be too. **Effort: M.**
- [ ] **Configurable keybindings** — remap the Vim bindings via config. **M.**
- [ ] **Import/export & code-gen** — OpenAPI → collection, Postman/Insomnia/Bruno
      import, and `.http`/`.rest` (VS Code REST Client) read/write — the last is
      very git-diff-friendly and popular. Code generation to curl/httpie/fetch.
      **Why:** onboarding from existing tools. **Effort: L.**
- [ ] **Secrets management** — `.env`-style secret vars, masked in the UI, kept
      out of committed collection files (`.gitignore`d). **Why:** Bruno secrets;
      avoids leaking tokens into git. **Effort: M.**
- [ ] **Network options** — per-request/global redirect follow toggle + redirect
      chain view, `--insecure` TLS, client certs, and proxy support. **Why:**
      Postman request settings. **Effort: M.**
- [ ] **Cookie jar / sessions** — persist cookies across requests in an
      environment so login sessions carry. **Why:** Postman cookie jar. **M.**
- [ ] **Plain-text collection format** — optional Bruno-style `.bru`-ish or
      `.http` on-disk format for cleaner git diffs than pretty-printed JSON.
      **Why:** Bruno's whole pitch is legible diffs. **Effort: M.**
- [ ] **GraphQL / WebSocket / SSE** — first-class GraphQL body (done in P0 body
      types), plus streaming protocols as a stretch. **Why:** modern APIs.
      **Effort: L.**

## Foundations (do alongside features — from the code review)

- [ ] **Corrupt-file resilience** — a truncated/hand-edited `*.json` collection
      should be skipped with a warning, not break the tree. **Effort: S.**
- [x] **`kveditor` test coverage** — done: `kveditor_test.go` covers the
      index/motion logic (h/l/j/k/gg/G, dd & dj delete + cursor clamping, o/O/i/I/A
      insert, space toggle, inline edit commit/esc/tab-hop, SetRows reset, empty-list
      safety). Package coverage 0% → 77% (remainder is lipgloss rendering).
      **Effort: S.**
- [x] **Persisted request schema versioning** — done: `internal/collections/format.go`
      holds a versioned `storedRequest` DTO (explicit `json` tags, `schemaVersion`,
      timeout as a `"10s"` string) so `model.Request` stays storage-agnostic; `Load`
      still accepts the legacy capitalized/nanosecond format. Saves are now atomic
      (temp + fsync + rename), and a failed collections list at startup surfaces in
      the status bar instead of being silently dropped.

---

## Suggested near-term order

1. **curl import/export** (S–M) — instant, visible win; low risk.
2. **Auth helpers** (M) — removes the most common daily friction.
3. **Environments + persisted/scoped variables** (M) — unlocks real multi-stage
   workflows and is prerequisite for extraction/chaining.
4. **Value extraction / chaining** (M) — turns Volley into a workflow tool.
5. **Load testing MVP** (L) — the differentiator; start with the shared
   transport + run config, then the live dashboard.

> Rule of thumb: reach **parity** on P0 (auth, envs, bodies, curl) so Volley is
> *sufficient* for daily use, then pour effort into the **load tester** and
> **keyboard-speed** workflows (fuzzy open, chaining, CLI runner) that no GUI
> can match.
