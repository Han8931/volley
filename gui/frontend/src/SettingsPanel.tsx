import { useEffect, useState, type Dispatch, type KeyboardEvent, type SetStateAction } from "react";
import { api, type SyncState } from "./api";
import {
  DEFAULT_APPEARANCE,
  THEMES,
  type Appearance,
  type CodeSize,
  type Density,
} from "./appearance";
import { Modal } from "./ui";

// radioNav implements the radio-group arrow-key contract: ←/↑ select the
// previous option, →/↓ the next, wrapping. Selection follows focus, per the
// WAI-ARIA radio pattern.
function radioNav<T>(e: KeyboardEvent, options: T[], current: T, select: (next: T) => void) {
  const dir =
    e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
  if (dir === 0) return;
  e.preventDefault();
  const i = options.indexOf(current);
  const next = options[(i + dir + options.length) % options.length];
  select(next);
  const group = e.currentTarget.closest('[role="radiogroup"]');
  const radios = group?.querySelectorAll<HTMLElement>('[role="radio"]');
  radios?.[(i + dir + options.length) % options.length]?.focus();
}

export default function SettingsPanel({
  appearance,
  onChange,
  onClose,
}: {
  appearance: Appearance;
  onChange: Dispatch<SetStateAction<Appearance>>;
  onClose: () => void;
}) {
  const patch = (next: Partial<Appearance>) => onChange((current) => ({ ...current, ...next }));

  return (
    <Modal title="Settings" onClose={onClose}>
      <div className="settings-panel">
        <section className="settings-section" aria-labelledby="theme-heading">
          <div className="settings-heading">
            <div>
              <h3 id="theme-heading">Color theme</h3>
              <p>Choose a palette that fits your workspace.</p>
            </div>
            <span className="setting-badge">Saved automatically</span>
          </div>
          <div className="theme-grid" role="radiogroup" aria-label="Color theme">
            {THEMES.map((theme) => (
              <button
                key={theme.id}
                className={"theme-card" + (appearance.theme === theme.id ? " active" : "")}
                role="radio"
                aria-checked={appearance.theme === theme.id}
                tabIndex={appearance.theme === theme.id ? 0 : -1}
                onClick={() => patch({ theme: theme.id })}
                onKeyDown={(e) =>
                  radioNav(e, THEMES.map((t) => t.id), appearance.theme, (theme) => patch({ theme }))
                }
              >
                <span className="theme-preview" style={{ background: theme.colors[0] }} aria-hidden="true">
                  <i style={{ background: theme.colors[1] }} />
                  <i style={{ background: theme.colors[2] }} />
                  <i style={{ background: theme.colors[1] }} />
                </span>
                <span className="theme-copy">
                  <b>{theme.name}</b>
                  <small>{theme.description}</small>
                </span>
                <span className="theme-check" aria-hidden="true">✓</span>
              </button>
            ))}
          </div>
        </section>

        <section className="settings-section split-settings">
          <SettingChoice<Density>
            title="Interface density"
            description="Adjust spacing throughout the workspace."
            value={appearance.density}
            choices={[
              ["comfortable", "Comfortable"],
              ["compact", "Compact"],
            ]}
            onChange={(density) => patch({ density })}
          />
          <SettingChoice<CodeSize>
            title="Editor text"
            description="Set the size of request and response text."
            value={appearance.codeSize}
            choices={[
              ["small", "Small"],
              ["medium", "Medium"],
              ["large", "Large"],
            ]}
            onChange={(codeSize) => patch({ codeSize })}
          />
        </section>

        <SyncSection />

        <div className="settings-footer">
          <p>Appearance settings stay on this device.</p>
          <button className="mini" onClick={() => onChange(DEFAULT_APPEARANCE)}>Reset appearance</button>
        </div>
      </div>
    </Modal>
  );
}

// SyncSection sets up and drives git-based sync of the volley config dir
// (Bruno-style). Push auth rides on the user's existing git credentials;
// environments/ (tokens) and loadresults/ are gitignored by default.
function SyncSection() {
  const [st, setSt] = useState<SyncState | null>(null);
  const [remote, setRemote] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const refresh = () =>
    api.SyncStatus().then((s) => {
      setSt(s);
      setRemote((r) => r || s.remote);
    });
  useEffect(() => {
    refresh();
  }, []);

  const setup = async () => {
    setBusy(true);
    setMsg("");
    try {
      setSt(await api.SyncSetup(remote));
      setMsg("sync configured");
    } catch (e) {
      setMsg(String(e));
    } finally {
      setBusy(false);
    }
  };

  const syncNow = async () => {
    setBusy(true);
    setMsg("syncing…");
    try {
      const report = await api.SyncNow();
      setMsg(report.detail || (report.committed ? "changes committed" : "nothing to sync"));
      refresh();
    } catch (e) {
      setMsg(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-section" aria-labelledby="sync-heading">
      <div className="settings-heading">
        <div>
          <h3 id="sync-heading">Sync (git)</h3>
          <p>Push collections and load profiles to a repo you own — GitHub, or any git remote.</p>
        </div>
        {st?.configured && (
          <span className="setting-badge">
            {st.branch}
            {st.dirty > 0 ? ` · ${st.dirty} changed` : " · clean"}
          </span>
        )}
      </div>
      {st !== null && !st.gitInstalled ? (
        <p className="hint">git is not installed (or not on PATH) — install it to enable sync.</p>
      ) : (
        <>
          <div className="sync-row">
            <input
              className="mono"
              placeholder="git@github.com:you/volley-collections.git"
              aria-label="git remote URL"
              value={remote}
              onChange={(e) => setRemote(e.target.value)}
            />
            <button className="mini" disabled={busy} onClick={setup}>
              {st?.configured ? "update remote" : "set up"}
            </button>
            <button className="primary" disabled={busy || !st?.configured} onClick={syncNow}>
              sync now
            </button>
          </div>
          <p className="hint">
            Secrets stay local: environments/ and loadresults/ are gitignored. Pushing uses your normal git
            credentials.
          </p>
        </>
      )}
      {msg && <p className="hint">{msg}</p>}
    </section>
  );
}

function SettingChoice<T extends string>({
  title,
  description,
  value,
  choices,
  onChange,
}: {
  title: string;
  description: string;
  value: T;
  choices: [T, string][];
  onChange: (value: T) => void;
}) {
  return (
    <div className="setting-choice">
      <h3>{title}</h3>
      <p>{description}</p>
      <div className="segmented" role="radiogroup" aria-label={title}>
        {choices.map(([id, label]) => (
          <button
            key={id}
            role="radio"
            aria-checked={value === id}
            tabIndex={value === id ? 0 : -1}
            className={value === id ? "active" : ""}
            onClick={() => onChange(id)}
            onKeyDown={(e) => radioNav(e, choices.map(([cid]) => cid), value, onChange)}
          >
            {label}
          </button>
        ))}
      </div>
    </div>
  );
}
