# Volley — Roadmap & UX Ideas

_Last updated: 2026-07-05_

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
| Auth helpers (Bearer/Basic/…)   |   ✅    |  ✅   | ✅ |
| Body types (form/multipart/GQL) |   ✅    |  ✅   | ❌ (raw only) |
| Assertions / tests              |   ✅    |  ✅   | ❌ |
| Value extraction / chaining     |   ✅    |  ✅   | ❌ |
| curl import / export            |   ✅    |  ✅   | ✅ |
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

- [ ] **Tab safety + session continuity** — make the new request-tab workflow safe
      and durable before adding more tab features. **Effort: M.**
  - [ ] Guard tree-click / `:tabnew` tab opens when the current editor is dirty,
        or open in the background without replacing the dirty buffer. This fixes
        the current risk of silently discarding edits when a clicked request is
        not already open. **S.**
  - [ ] Track per-tab dirty state and render a dirty marker on the tab label;
        block or confirm closing/switching away from dirty tabs. **M.**
  - [ ] Persist/restore open tabs, active tab, tree expansion, tree visibility,
        and active saved request under the Volley config directory. **S–M.**

- [x] **Auth helpers** — done: a request-level `model.Auth` (Bearer / Basic /
      API-key) materialized at send time by `Request.ApplyAuth` into the right
      header (or query param for API-key). Edited in a new **Auth** tab in the
      request pane (`components.AuthEditor`: type selector + type-specific
      fields, password masked). `{{vars}}` in auth fields are expanded by
      `vars.Apply` and flagged by `vars.Unresolved`; auth persists via the
      versioned storage DTO (legacy files without it still load) and counts
      toward unsaved-changes detection. Injected header is appended last so it
      overrides a hand-written one. Tested at the model, vars, storage, and
      component levels. _Follow-up: OAuth2 flows are out of scope for now._
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
- [x] **curl import & export** — done: `internal/curl` (Parse + Format, fully
      tested incl. browser "Copy as cURL" format and round-trip). `:import curl …`
      fills the request (method, headers, data, basic auth, --max-time; unknown
      flags warned, not fatal) and guards unsaved edits; `:copy curl` copies the
      current request (vars expanded, query folded) to the clipboard. Follow-ups:
      `yc` keybind, and splitting an imported query string into the Params table.
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
  - [ ] Summary report + export (JSON/CSV, plus an HTML CI artifact). **S.**
  - [ ] **Generate load test from the current request** — capstone: take the
        open/saved `model.Request` and hand it to `loadx` (`:load` /
        `:loadtest`). Turns the API client and the load tester into one
        workflow instead of two features. **S** (once the MVP exists).
  - [ ] **Load-test comparison mode** — diff two runs (latency p50/p90/p99, RPS,
        error-rate deltas) to answer "did my change regress p99?". **M.**
  - **Why:** neither Postman nor Bruno does this; k6/vegeta aren't interactive.
        This is Volley's signature feature — and generate-from-request +
        comparison are what make it a *workflow*, not just a dashboard.
- [x] **Numbered pane jump (` ,g`)** — EasyMotion-style focus hints over the
      panes; press the shown number to jump directly. **Effort: S.**
- [ ] **Fuzzy quick-open / command palette (`ctrl+p`)** — Telescope-style
      incremental finder over saved requests, and extend the same picker to
      commands, environments, and variables. **Why:** big trees are slow to
      `j/k` through; fuzzy jump is the Vim-user expectation. **Effort: M.**
- [ ] **Request history** — ring of recent sends (method, URL, status, time),
      re-run or promote to a saved request. Persisted. **Why:** Postman history
      panel; great for exploratory work. **Effort: M.**
- [ ] **Richer response views** — _Done: **Raw ↔ Pretty** toggle (`p`) on the
      response Body tab, mode shown in `respTabBar`._ Still to do, as one
      "response inspector" push:
  - a **Cookies** tab (parsed `Set-Cookie`).
  - a **Timing** waterfall (DNS / connect / TLS / TTFB / download via
        `httptrace`), with **latency-budget warnings** — highlight hops/totals
        over a configured threshold.
  - a **TLS/certificate inspector** (chain, issuer, expiry) — cheap once the
        `httptrace` timing plumbing exists; bundle it here.
  - a **redirect-chain** view (each hop: status, `Location`, timing) — see also
        P2 Network options.
  - a **rate-limit helper**: on `429`, surface reset time from
        `Retry-After` / `X-RateLimit-*` headers.
  - _Done:_ JSON syntax highlighting for the response body reuses the request
        body highlighter.
  - add XML/HTML syntax highlighting.
  **Why:** Postman's response inspector, but terminal-native. **Effort: M.**
- [ ] **Save response to file / yank path** — `:w response.json`, and
      `:extract`-style copy of a JSONPath value to the clipboard. Complements the
      10 MiB read cap (offer "save full body"). **Why:** large payloads. **S.**
- [ ] **Response diff & snapshots** — save an expected response for a request
      (a "snapshot" file next to the collection) and diff the current response
      against it — or against the previous response — with a side-by-side/unified
      view (`:diff`, `:snapshot`). Feeds golden-file regression checks in the
      Tests + CLI runner. **Why:** git-friendly regression detection no GUI does
      in-terminal; pairs with assertions. **Effort: M.**
- [ ] **Assertions / tests (no JS)** — a terminal-friendly DSL per request:
      `status == 200`, `header Content-Type ~ json`, `json .ok == true`,
      `time < 500ms`. Show a **Tests** tab with pass/fail. **Why:** Postman/Bruno
      tests without embedding a JS engine. **Effort: M–L.**
- [ ] **Headless CLI runner** — `volley run auth/login` (and
      `volley run --dir APISet1`) executes saved requests + assertions, exits
      non-zero on failure, `--json` output. **Why:** `bru run` / Newman for CI;
      makes collections executable. **Effort: M.** _Reuses the engine + Tests._

- [ ] **Quick wins (S each)** — small, high-satisfaction editor niceties:
  - **Open in `$EDITOR`** — edit large bodies/headers in external Vim/Neovim.
  - **Variable autocomplete** — complete `{{token}}`, `{{baseUrl}}`, … from the
        active scopes while typing.
  - **Inline variable preview** — show the fully-resolved URL / headers / body
        before send (extends the already-done unresolved-`{{var}}` warning).

## P2 — Polish, interop & power features

- [ ] **`:config` command + config file** — add an in-app configuration command
      backed by `~/.config/volley/config.toml`. It should let users inspect and
      change settings without hand-editing TOML, e.g. `:config theme dark`,
      `:config collections.dir ~/api`, `:config timeout 10s`,
      `:config editor nvim`, `:config get`, and `:config reset <key>`. Persist
      changes atomically and reload affected UI/runtime settings immediately
      where possible. **Why:** users should be able to customize Volley from
      inside Volley. **Effort: M.**
  - [ ] **Theme configuration** — palette/theme name, with bundled dark/light
        themes and a future path for custom colors.
  - [ ] **Request save directory** — configurable collections root, including
        validation, migration guidance, and tree reload after change.
  - [ ] **Runtime defaults** — default timeout, redirect policy, raw/pretty
        response preference, startup focus/mode, and tab/session restore.
  - [ ] **Configurable keybindings** — remap Vim bindings via config once the
        config layer exists. **M.**
- [ ] **`:editor` command / external editor integration** — _partial:_
      `:editor` opens the current request as JSON and `:editor <request-name>`
      opens a named saved request in `$VISUAL` or `$EDITOR`, suspends Bubble Tea
      with `tea.ExecProcess`, and reloads/saves edits when the editor exits.
      Remaining work: support `:config editor`, YAML, focused-section editing
      (`body`, `headers`, `params`, `auth`), friendlier structured validation,
      and richer dirty/conflict handling. **Why:** large bodies and complex
      requests are easier in a real editor. **Effort: M.**
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

## Considered & deferred (2026-07-05 idea review)

Batch of feature ideas reviewed against the wedge; decisions recorded so they
aren't re-litigated:

- **Flow runner / session recorder** — _not a separate feature._ "login →
  extract token → call → assert" is already the composition of **value
  extraction/chaining** + **assertions** + the **headless CLI runner**. Build
  those primitives first; a "flow" becomes a saved sequence of them. Revisit a
  dedicated flow UI only if the composed primitives prove awkward.
- **Pre-request / post-response hooks** — powerful but a big surface; most use
  cases are covered by extraction + assertions. Defer until those ship and a
  real gap is visible.
- **Macros (keystroke recording)** — declined. It's a Vim TUI; users already
  have Vim macros. Poor ROI.
- **Generate tests from response** — worth doing, but folds naturally into the
  Assertions/tests item (offer "make assertions from this status/body shape").
- **Collection README/docs pane** — markdown notes per collection/folder;
  git-friendly, low urgency. Parking in P2 if it comes up again.
- **Secret detection**, **OpenAPI endpoint browser** — already covered by P2
  Secrets management and Import/export & code-gen respectively.

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
