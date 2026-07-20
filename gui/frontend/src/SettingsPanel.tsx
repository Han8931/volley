// SettingsPanel — configuration grouped into sections behind a left-hand
// nav, so the dialog shows one topic at a time instead of one long scroll.

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
import VarsSection from "./VarsPanel";
import type { EnvState } from "./api";

type SectionID = "appearance" | "interface" | "variables" | "sync";

const SECTIONS: { id: SectionID; label: string; blurb: string }[] = [
  { id: "appearance", label: "Appearance", blurb: "Color theme" },
  { id: "interface", label: "Interface", blurb: "Density and text size" },
  { id: "variables", label: "Variables", blurb: "Environments and overrides" },
  { id: "sync", label: "Sync", blurb: "Git remote" },
];

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

// tablistNav gives a tablist its expected keyboard contract: arrows move
// (and select) along the list, Home/End jump to the ends, wrapping.
function tablistNav<T>(e: KeyboardEvent, ids: T[], current: T, select: (next: T) => void) {
  const i = ids.indexOf(current);
  let next = i;
  if (e.key === "ArrowDown" || e.key === "ArrowRight") next = (i + 1) % ids.length;
  else if (e.key === "ArrowUp" || e.key === "ArrowLeft") next = (i - 1 + ids.length) % ids.length;
  else if (e.key === "Home") next = 0;
  else if (e.key === "End") next = ids.length - 1;
  else return;
  e.preventDefault();
  select(ids[next]);
  const tabs = e.currentTarget.closest('[role="tablist"]')?.querySelectorAll<HTMLElement>('[role="tab"]');
  tabs?.[next]?.focus();
}

export default function SettingsPanel({
  appearance,
  onChange,
  onClose,
  env,
  onEnvChange,
  onNote,
  initialSection = "appearance",
}: {
  appearance: Appearance;
  onChange: Dispatch<SetStateAction<Appearance>>;
  onClose: () => void;
  env: EnvState;
  onEnvChange: (st: EnvState) => void;
  onNote: (s: string) => void;
  initialSection?: SectionID;
}) {
  const [section, setSection] = useState<SectionID>(initialSection);
  const patch = (next: Partial<Appearance>) => onChange((current) => ({ ...current, ...next }));

  return (
    <Modal title="Settings" onClose={onClose} wide>
      <div className="settings-layout">
        <nav className="settings-nav" role="tablist" aria-orientation="vertical" aria-label="settings sections">
          {SECTIONS.map((s) => (
            <button
              key={s.id}
              id={`settings-tab-${s.id}`}
              role="tab"
              aria-selected={section === s.id}
              aria-controls={`settings-panel-${s.id}`}
              tabIndex={section === s.id ? 0 : -1}
              className={"settings-navitem" + (section === s.id ? " active" : "")}
              onClick={() => setSection(s.id)}
              onKeyDown={(e) => tablistNav(e, SECTIONS.map((x) => x.id), section, setSection)}
            >
              <b>{s.label}</b>
              <small>{s.blurb}</small>
            </button>
          ))}
        </nav>

        {/* Every panel stays mounted — unmounting would throw away
            half-finished environment rows or JSON when you navigate away. */}
        <div className="settings-body">
          <div
            id="settings-panel-appearance"
            role="tabpanel"
            aria-labelledby="settings-tab-appearance"
            hidden={section !== "appearance"}
          >
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
              <div className="settings-footer">
                <p>Appearance settings stay on this device.</p>
                <button className="mini" onClick={() => onChange(DEFAULT_APPEARANCE)}>Reset appearance</button>
              </div>
            </section>
          </div>

          <div
            id="settings-panel-interface"
            role="tabpanel"
            aria-labelledby="settings-tab-interface"
            hidden={section !== "interface"}
          >
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
          </div>

          <div
            id="settings-panel-variables"
            role="tabpanel"
            aria-labelledby="settings-tab-variables"
            hidden={section !== "variables"}
          >
            <VarsSection env={env} onEnvChange={onEnvChange} onNote={onNote} />
          </div>

          <div
            id="settings-panel-sync"
            role="tabpanel"
            aria-labelledby="settings-tab-sync"
            hidden={section !== "sync"}
          >
            <SyncSection />
          </div>
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
              {st?.configured ? "Update remote" : "Set up"}
            </button>
            <button className="primary" disabled={busy || !st?.configured} onClick={syncNow}>
              Sync now
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
