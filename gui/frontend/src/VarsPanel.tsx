// VarsSection — session {{variable}} overrides (the TUI's :set) and named
// environment management (:env / :envnew / :envedit / :envrm). Rendered as a
// section inside Settings.
// Environment values are masked by default — they tend to hold tokens — with
// a per-row reveal; editing is key/value rows, with raw JSON as a toggle.

import { useCallback, useEffect, useState } from "react";
import { api, type EnvState } from "./api";
import { appConfirm, appPrompt } from "./dialogs";
import { IconClose, IconEye, IconEyeOff, IconPlus } from "./icons";
import { useT } from "./i18n";

// parseEnvJSON accepts only a flat {"name": "value"} object — the on-disk
// environment shape. Returns null when the text doesn't qualify.
function parseEnvJSON(text: string): Record<string, string> | null {
  try {
    const parsed: unknown = JSON.parse(text);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return null;
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(parsed)) {
      if (typeof v !== "string") return null;
      out[k] = v;
    }
    return out;
  } catch {
    return null;
  }
}

interface EnvRow {
  key: string;
  value: string;
  shown: boolean;
}

export default function VarsSection({
  env,
  onEnvChange,
  onNote,
}: {
  env: EnvState;
  onEnvChange: (st: EnvState) => void;
  onNote: (s: string) => void;
}) {
  const t = useT();
  const [session, setSession] = useState<Record<string, string>>({});
  const [revealed, setRevealed] = useState<Set<string>>(new Set()); // session rows shown in clear
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
        onNote(t("vars.duplicate", { name: k }));
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
      vals = parseEnvJSON(jsonText);
      if (vals === null) {
        onNote(t("vars.badJSON"));
        return;
      }
    } else {
      vals = rowsToVals(rows);
      if (vals === null) return;
    }
    try {
      onEnvChange(await api.SaveEnvironment(editing, vals));
      onNote(t("vars.savedEnv", { name: editing }));
      setEditing(null);
    } catch (e) {
      onNote(String(e));
    }
  };

  const deleteEnv = async (name: string) => {
    if (!(await appConfirm(t("vars.deleteEnv", { name }), { danger: true }))) return;
    try {
      onEnvChange(await api.DeleteEnvironment(name));
      if (editing === name) setEditing(null);
    } catch (e) {
      onNote(String(e));
    }
  };

  return (
    <div className="vars">
        <h3>{t("vars.session")}</h3>
        <p className="hint">{t("vars.sessionHelp")}</p>
        {Object.entries(session)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([k, v]) => (
            <div className="row" key={k}>
              <span className="k mono">{k}</span>
              <input
                className="v mono"
                type={revealed.has(k) ? "text" : "password"}
                aria-label={t("vars.value") + " " + k}
                defaultValue={v}
                onBlur={(e) => e.target.value !== v && setVar(k, e.target.value)}
              />
              <button
                className="mini"
                aria-label={t(revealed.has(k) ? "vars.hide" : "vars.reveal", { name: k })}
                title={t(revealed.has(k) ? "vars.hide" : "vars.reveal", { name: k })}
                onClick={() =>
                  setRevealed((s) => {
                    const next = new Set(s);
                    if (next.has(k)) next.delete(k);
                    else next.add(k);
                    return next;
                  })
                }
              >
                {revealed.has(k) ? <IconEyeOff size={14} /> : <IconEye size={14} />}
              </button>
              <button className="del" aria-label={t("vars.remove", { name: k })} onClick={() => setVar(k, "")}>
                <IconClose size={14} />
              </button>
            </div>
          ))}
        <div className="row">
          <input className="k" placeholder={t("vars.name")} aria-label={t("vars.name")} value={newKey} onChange={(e) => setNewKey(e.target.value)} />
          <input
            className="v"
            placeholder={t("vars.value")}
            aria-label={t("vars.value")}
            value={newVal}
            onChange={(e) => setNewVal(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addVar()}
          />
          <button className="add" onClick={addVar}>
            <IconPlus size={14} /> {t("vars.set")}
          </button>
        </div>

        <h3>{t("vars.environments")}</h3>
        <p className="hint">{t("vars.envHelp")}</p>
        {env.names.map((n) => (
          <div className="row env-row" key={n}>
            <button
              className={"env-name" + (n === env.active ? " active" : "")}
              onClick={async () => onEnvChange(await api.UseEnvironment(n === env.active ? "" : n))}
              title={t(n === env.active ? "vars.deactivate" : "vars.activate")}
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
              {t("vars.edit")}
            </button>
            <button className="mini danger" onClick={() => deleteEnv(n)}>
              {t("vars.delete")}
            </button>
          </div>
        ))}
        <button
          className="add"
          onClick={async () => {
            const name = await appPrompt(t("vars.newEnv"), {
              label: t("vars.envName"),
              placeholder: "staging",
            });
            if (name) openEnv(name, { base_url: "https://api.example.com" });
          }}
        >
          <IconPlus size={14} /> {t("vars.newEnv")}
        </button>

        {editing !== null && (
          <div className="env-edit">
            <h3>{editing}</h3>
            {showJSON ? (
              <textarea
                className="mono"
                aria-label={editing + " JSON"}
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
                      placeholder={t("vars.name")}
                      aria-label={t("vars.name")}
                      value={r.key}
                      onChange={(e) => setRows(rows.map((x, j) => (i === j ? { ...x, key: e.target.value } : x)))}
                    />
                    <input
                      className="v mono"
                      type={r.shown ? "text" : "password"}
                      placeholder={t("vars.value")}
                      aria-label={t("vars.value")}
                      value={r.value}
                      onChange={(e) => setRows(rows.map((x, j) => (i === j ? { ...x, value: e.target.value } : x)))}
                    />
                    <button
                      className="mini"
                      aria-label={t(r.shown ? "vars.hide" : "vars.reveal", { name: r.key })}
                      title={t(r.shown ? "vars.hide" : "vars.reveal", { name: r.key })}
                      onClick={() => setRows(rows.map((x, j) => (i === j ? { ...x, shown: !x.shown } : x)))}
                    >
                      {r.shown ? <IconEyeOff size={14} /> : <IconEye size={14} />}
                    </button>
                    <button
                      className="del"
                      aria-label={t("vars.remove", { name: r.key })}
                      onClick={() => setRows(rows.filter((_, j) => j !== i))}
                    >
                      <IconClose size={14} />
                    </button>
                  </div>
                ))}
                <button className="add" onClick={() => setRows([...rows, { key: "", value: "", shown: true }])}>
                  <IconPlus size={14} /> {t("vars.addVar")}
                </button>
              </div>
            )}
            <div className="row-buttons">
              <button className="primary" onClick={saveEnv}>
                {t("vars.saveActivate")}
              </button>
              <button
                className="mini"
                onClick={() => {
                  // Both directions carry the edits across, so switching
                  // views never silently drops what you just typed.
                  if (showJSON) {
                    const vals = parseEnvJSON(jsonText);
                    if (vals === null) {
                      onNote(t("vars.badJSON"));
                      return;
                    }
                    setRows(
                      Object.entries(vals)
                        .sort(([a], [b]) => a.localeCompare(b))
                        .map(([key, value]) => ({ key, value, shown: false })),
                    );
                    setShowJSON(false);
                    return;
                  }
                  const vals = rowsToVals(rows);
                  if (vals === null) return;
                  setJSONText(JSON.stringify(vals, null, 2));
                  setShowJSON(true);
                }}
              >
                {t(showJSON ? "vars.backToFields" : "vars.editJSON")}
              </button>
              <button onClick={() => setEditing(null)}>{t("dlg.cancel")}</button>
            </div>
          </div>
        )}
    </div>
  );
}
