import { useCallback, useEffect, useMemo, useState } from "react";
import {
  api,
  blankRequest,
  formatDuration,
  METHODS,
  parseDuration,
  type EnvState,
  type Header,
  type KV,
  type RequestDef,
  type ResponseDef,
  type TreeItem,
} from "./api";
import LoadPanel from "./LoadPanel";
import VarsPanel from "./VarsPanel";
import { Modal } from "./ui";

type ReqTab = "headers" | "query" | "body" | "auth";
type Panel = "" | "vars" | "load" | "curl-import";

export default function App() {
  const [tree, setTree] = useState<TreeItem[]>([]);
  const [current, setCurrent] = useState<string>(""); // saved name backing the editor
  const [req, setReq] = useState<RequestDef>(blankRequest());
  const [baseline, setBaseline] = useState<string>(JSON.stringify(blankRequest()));
  const [tab, setTab] = useState<ReqTab>("headers");
  const [resp, setResp] = useState<ResponseDef | null>(null);
  const [sending, setSending] = useState(false);
  const [env, setEnv] = useState<EnvState>({ active: "", names: [] });
  const [note, setNote] = useState("");
  const [panel, setPanel] = useState<Panel>("");
  const [curlText, setCurlText] = useState("");
  const [targetUrl, setTargetUrl] = useState("");

  const dirty = useMemo(() => JSON.stringify(req) !== baseline, [req, baseline]);

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

  const guardUnsaved = () => !dirty || window.confirm("Discard unsaved changes?");

  const adopt = (r: RequestDef, name: string) => {
    setReq(r);
    setBaseline(JSON.stringify(r));
    setCurrent(name);
    setResp(null);
  };

  const open = async (name: string) => {
    if (!guardUnsaved()) return;
    try {
      adopt(await api.LoadRequest(name), name);
      setNote("");
    } catch (e) {
      setNote(String(e));
    }
  };

  const newRequest = () => {
    if (!guardUnsaved()) return;
    adopt(blankRequest(), "");
  };

  const save = async () => {
    const name = current || window.prompt("Save as (e.g. auth/login):") || "";
    if (!name) return;
    try {
      await api.SaveRequest(name, req);
      setBaseline(JSON.stringify(req));
      setCurrent(name);
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
    setSending(true);
    setNote("");
    try {
      setResp(await api.Send(req));
    } catch (e) {
      setNote(String(e));
    } finally {
      setSending(false);
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
      if (!guardUnsaved()) return;
      setReq(got.request);
      setBaseline(JSON.stringify(blankRequest())); // imported = unsaved edits
      setCurrent("");
      setResp(null);
      setPanel("");
      setCurlText("");
      setNote(got.warnings.length > 0 ? `imported with warnings: ${got.warnings.join(" · ")}` : "imported curl command");
    } catch (e) {
      setNote(String(e));
    }
  };

  const exportCurl = async () => {
    const cmd = await api.ExportCurl(req);
    navigator.clipboard
      .writeText(cmd)
      .then(() => setNote("copied request as curl"))
      .catch(() => setNote("clipboard unavailable"));
  };

  // Tree CRUD, mirroring the tree's m menu (add/rename/copy/delete).
  const renameItem = async (it: TreeItem) => {
    const to = window.prompt(`Rename ${it.isDir ? "group" : "request"}:`, it.name);
    if (!to || to === it.name) return;
    try {
      await (it.isDir ? api.RenameGroup(it.name, to) : api.RenameRequest(it.name, to));
      if (!it.isDir && current === it.name) setCurrent(to);
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  const copyItem = async (it: TreeItem) => {
    const to = window.prompt("Copy to:", it.name + "-copy");
    if (!to) return;
    try {
      await api.CopyRequest(it.name, to);
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  const deleteItem = async (it: TreeItem) => {
    if (!window.confirm(`Delete ${it.isDir ? "group" : "request"} ${it.name}?`)) return;
    try {
      await (it.isDir ? api.DeleteGroup(it.name) : api.DeleteRequest(it.name));
      if (!it.isDir && current === it.name) setCurrent("");
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  const newGroup = async () => {
    const name = window.prompt("New group name:");
    if (!name) return;
    try {
      await api.CreateGroup(name);
      refreshTree();
    } catch (e) {
      setNote(String(e));
    }
  };

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">VOLLEY</div>
        <div className="tree-toolbar">
          <button onClick={newRequest} title="new request">
            +
          </button>
          <button onClick={newGroup} title="new group">
            +▸
          </button>
          <button onClick={refreshTree} title="reload from disk">
            ⟳
          </button>
        </div>
        <div className="tree">
          {tree.map((it) => (
            <div key={it.name} className="tree-line">
              {it.isDir ? (
                <span className="tree-group">{it.name}/</span>
              ) : (
                <button
                  className={"tree-item" + (it.name === current ? " active" : "")}
                  onClick={() => open(it.name)}
                >
                  <span className={"method m-" + (it.method ?? "GET")}>{it.method}</span>
                  <span className="tree-name">{it.name}</span>
                </button>
              )}
              <span className="tree-actions">
                <button title="rename" onClick={() => renameItem(it)}>
                  r
                </button>
                {!it.isDir && (
                  <button title="copy" onClick={() => copyItem(it)}>
                    c
                  </button>
                )}
                <button title="delete" className="danger" onClick={() => deleteItem(it)}>
                  d
                </button>
              </span>
            </div>
          ))}
          {tree.length === 0 && <div className="empty">no saved requests</div>}
        </div>
        <div className="envbox">
          <label>environment</label>
          <select value={env.active} onChange={(e) => switchEnv(e.target.value)}>
            <option value="">(none)</option>
            {env.names.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
          <button className="varsbtn" onClick={() => setPanel("vars")}>
            {"{{vars}}"}
          </button>
        </div>
      </aside>

      <main className="main">
        <div className="topbar">
          <select
            className={"method-select m-" + req.method}
            value={req.method}
            onChange={(e) => setReq({ ...req, method: e.target.value })}
          >
            {METHODS.map((m) => (
              <option key={m}>{m}</option>
            ))}
          </select>
          <input
            className="url"
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
          <button className="send" onClick={send} disabled={sending}>
            {sending ? "…" : "SEND"}
          </button>
          <button className="test" onClick={openLoadPanel}>
            TEST
          </button>
          <button className="save" onClick={save} title={current ? `save ${current}` : "save as"}>
            SAVE
          </button>
        </div>

        <div className="tabs">
          {(["headers", "query", "body", "auth"] as ReqTab[]).map((t) => (
            <button key={t} className={t === tab ? "tab active" : "tab"} onClick={() => setTab(t)}>
              {t}
            </button>
          ))}
          <button className="curlbtn" onClick={() => setPanel("curl-import")}>
            import curl
          </button>
          <button className="curlbtn" onClick={exportCurl}>
            copy as curl
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
            <RowsEditor rows={req.query} placeholderKey="param" onChange={(rows) => setReq({ ...req, query: rows })} />
          )}
          {tab === "body" && (
            <textarea
              className="body"
              placeholder='{"raw": "request body"}'
              value={req.body}
              onChange={(e) => setReq({ ...req, body: e.target.value })}
            />
          )}
          {tab === "auth" && <AuthEditor req={req} onChange={setReq} />}
        </section>

        <ResponsePane resp={resp} sending={sending} onNote={setNote} />
        {note && <div className="note">{note}</div>}
      </main>

      {panel === "vars" && (
        <VarsPanel env={env} onEnvChange={setEnv} onClose={() => setPanel("")} onNote={setNote} />
      )}
      {panel === "load" && (
        <LoadPanel req={req} targetUrl={targetUrl} onClose={() => setPanel("")} onNote={setNote} />
      )}
      {panel === "curl-import" && (
        <Modal title="Import curl" onClose={() => setPanel("")}>
          <div className="curl-import">
            <textarea
              className="mono"
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
    </div>
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
          <input type="checkbox" checked={r.enabled} onChange={(e) => set(i, { enabled: e.target.checked })} />
          <input className="k" placeholder={placeholderKey} value={r.key} onChange={(e) => set(i, { key: e.target.value })} />
          <input className="v" placeholder="value" value={r.value} onChange={(e) => set(i, { value: e.target.value })} />
          <button className="del" onClick={() => onChange(rows.filter((_, j) => j !== i))}>
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
      <select value={a.type} onChange={(e) => set({ type: e.target.value as RequestDef["auth"]["type"] })}>
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
