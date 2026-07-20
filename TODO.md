# Volley — Roadmap & UX Ideas

_Last updated: 2026-07-20 (rev 2)_

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
| Native desktop app              |   ✅    |  ✅   | ✅ (Wails, shared core) |
| Collections / folders           |   ✅    |  ✅   | ✅ |
| Request tabs                    |   ✅    |  ✅   | ✅ (TUI + GUI) |
| Variables                       |   ✅    |  ✅   | ✅ layered (session → env → OS) |
| Environments (dev/stg/prod)     |   ✅    |  ✅   | ✅ |
| Auth helpers (Bearer/Basic/…)   |   ✅    |  ✅   | ✅ |
| Body types (form/multipart/GQL) |   ✅    |  ✅   | ❌ (raw only) |
| Assertions / tests              |   ✅    |  ✅   | ❌ |
| Value extraction / chaining     |   ✅    |  ✅   | ❌ |
| curl import / export            |   ✅    |  ✅   | ✅ |
| Code generation (curl/wget/httpie) | ✅   |  ✅   | ✅ GUI (TUI pending) |
| Git sync (one-click)            |   ❌ (cloud) | ⚠️ (external git) | ✅ GUI (TUI pending) |
| OpenAPI import                  |   ✅    |  ✅   | ❌ |
| Request history                 |   ✅    |  ⚠️   | ❌ |
| Headless CLI runner (CI)        |  (Newman) | ✅ (`bru run`) | ❌ |
| **Load testing**                |   ❌    |  ❌   | ✅ shipped (unique): two executors (rate + concurrent users), shaped profiles, live charts, k6-style analysis, results browser |

**Volley's wedge:** the only tool that is *Vim-native + git-friendly + an API
client **and** a load tester* in one binary (terminal + desktop). The wedge is
now real — parity remains on bodies/tests/chaining, and the two front-ends need
to stay in lockstep (codegen + sync are GUI-only today).

---

## P0 — Essentials for daily use (close the parity gap)

These are the things whose absence makes a Postman user bounce.

- [ ] **Tab safety + session continuity** — make the request-tab workflow safe
      and durable before adding more tab features. **Effort: M.**
  - [x] Guard tab opens against dirty buffers — done in both front-ends: the
        TUI guards opening into the current tab; the GUI opens every request
        in its own tab (focus-if-open, Bruno-style), so nothing is clobbered.
  - [x] Per-tab dirty state with a marker on the tab label and a confirm on
        closing a dirty tab — done in TUI (`●` in the tabline) and GUI
        (dirty dot, styled confirm dialog).
  - [ ] Persist/restore open tabs, active tab, tree expansion, tree visibility,
        and active saved request under the Volley config directory. **S–M.**
        _One state file should also cover the GUI's tabs and the active
        environment (see Environments follow-up)._

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
- [x] **Named environments and reusable variables** — done (2026-07-19/20),
      in both front-ends. `vars.Layered` resolves session `:set` → active
      environment → process env, leaving unknown placeholders visible;
      `vars.EnvStore` keeps flat JSON files under `environments/` (`0600`,
      traversal-safe names, corrupt files skipped). TUI: `:env` /
      `:env <name>` / `:env off` / `:envnew` / `:envedit` ($EDITOR) /
      `:envrm`, Tab completion, active-env chip in the status bar, bare
      `:set` lists variable *names* only. GUI: env selector in the sidebar,
      masked key/value editor (JSON as an advanced toggle), save-activates.
      In-flight requests and load tests are frozen by construction — the
      request is built once at Send/TEST time. Remaining follow-ups:
  - [ ] Persist the selected environment across restarts (fold into the
        session-state file above). **S.**
  - [ ] `:envset name=value` / `:envunset` for quick edits without the
        $EDITOR round-trip. **S.**
  - [ ] Collection/global variable scopes between environment and OS env. **M.**
- [ ] **Front-end parity debt (TUI catch-up)** — the desktop app (Wails,
      `gui/`) has grown several features the TUI lacks. Keep the two
      front-ends in lockstep — shared logic belongs in `internal/`. **M.**
  - [ ] **Users-mode awareness (highest priority — a live inconsistency).**
        The engine dispatches on `Profile.Mode`, so the TUI already *runs*
        closed-loop profiles correctly, but every label still says rps: the
        run view's "rps achieved · target now", the picker's "peak N rps",
        and the shape editor can't set `mode` or `thinkTime` at all. A
        profile built in the GUI therefore executes right and reads wrong.
        **S–M.**
  - [ ] `:codegen curl|wget|httpie` — `internal/codegen` already exists; wire
        a command that copies to the clipboard (generalizes `:copy curl`). **S.**
  - [ ] `:sync` — extract `gui/sync.go` into `internal/gitsync` (config dir as
        a git repo, commit → pull --rebase → push, `environments/` and
        `loadresults/` gitignored) and call it from both front-ends. **S–M.**
  - [ ] `:loadresults` / `:loadcompare` (see Load testing). **M.**
  - [ ] GUI response search (the TUI's `/` `n/N`). **S–M.**

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

## P0.5 — GUI design debt (from the 2026-07-20 design review, 8/10)

Styling and palette are settled; what remains is information architecture.

- [ ] **Promote Load Tests from a modal to a workspace.** One dialog now holds
      profile selection, graphical shape editing, confirmation, live run,
      charts, history and comparison — a second workspace wearing a dialog's
      clothes, and the users executor made it denser still. Give it a
      first-class destination with Profiles / Live run / Results views. **L.**
- [ ] **One icon family.** The UI mixes a custom SVG gear with Unicode
      glyphs (`⟳ ⇅ ✎ ⧉ ✕ </> ◉ ◡`). A single 16px outlined SVG set would do
      more for the professional finish than any further spacing work. **M.**
- [ ] **Calm the central chrome.** Three stacked horizontal layers (request
      tabs, method/URL bar, Headers/Query/Body/Auth tabs). The duplicated
      request name is gone; folding "Import curl" into a compact actions menu
      is the remaining step. **S.**
- [x] Results as metric cards with deltas, raw report behind a disclosure.
- [x] Empty collection state offers create/import actions.
- [x] A 960×600 minimum window instead of chasing narrow layouts.

## P1 — Differentiators & workflow speed

Where a Vim TUI can feel *faster* than the GUIs.

- [x] **Load testing (the roadmap headline)** — shipped: `internal/loadtest`
      engine with shaped RPS profiles, worker cap, and drop accounting.
  - [x] Shared `http.Transport` with tuned `MaxIdleConnsPerHost`
        (`httpx.sharedClient` + `DoLoad`).
  - [x] Run config via editable load-shape profiles (constant / ramp / spike /
        step / sawtooth + user shapes; `maxRequests`, `maxWorkers`).
  - [x] Live dashboard in both front-ends: counters, achieved-vs-target RPS,
        p50/p90/p95/p99/max, sparkline charts (TUI) / SVG charts (GUI).
  - [x] Summary report + export: k6-style analysis auto-saved under
        `loadresults/`. _Remaining: CSV export / HTML CI artifact if needed._
  - [x] Generate load test from the current request — TEST / `:loadtest` runs
        the open request through the picked profile after a confirm step.
  - [x] Shape editing: TUI key-driven shape editor; GUI drag-the-points
        editor with raw-JSON toggle.
  - [x] **Load-test comparison** — GUI results browser (history in the load
        panel): runs filterable by profile, p99 trend across runs, and a
        delta line vs the previous run of the same profile.
  - [x] **Closed-loop "users" executor** (2026-07-20) — profiles carry a
        `mode`: `""` keeps the open-loop rate model (target rps on a clock,
        drops when the worker cap bites), `"users"` runs N concurrent virtual
        users, each looping send → await response → think → repeat, so
        throughput becomes an outcome and nothing is ever dropped. The
        plotted shape means user count in that mode, so ramps/spikes/steps
        work unchanged; `thinkTime` sets the per-user pause. GUI exposes
        mode + think time in the shape editor with mode-aware previews,
        counters and reports.
  - [ ] TUI equivalent: `:loadresults` / `:loadcompare` over the same
        `ResultStore` data. **M.**
  - [ ] CSV export of a run, and an HTML artifact for CI. **S–M.**
  - **Why:** neither Postman nor Bruno does this; k6/vegeta aren't
        interactive. This is Volley's signature feature — shipped.
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
      very git-diff-friendly and popular. _Partial: code generation to
      curl/wget/httpie shipped (`internal/codegen`, GUI `</>` button); fetch/Go
      snippets and the import side remain._ **Why:** onboarding from existing
      tools. **Effort: L.**
- [ ] **Secrets management** — `.env`-style secret vars, masked in the UI, kept
      out of committed collection files (`.gitignore`d). _Partial: environment
      files are `0600`, values are masked in the GUI editor, bare `:set` lists
      names only, and git sync gitignores `environments/` by default. Remaining:
      per-variable sensitive marking and export scrubbing._ **Why:** Bruno
      secrets; avoids leaking tokens into git. **Effort: M.**
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

_(curl, auth, environments, and the load-testing MVP from the original list
have all shipped — in both front-ends where applicable.)_

1. **TUI users-mode awareness** (S–M) — closes an inconsistency that exists
   *today*: closed-loop profiles run correctly in the TUI but are labelled as
   rps everywhere, and can't be created there.
2. **TUI catch-up** (M) — `:codegen`, `:sync` (extract `internal/gitsync`),
   `:loadresults`/`:loadcompare`; keeps the two-front-end promise honest.
3. **Load Tests workspace** (L) — the design review's top item, now overdue.
4. **Session persistence** (S–M) — one state file for open tabs, tree state,
   and the active environment, shared by both apps.
5. **Value extraction / chaining** (M) — turns Volley into a workflow tool;
   environments (its prerequisite) now exist.
6. **Body types + auto Content-Type** (L) — the last "Postman user bounces"
   parity gap.
7. **Request history** (M) — cheap now that the results browser set the
   pattern for browsing stored run data.

> Rule of thumb: whatever ships next, ship it in **both front-ends** (logic in
> `internal/`), then keep differentiating on load testing and keyboard-speed
> workflows (fuzzy open, chaining, CLI runner) that no GUI can match.
