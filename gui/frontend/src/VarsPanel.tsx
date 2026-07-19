// VarsPanel — session {{variable}} overrides (the TUI's :set) and named
// environment management (:env / :envnew / :envedit / :envrm), in one modal.

import { useCallback, useEffect, useState } from "react";
import { api, type EnvState } from "./api";
import { Modal } from "./ui";

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
  const [editText, setEditText] = useState("");

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

  const openEnv = async (name: string) => {
    try {
      const vals = await api.GetEnvironment(name);
      setEditing(name);
      setEditText(JSON.stringify(vals, null, 2));
    } catch (e) {
      onNote(String(e));
    }
  };

  const newEnv = async () => {
    const name = window.prompt("New environment name (e.g. staging):");
    if (!name) return;
    setEditing(name);
    setEditText(JSON.stringify({ base_url: "https://api.example.com" }, null, 2));
  };

  const saveEnv = async () => {
    if (editing === null) return;
    let vals: Record<string, string>;
    try {
      const parsed: unknown = JSON.parse(editText);
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
    try {
      onEnvChange(await api.SaveEnvironment(editing, vals));
      onNote(`saved environment ${editing} — active`);
      setEditing(null);
    } catch (e) {
      onNote(String(e));
    }
  };

  const deleteEnv = async (name: string) => {
    if (!window.confirm(`Delete environment ${name}?`)) return;
    try {
      onEnvChange(await api.DeleteEnvironment(name));
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
              <input className="v" defaultValue={v} onBlur={(e) => e.target.value !== v && setVar(k, e.target.value)} />
              <button className="del" onClick={() => setVar(k, "")}>
                ×
              </button>
            </div>
          ))}
        <div className="row">
          <input className="k" placeholder="name" value={newKey} onChange={(e) => setNewKey(e.target.value)} />
          <input
            className="v"
            placeholder="value"
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
            <button className="mini" onClick={() => openEnv(n)}>
              edit
            </button>
            <button className="mini danger" onClick={() => deleteEnv(n)}>
              delete
            </button>
          </div>
        ))}
        <button className="add" onClick={newEnv}>
          + new environment
        </button>

        {editing !== null && (
          <div className="env-edit">
            <h3>{editing}.json</h3>
            <textarea
              className="mono"
              value={editText}
              onChange={(e) => setEditText(e.target.value)}
              spellCheck={false}
            />
            <div className="row-buttons">
              <button className="primary" onClick={saveEnv}>
                save & activate
              </button>
              <button onClick={() => setEditing(null)}>cancel</button>
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
