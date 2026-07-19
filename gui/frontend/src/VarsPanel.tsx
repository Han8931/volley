// VarsPanel — session {{variable}} overrides (the TUI's :set) and named
// environment management (:env / :envnew / :envedit / :envrm), in one modal.
// Environment values are masked by default — they tend to hold tokens — with
// a per-row reveal; editing is key/value rows, with raw JSON as a toggle.

import { useCallback, useEffect, useState } from "react";
import { api, type EnvState } from "./api";
import { appConfirm, appPrompt } from "./dialogs";
import { Modal } from "./ui";

interface EnvRow {
  key: string;
  value: string;
  shown: boolean;
}

export default function VarsPanel({
  env,
  onEnvChange,
  onClose,
  onNote,
}: {
  env: EnvState;
  onEnvChange: (st: EnvState) => void;
  onClose: () => void;
  onNote: (s: string) => void;
}) {
  const [session, setSession] = useState<Record<string, string>>({});
  const [newKey, setNewKey] = useState("");
  const [newVal, setNewVal] = useState("");
  const [editing, setEditing] = useState<string | null>(null); // env being edited
  const [rows, setRows] = useState<EnvRow[]>([]);
  const [showJSON, setShowJSON] = useState(false);
  const [jsonText, setJSONText] = useState("");

  const refresh = useCallback(() => {
    api.SessionVars().then(setSession).catch(() => {});
  }, []);
  useEffect(refresh, [refresh]);

  const setVar = async (k: string, v: string) => {
    await api.SetSessionVar(k, v);
    refresh();
  };

  const addVar = async () => {
    if (!newKey.trim()) return;
    await setVar(newKey.trim(), newVal);
    setNewKey("");
    setNewVal("");
  };

  const openEnv = async (name: string, vals: Record<string, string>) => {
    setEditing(name);
    setShowJSON(false);
    setRows(
      Object.entries(vals)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, value]) => ({ key, value, shown: false })),
    );
  };

  const rowsToVals = (rs: EnvRow[]): Record<string, string> | null => {
    const vals: Record<string, string> = {};
    for (const r of rs) {
      const k = r.key.trim();
      if (k === "") continue; // blank rows are simply dropped
      if (k in vals) {
        onNote(`duplicate variable name: ${k}`);
        return null;
      }
      vals[k] = r.value;
    }
    return vals;
  };

  const saveEnv = async () => {
    if (editing === null) return;
    let vals: Record<string, string> | null;
    if (showJSON) {
      try {
        const parsed: unknown = JSON.parse(jsonText);
        if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) throw new Error("not an object");
        vals = {};
        for (const [k, v] of Object.entries(parsed)) {
          if (typeof v !== "string") throw new Error(`"${k}" must be a string`);
          vals[k] = v;
        }
      } catch (e) {
        onNote(`environment JSON must be a flat {"name": "value"} object — ${String(e)}`);
        return;
      }
    } else {
      vals = rowsToVals(rows);
      if (vals === null) return;
    }
    try {
      onEnvChange(await api.SaveEnvironment(editing, vals));
      onNote(`saved environment ${editing} — active`);
      setEditing(null);
    } catch (e) {
      onNote(String(e));
    }
  };

  const deleteEnv = async (name: string) => {
    if (!(await appConfirm(`Delete environment ${name}?`, { danger: true }))) return;
    try {
      onEnvChange(await api.DeleteEnvironment(name));
      if (editing === name) setEditing(null);
    } catch (e) {
      onNote(String(e));
    }
  };

  return (
    <Modal title="Variables" onClose={onClose}>
      <div className="vars">
        <h3>session overrides</h3>
        <p className="hint">Highest precedence; gone when the app closes. Clearing a value removes it.</p>
        {Object.entries(session)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([k, v]) => (
            <div className="row" key={k}>
              <span className="k mono">{k}</span>
              <input
                className="v"
                aria-label={`value of ${k}`}
                defaultValue={v}
                onBlur={(e) => e.target.value !== v && setVar(k, e.target.value)}
              />
              <button className="del" aria-label={`remove ${k}`} onClick={() => setVar(k, "")}>
                ×
              </button>
            </div>
          ))}
        <div className="row">
          <input className="k" placeholder="name" aria-label="new variable name" value={newKey} onChange={(e) => setNewKey(e.target.value)} />
          <input
            className="v"
            placeholder="value"
            aria-label="new variable value"
            value={newVal}
            onChange={(e) => setNewVal(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addVar()}
          />
          <button className="add" onClick={addVar}>
            + set
          </button>
        </div>

        <h3>environments</h3>
        <p className="hint">Stored under the volley config dir; the active one resolves after session overrides.</p>
        {env.names.map((n) => (
          <div className="row env-row" key={n}>
            <button
              className={"env-name" + (n === env.active ? " active" : "")}
              onClick={async () => onEnvChange(await api.UseEnvironment(n === env.active ? "" : n))}
              title={n === env.active ? "click to deactivate" : "click to activate"}
            >
              {n === env.active ? `● ${n}` : n}
            </button>
            <button
              className="mini"
              onClick={async () => {
                try {
                  openEnv(n, await api.GetEnvironment(n));
                } catch (e) {
                  onNote(String(e));
                }
              }}
            >
              edit
            </button>
            <button className="mini danger" onClick={() => deleteEnv(n)}>
              delete
            </button>
          </div>
        ))}
        <button
          className="add"
          onClick={async () => {
            const name = await appPrompt("New environment", {
              label: "Environment name",
              placeholder: "staging",
            });
            if (name) openEnv(name, { base_url: "https://api.example.com" });
          }}
        >
          + new environment
        </button>

        {editing !== null && (
          <div className="env-edit">
            <h3>{editing}</h3>
            {showJSON ? (
              <textarea
                className="mono"
                aria-label={`${editing} as JSON`}
                value={jsonText}
                onChange={(e) => setJSONText(e.target.value)}
                spellCheck={false}
              />
            ) : (
              <div className="rows">
                {rows.map((r, i) => (
                  <div className="row" key={i}>
                    <input
                      className="k mono"
                      placeholder="name"
                      aria-label={`variable ${i + 1} name`}
                      value={r.key}
                      onChange={(e) => setRows(rows.map((x, j) => (i === j ? { ...x, key: e.target.value } : x)))}
                    />
                    <input
                      className="v mono"
                      type={r.shown ? "text" : "password"}
                      placeholder="value"
                      aria-label={`variable ${i + 1} value`}
                      value={r.value}
                      onChange={(e) => setRows(rows.map((x, j) => (i === j ? { ...x, value: e.target.value } : x)))}
                    />
                    <button
                      className="mini"
                      aria-label={r.shown ? "hide value" : "reveal value"}
                      title={r.shown ? "hide" : "reveal"}
                      onClick={() => setRows(rows.map((x, j) => (i === j ? { ...x, shown: !x.shown } : x)))}
                    >
                      {r.shown ? "◡" : "◉"}
                    </button>
                    <button
                      className="del"
                      aria-label={`remove row ${i + 1}`}
                      onClick={() => setRows(rows.filter((_, j) => j !== i))}
                    >
                      ×
                    </button>
                  </div>
                ))}
                <button className="add" onClick={() => setRows([...rows, { key: "", value: "", shown: true }])}>
                  + add variable
                </button>
              </div>
            )}
            <div className="row-buttons">
              <button className="primary" onClick={saveEnv}>
                save & activate
              </button>
              <button
                className="mini"
                onClick={() => {
                  if (showJSON) {
                    setShowJSON(false);
                    return;
                  }
                  const vals = rowsToVals(rows);
                  if (vals === null) return;
                  setJSONText(JSON.stringify(vals, null, 2));
                  setShowJSON(true);
                }}
              >
                {showJSON ? "back to fields" : "edit as JSON"}
              </button>
              <button onClick={() => setEditing(null)}>cancel</button>
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
