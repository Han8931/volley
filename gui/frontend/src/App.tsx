import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  blankRequest,
  CODE_FORMATS,
  formatDuration,
  METHODS,
  parseDuration,
  type CodeFormat,
  type EnvState,
  type Header,
  type KV,
  type RequestDef,
  type ResponseDef,
  type TreeItem,
} from "./api";
import { appConfirm, appPrompt, DialogHost } from "./dialogs";
import LoadPanel from "./LoadPanel";
import SettingsPanel from "./SettingsPanel";
import { useAppearance } from "./appearance";
import { Modal } from "./ui";
import VarsPanel from "./VarsPanel";

type ReqTab = "headers" | "query" | "body" | "auth";
type Panel = "" | "vars" | "load" | "curl-import" | "settings" | "code";

// Layout preferences survive restarts (px for the sidebar, fraction of the
// column for the request editor).
const savedNum = (key: string, fallback: number) => {
  const v = Number(localStorage.getItem(key));
  return Number.isFinite(v) && v > 0 ? v : fallback;
};

// A Tab is one open request buffer: its own edits, baseline (for the dirty
// dot), and response — switching tabs preserves everything, like the TUI's
// per-tab in-memory buffers and Bruno's open-request tabs.
interface OpenTab {
  id: number;
  name: string; // saved name backing the buffer, "" for an unsaved draft
  req: RequestDef;
  baseline: string;
  resp: ResponseDef | null;
}

const freshTab = (id: number, req = blankRequest(), name = ""): OpenTab => ({
  id,
  name,
  req,
  baseline: JSON.stringify(req),
  resp: null,
});

const tabDirty = (t: OpenTab) => JSON.stringify(t.req) !== t.baseline;

export default function App() {
  const [appearance, setAppearance] = useAppearance();
  const [tree, setTree] = useState<TreeItem[]>([]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [tabs, setTabs] = useState<OpenTab[]>([freshTab(1)]);
  const [active, setActive] = useState(0);
  const nextTabId = useRef(2);
  const [tab, setTab] = useState<ReqTab>("headers");
  const [sendingId, setSendingId] = useState<number | null>(null);
  const [env, setEnv] = useState<EnvState>({ active: "", names: [] });
  const [note, setNote] = useState("");
  const [panel, setPanel] = useState<Panel>("");
  const [curlText, setCurlText] = useState("");
  const [targetUrl, setTargetUrl] = useState("");
  const [sidebarW, setSidebarW] = useState(() => savedNum("volley.sidebarW", 230));
  const [treeFolded, setTreeFolded] = useState(() => localStorage.getItem("volley.treeFolded") === "on");
  const [editorFrac, setEditorFrac] = useState(() =>
    Math.min(0.75, Math.max(0.25, savedNum("volley.reqFrac", 0.48))),
  );

  const foldTree = (folded: boolean) => {
    setTreeFolded(folded);
    localStorage.setItem("volley.treeFolded", folded ? "on" : "off");
  };

  // The active tab's buffer, exposed under the names the rest of the
  // component always used.
  const activeTab = tabs[active];
  const req = activeTab.req;
  const current = activeTab.name;
  const resp = activeTab.resp;
  const sending = sendingId === activeTab.id;
  const dirty = useMemo(() => tabDirty(activeTab), [activeTab]);

  const patchTab = (i: number, p: Partial<OpenTab>) =>
    setTabs((ts) => ts.map((t, j) => (j === i ? { ...t, ...p } : t)));
  const setReq = (r: RequestDef) => patchTab(active, { req: r });

  const openTab = (t: OpenTab) => {
    setTabs((ts) => [...ts, t]);
    setActive(tabs.length); // index of the appended tab
  };

  const closeTab = async (i: number) => {
    const t = tabs[i];
    if (
      tabDirty(t) &&
      !(await appConfirm(`Close ${t.name || "this draft"}?`, { body: "The tab has unsaved edits." }))
    ) {
      return;
    }
    setTabs((ts) => {
      const next = ts.filter((_, j) => j !== i);
      return next.length > 0 ? next : [freshTab(nextTabId.current++)];
    });
    setActive((a) => Math.max(0, a > i ? a - 1 : Math.min(a, tabs.length - 2)));
  };

  const refreshTree = useCallback(() => {
    api.ListRequests().then(setTree).catch((e) => setNote(String(e)));
  }, []);

  useEffect(() => {
    refreshTree();
    api.Environments().then(setEnv).catch(() => {});
  }, [refreshTree]);

  // Transient notes fade like the TUI's status line.
  useEffect(() => {
    if (!note) return;
    const t = window.setTimeout(() => setNote(""), 6000);
    return () => window.clearTimeout(t);
  }, [note]);

  // Opening from the tree focuses an existing tab for that request, or opens
  // a new one — never clobbers another buffer (Bruno-style).
  const open = async (name: string) => {
    const existing = tabs.findIndex((t) => t.name === name);
    if (existing >= 0) {
      setActive(existing);
      return;
    }
    try {
      openTab(freshTab(nextTabId.current++, await api.LoadRequest(name), name));
      setNote("");
    } catch (e) {
      setNote(String(e));
    }
  };

  const newRequest = () => openTab(freshTab(nextTabId.current++));

  const save = async () => {
    const name =
      current ||
      (await appPrompt("Save request", { label: "Name (groups by slash)", placeholder: "auth/login" })) ||
      "";
    if (!name) return;
    try {
      await api.SaveRequest(name, req);
      patchTab(active, { name, baseline: JSON.stringify(req) });
      setNote(`saved ${name}`);
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  const send = async () => {
    if (!req.url.trim()) {
      setNote("URL is empty");
      return;
    }
    const missing = await api.Unresolved(req).catch(() => []);
    if (missing.length > 0) {
      setNote(`unresolved: ${missing.map((m) => `{{${m}}}`).join(" ")}`);
      return;
    }
    const id = activeTab.id; // the response lands on the tab it was sent from
    setSendingId(id);
    setNote("");
    try {
      const got = await api.Send(req);
      setTabs((ts) => ts.map((t) => (t.id === id ? { ...t, resp: got } : t)));
    } catch (e) {
      setNote(String(e));
    } finally {
      setSendingId((s) => (s === id ? null : s));
    }
  };

  const switchEnv = async (name: string) => {
    try {
      setEnv(await api.UseEnvironment(name));
    } catch (e) {
      setNote(String(e));
    }
  };

  const openLoadPanel = async () => {
    if (!req.url.trim()) {
      setNote("cannot load test: URL is empty");
      return;
    }
    setTargetUrl(await api.BuiltURL(req).catch(() => req.url));
    setPanel("load");
  };

  const importCurl = async () => {
    try {
      const got = await api.ImportCurl(curlText);
      // Imported requests get their own tab as an unsaved draft (baseline is
      // blank, so the dirty dot shows until saved).
      const t = freshTab(nextTabId.current++, got.request);
      openTab({ ...t, baseline: JSON.stringify(blankRequest()) });
      setPanel("");
      setCurlText("");
      setNote(got.warnings.length > 0 ? `imported with warnings: ${got.warnings.join(" · ")}` : "imported curl command");
    } catch (e) {
      setNote(String(e));
    }
  };

  const syncNow = async () => {
    const st = await api.SyncStatus();
    if (!st.configured) {
      setNote("sync is not set up — configure it in Settings");
      setPanel("settings");
      return;
    }
    setNote("syncing…");
    try {
      const report = await api.SyncNow();
      setNote(report.detail || (report.committed ? "changes committed" : "nothing to sync"));
    } catch (e) {
      setNote(String(e));
    }
  };

  // Tree CRUD, mirroring the TUI tree's m menu (add/rename/copy/delete).
  const renameItem = async (it: TreeItem) => {
    const to = await appPrompt(`Rename ${it.isDir ? "group" : "request"}`, { initial: it.name });
    if (!to || to === it.name) return;
    try {
      await (it.isDir ? api.RenameGroup(it.name, to) : api.RenameRequest(it.name, to));
      if (!it.isDir) {
        setTabs((ts) => ts.map((t) => (t.name === it.name ? { ...t, name: to } : t)));
      }
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  const copyItem = async (it: TreeItem) => {
    const to = await appPrompt("Copy request", { label: "Copy to", initial: it.name + "-copy" });
    if (!to) return;
    try {
      await api.CopyRequest(it.name, to);
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  const deleteItem = async (it: TreeItem) => {
    if (!(await appConfirm(`Delete ${it.isDir ? "group" : "request"} ${it.name}?`, { danger: true }))) return;
    try {
      await (it.isDir ? api.DeleteGroup(it.name) : api.DeleteRequest(it.name));
      // An open tab for a deleted request becomes an unsaved draft — its
      // buffer is now the only copy, so it must read as dirty.
      setTabs((ts) =>
        ts.map((t) =>
          t.name === it.name || (it.isDir && t.name.startsWith(it.name + "/"))
            ? { ...t, name: "", baseline: JSON.stringify(blankRequest()) }
            : t,
        ),
      );
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  const newGroup = async () => {
    const name = await appPrompt("New group", { label: "Group name", placeholder: "auth" });
    if (!name) return;
    try {
      await api.CreateGroup(name);
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  // Rows hidden under a collapsed group are filtered out; depth indents.
  const visibleTree = useMemo(
    () =>
      tree.filter((it) => {
        for (const c of collapsed) {
          if (it.name !== c && it.name.startsWith(c + "/")) return false;
        }
        return true;
      }),
    [tree, collapsed],
  );

  const toggleGroup = (name: string) =>
    setCollapsed((s) => {
      const next = new Set(s);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });

  // Divider drags. Pointer events on the whole window keep the drag alive
  // when the cursor outruns the 6px handle.
  const dragSidebar = (e: React.PointerEvent) => {
    e.preventDefault();
    const move = (ev: PointerEvent) => {
      const w = Math.min(Math.max(ev.clientX, 160), Math.min(500, window.innerWidth * 0.5));
      setSidebarW(w);
      localStorage.setItem("volley.sidebarW", String(w));
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  // Request/response are side-by-side; the split fraction is the request
  // column's share of the work row's width.
  const workRowRef = useRef<HTMLDivElement | null>(null);
  const dragSplit = (e: React.PointerEvent) => {
    e.preventDefault();
    const row = workRowRef.current?.getBoundingClientRect();
    if (!row) return;
    const move = (ev: PointerEvent) => {
      const frac = Math.min(0.75, Math.max(0.25, (ev.clientX - row.left) / Math.max(1, row.width)));
      setEditorFrac(frac);
      localStorage.setItem("volley.reqFrac", String(frac));
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  return (
    <div className="shell">
      {!treeFolded && (
        <aside className="sidebar" style={{ width: sidebarW, minWidth: sidebarW }}>
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">V</span>
          <span className="brand-copy">
            <b>Volley</b>
            <small>API workspace</small>
          </span>
        </div>
        <div className="tree-toolbar" role="toolbar" aria-label="collection actions">
          <button onClick={newRequest}>+ request</button>
          <button onClick={newGroup}>+ group</button>
          <button onClick={refreshTree} aria-label="reload collections from disk" title="reload from disk">
            ⟳
          </button>
          <button onClick={syncNow} aria-label="sync collections with git remote" title="sync (git)">
            ⇅
          </button>
        </div>
        <div className="tree">
          {visibleTree.map((it) => {
            const depth = it.name.split("/").length - 1;
            const leaf = it.name.split("/").pop() ?? it.name;
            return (
              <div key={it.name} className="tree-line" style={{ paddingLeft: depth * 14 }}>
                {it.isDir ? (
                  <button
                    className="tree-group"
                    aria-expanded={!collapsed.has(it.name)}
                    onClick={() => toggleGroup(it.name)}
                  >
                    <span className="twist">{collapsed.has(it.name) ? "▸" : "▾"}</span> {leaf}/
                  </button>
                ) : (
                  <button
                    className={"tree-item" + (it.name === current ? " active" : "")}
                    onClick={() => open(it.name)}
                  >
                    <span className={"method m-" + (it.method ?? "GET")}>{it.method}</span>
                    <span className="tree-name">{leaf}</span>
                  </button>
                )}
                <span className="tree-actions">
                  <button title={`rename ${it.name}`} aria-label={`rename ${it.name}`} onClick={() => renameItem(it)}>
                    ✎
                  </button>
                  {!it.isDir && (
                    <button title={`copy ${it.name}`} aria-label={`copy ${it.name}`} onClick={() => copyItem(it)}>
                      ⧉
                    </button>
                  )}
                  <button
                    title={`delete ${it.name}`}
                    aria-label={`delete ${it.name}`}
                    className="danger"
                    onClick={() => deleteItem(it)}
                  >
                    ✕
                  </button>
                </span>
              </div>
            );
          })}
          {tree.length === 0 && <div className="empty">no saved requests</div>}
        </div>
        <div className="envbox">
          <label htmlFor="env-select">environment</label>
          <select id="env-select" value={env.active} onChange={(e) => switchEnv(e.target.value)}>
            <option value="">(none)</option>
            {env.names.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
          <button className="varsbtn" onClick={() => setPanel("vars")}>
            <span aria-hidden="true">{"{·}"}</span> Variables
          </button>
        </div>
        </aside>
      )}

      <button
        className="pane-rail"
        onClick={() => foldTree(!treeFolded)}
        title={treeFolded ? "show collections" : "hide collections"}
        aria-label={treeFolded ? "show collections" : "hide collections"}
        aria-expanded={!treeFolded}
      >
        <span aria-hidden="true">{treeFolded ? "›" : "‹"}</span>
      </button>

      {!treeFolded && (
        <div
          className="divider v sidebar-resize"
          role="separator"
          aria-orientation="vertical"
          aria-label="resize sidebar"
          aria-valuemin={160}
          aria-valuemax={500}
          aria-valuenow={Math.round(sidebarW)}
          tabIndex={0}
          onPointerDown={dragSidebar}
          onKeyDown={(e) => {
            const delta = e.key === "ArrowLeft" ? -16 : e.key === "ArrowRight" ? 16 : 0;
            if (delta === 0) return;
            e.preventDefault();
            const w = Math.min(Math.max(sidebarW + delta, 160), Math.min(500, window.innerWidth * 0.5));
            setSidebarW(w);
            localStorage.setItem("volley.sidebarW", String(w));
          }}
        />
      )}

      <main className="main">
        <div className="tabstrip" role="tablist" aria-label="open requests">
          {tabs.map((t, i) => (
            <div className={"rtab" + (i === active ? " active" : "")} key={t.id}>
              <button
                className="rtab-main"
                role="tab"
                aria-selected={i === active}
                onClick={() => setActive(i)}
                title={t.name || "unsaved draft"}
              >
                <span className={"method m-" + t.req.method}>
                  {t.req.method === "DELETE" ? "DEL" : t.req.method === "OPTIONS" ? "OPT" : t.req.method}
                </span>
                <span className="rtab-name">{t.name ? t.name.split("/").pop() : "Untitled"}</span>
                {tabDirty(t) && <span className="dirty">●</span>}
              </button>
              <button
                className="rtab-x"
                aria-label={`close ${t.name || "draft"} tab`}
                onClick={() => closeTab(i)}
              >
                ×
              </button>
            </div>
          ))}
          <button className="rtab-new" onClick={newRequest} aria-label="new request tab" title="new request">
            +
          </button>
        </div>

        <div className="topbar">
          <select
            className={"method-select m-" + req.method}
            aria-label="HTTP method"
            value={req.method}
            onChange={(e) => setReq({ ...req, method: e.target.value })}
          >
            {METHODS.map((m) => (
              <option key={m}>{m}</option>
            ))}
          </select>
          <input
            className="url"
            aria-label="request URL"
            placeholder="https://api.example.com/v1/ping — {{vars}} welcome"
            value={req.url}
            onChange={(e) => setReq({ ...req, url: e.target.value })}
            onKeyDown={(e) => e.key === "Enter" && send()}
          />
          <TimeoutInput
            ms={req.timeoutMs}
            onChange={(ms) => setReq({ ...req, timeoutMs: ms })}
            onBad={() => setNote("bad duration — try 500ms, 10s, 2m")}
          />
          <button className="codebtn" onClick={() => setPanel("code")} title="generate code (curl · wget · httpie)">
            {"</>"}
          </button>
          <button className="send" onClick={send} disabled={sending}>
            {sending ? "Sending…" : "Send"}
          </button>
          <button className="test" onClick={openLoadPanel}>
            Load test
          </button>
          <button className="save" onClick={save} title={current ? `save ${current}` : "save as"}>
            Save
          </button>
          <button className="settings-button" onClick={() => setPanel("settings")} aria-label="Open settings" title="Settings">
            <SettingsIcon />
          </button>
        </div>

        <div className="workrow" ref={workRowRef}>
          <div className="req-col" style={{ flex: `0 0 ${editorFrac * 100}%` }}>
            <div className="tabs" role="tablist">
              {(["headers", "query", "body", "auth"] as ReqTab[]).map((t) => (
                <button
                  key={t}
                  role="tab"
                  aria-selected={t === tab}
                  className={t === tab ? "tab active" : "tab"}
                  onClick={() => setTab(t)}
                >
                  {t}
                </button>
              ))}
              <button className="curlbtn" onClick={() => setPanel("curl-import")}>
                import curl
              </button>
              <div className="doc">
                {current || "[No Name]"}
                {dirty && <span className="dirty"> ●</span>}
              </div>
            </div>

            <section className="editor">
              {tab === "headers" && (
                <RowsEditor
                  rows={req.headers.map((h) => ({ key: h.name, value: h.value, enabled: h.enabled }))}
                  placeholderKey="Header-Name"
                  onChange={(rows) =>
                    setReq({
                      ...req,
                      headers: rows.map((r): Header => ({ name: r.key, value: r.value, enabled: r.enabled })),
                    })
                  }
                />
              )}
              {tab === "query" && (
                <RowsEditor
                  rows={req.query}
                  placeholderKey="param"
                  onChange={(rows) => setReq({ ...req, query: rows })}
                />
              )}
              {tab === "body" && (
                <textarea
                  className="body"
                  aria-label="request body"
                  placeholder='{"raw": "request body"}'
                  value={req.body}
                  onChange={(e) => setReq({ ...req, body: e.target.value })}
                />
              )}
              {tab === "auth" && <AuthEditor req={req} onChange={setReq} />}
            </section>
          </div>

          <div
            className="divider v split"
            role="separator"
            aria-orientation="vertical"
            aria-label="resize request/response split"
            aria-valuemin={25}
            aria-valuemax={75}
            aria-valuenow={Math.round(editorFrac * 100)}
            tabIndex={0}
            onPointerDown={dragSplit}
            onKeyDown={(e) => {
              const delta = e.key === "ArrowLeft" ? -0.04 : e.key === "ArrowRight" ? 0.04 : 0;
              if (delta === 0) return;
              e.preventDefault();
              const frac = Math.min(0.75, Math.max(0.25, editorFrac + delta));
              setEditorFrac(frac);
              localStorage.setItem("volley.reqFrac", String(frac));
            }}
          />

          <ResponsePane resp={resp} sending={sending} onNote={setNote} />
        </div>
        {note && (
          <div className="note" role="status">
            {note}
          </div>
        )}
      </main>

      {panel === "code" && <CodeModal req={req} onClose={() => setPanel("")} onNote={setNote} />}
      {panel === "vars" && (
        <VarsPanel env={env} onEnvChange={setEnv} onClose={() => setPanel("")} onNote={setNote} />
      )}
      {panel === "load" && (
        <LoadPanel req={req} targetUrl={targetUrl} onClose={() => setPanel("")} onNote={setNote} />
      )}
      {panel === "settings" && (
        <SettingsPanel appearance={appearance} onChange={setAppearance} onClose={() => setPanel("")} />
      )}
      {panel === "curl-import" && (
        <Modal title="Import curl" onClose={() => setPanel("")}>
          <div className="curl-import">
            <textarea
              className="mono"
              aria-label="curl command"
              placeholder="curl -X POST https://api.example.com -H 'Content-Type: application/json' -d '…'"
              value={curlText}
              onChange={(e) => setCurlText(e.target.value)}
              spellCheck={false}
            />
            <div className="row-buttons">
              <button className="primary" onClick={importCurl}>
                import
              </button>
              <button onClick={() => setPanel("")}>cancel</button>
            </div>
          </div>
        </Modal>
      )}
      <DialogHost />
    </div>
  );
}

function SettingsIcon() {
  return (
    <svg className="settings-icon" viewBox="0 0 24 24" aria-hidden="true">
      <path d="M12 8.2a3.8 3.8 0 1 0 0 7.6 3.8 3.8 0 0 0 0-7.6Z" />
      <path d="m19.2 13.5 1.3 1-.2 1.2-1.6.5a7.7 7.7 0 0 1-1.2 1.4l.3 1.7-1 .7-1.5-.8a7.8 7.8 0 0 1-1.8.6l-.8 1.5h-1.3l-.8-1.5a7.8 7.8 0 0 1-1.8-.6l-1.5.8-1-.7.3-1.7a7.7 7.7 0 0 1-1.2-1.4l-1.6-.5-.2-1.2 1.3-1a7.8 7.8 0 0 1 0-1.9l-1.3-1 .2-1.2 1.6-.5a7.7 7.7 0 0 1 1.2-1.4l-.3-1.7 1-.7 1.5.8a7.8 7.8 0 0 1 1.8-.6l.8-1.5h1.3l.8 1.5a7.8 7.8 0 0 1 1.8.6l1.5-.8 1 .7-.3 1.7a7.7 7.7 0 0 1 1.2 1.4l1.6.5.2 1.2-1.3 1a7.8 7.8 0 0 1 0 1.9Z" />
    </svg>
  );
}

// CodeModal renders the built request as a runnable command — Bruno's
// generate-code button. Format switch regenerates through the Go side so
// vars/auth/query handling matches what Send would do.
function CodeModal({
  req,
  onClose,
  onNote,
}: {
  req: RequestDef;
  onClose: () => void;
  onNote: (s: string) => void;
}) {
  const [format, setFormat] = useState<CodeFormat>("curl");
  const [code, setCode] = useState("");

  useEffect(() => {
    api
      .GenerateCode(format, req)
      .then(setCode)
      .catch((e) => setCode(String(e)));
  }, [format, req]);

  return (
    <Modal title="Generate code" onClose={onClose}>
      <div className="code-modal">
        <div className="segmented" role="radiogroup" aria-label="code format">
          {CODE_FORMATS.map((f) => (
            <button
              key={f}
              role="radio"
              aria-checked={format === f}
              tabIndex={format === f ? 0 : -1}
              className={format === f ? "active" : ""}
              onClick={() => setFormat(f)}
            >
              {f}
            </button>
          ))}
        </div>
        <pre className="code-out mono">{code}</pre>
        <div className="row-buttons">
          <button
            className="primary"
            onClick={() =>
              navigator.clipboard
                .writeText(code)
                .then(() => onNote(`copied ${format} command`))
                .catch(() => onNote("clipboard unavailable"))
            }
          >
            ⧉ copy
          </button>
          <button onClick={onClose}>close</button>
        </div>
      </div>
    </Modal>
  );
}

function TimeoutInput({
  ms,
  onChange,
  onBad,
}: {
  ms: number;
  onChange: (ms: number) => void;
  onBad: () => void;
}) {
  const [text, setText] = useState(formatDuration(ms));
  useEffect(() => setText(formatDuration(ms)), [ms]);
  const commit = () => {
    const parsed = parseDuration(text);
    if (parsed === null) {
      onBad();
      setText(formatDuration(ms));
      return;
    }
    onChange(parsed);
  };
  return (
    <input
      className="timeout"
      placeholder="30s"
      aria-label="request timeout"
      title="request timeout (empty = default)"
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => e.key === "Enter" && commit()}
    />
  );
}

function RowsEditor({
  rows,
  placeholderKey,
  onChange,
}: {
  rows: KV[];
  placeholderKey: string;
  onChange: (rows: KV[]) => void;
}) {
  const set = (i: number, patch: Partial<KV>) =>
    onChange(rows.map((r, j) => (i === j ? { ...r, ...patch } : r)));
  return (
    <div className="rows">
      {rows.map((r, i) => (
        <div className="row" key={i}>
          <input
            type="checkbox"
            aria-label={`row ${i + 1} enabled`}
            checked={r.enabled}
            onChange={(e) => set(i, { enabled: e.target.checked })}
          />
          <input className="k" placeholder={placeholderKey} value={r.key} onChange={(e) => set(i, { key: e.target.value })} />
          <input className="v" placeholder="value" value={r.value} onChange={(e) => set(i, { value: e.target.value })} />
          <button className="del" aria-label={`delete row ${i + 1}`} onClick={() => onChange(rows.filter((_, j) => j !== i))}>
            ×
          </button>
        </div>
      ))}
      <button className="add" onClick={() => onChange([...rows, { key: "", value: "", enabled: true }])}>
        + add
      </button>
    </div>
  );
}

function AuthEditor({ req, onChange }: { req: RequestDef; onChange: (r: RequestDef) => void }) {
  const a = req.auth;
  const set = (patch: Partial<RequestDef["auth"]>) => onChange({ ...req, auth: { ...a, ...patch } });
  return (
    <div className="auth">
      <select
        aria-label="auth type"
        value={a.type}
        onChange={(e) => set({ type: e.target.value as RequestDef["auth"]["type"] })}
      >
        <option value="">no auth</option>
        <option value="bearer">bearer token</option>
        <option value="basic">basic</option>
        <option value="apikey">api key</option>
      </select>
      {a.type === "bearer" && (
        <input placeholder="token" value={a.token ?? ""} onChange={(e) => set({ token: e.target.value })} />
      )}
      {a.type === "basic" && (
        <>
          <input placeholder="username" value={a.username ?? ""} onChange={(e) => set({ username: e.target.value })} />
          <input type="password" placeholder="password" value={a.password ?? ""} onChange={(e) => set({ password: e.target.value })} />
        </>
      )}
      {a.type === "apikey" && (
        <>
          <input placeholder="key name" value={a.key ?? ""} onChange={(e) => set({ key: e.target.value })} />
          <input placeholder="value" value={a.value ?? ""} onChange={(e) => set({ value: e.target.value })} />
          <label className="inq">
            <input type="checkbox" checked={a.inQuery ?? false} onChange={(e) => set({ inQuery: e.target.checked })} />
            in query string
          </label>
        </>
      )}
    </div>
  );
}

function ResponsePane({
  resp,
  sending,
  onNote,
}: {
  resp: ResponseDef | null;
  sending: boolean;
  onNote: (s: string) => void;
}) {
  const [view, setView] = useState<"body" | "headers">("body");
  const [raw, setRaw] = useState(false);
  const pretty = useMemo(() => {
    if (!resp) return "";
    try {
      return JSON.stringify(JSON.parse(resp.body), null, 2);
    } catch {
      return resp.body;
    }
  }, [resp]);

  if (sending) return <section className="response wait">sending…</section>;
  if (!resp) return <section className="response empty">send a request to see the result here</section>;
  if (resp.error) return <section className="response err">{resp.error}</section>;

  const cls = resp.statusCode >= 500 ? "s5" : resp.statusCode >= 400 ? "s4" : resp.statusCode >= 300 ? "s3" : "s2";
  const bodyText = raw ? resp.body : pretty;
  return (
    <section className="response">
      <div className="status-line">
        <span className={"status " + cls}>{resp.status}</span>
        <span className="meta">
          {resp.durationMs} ms · {resp.size} B{resp.truncated ? " (truncated)" : ""} · {resp.proto}
        </span>
        <span className="resp-tools">
          <button className={view === "body" ? "on" : ""} onClick={() => setView("body")}>
            body
          </button>
          <button className={view === "headers" ? "on" : ""} onClick={() => setView("headers")}>
            headers
          </button>
          {view === "body" && (
            <button className={raw ? "on" : ""} onClick={() => setRaw(!raw)} title="toggle raw / pretty JSON">
              raw
            </button>
          )}
          <button
            onClick={() => {
              const text = view === "headers" ? resp.headers.map((h) => `${h.name}: ${h.value}`).join("\n") : bodyText;
              navigator.clipboard
                .writeText(text)
                .then(() => onNote("copied to clipboard"))
                .catch(() => onNote("clipboard unavailable"));
            }}
          >
            ⧉ copy
          </button>
        </span>
      </div>
      {view === "headers" ? (
        <pre className="resp-body">{resp.headers.map((h) => `${h.name}: ${h.value}`).join("\n")}</pre>
      ) : (
        <pre className="resp-body">{bodyText}</pre>
      )}
    </section>
  );
}
