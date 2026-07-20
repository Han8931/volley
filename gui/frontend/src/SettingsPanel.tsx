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
import { LOCALES, useI18n, useT, type Locale } from "./i18n";
import VarsSection from "./VarsPanel";
import type { EnvState } from "./api";

type SectionID = "appearance" | "interface" | "language" | "variables" | "sync";

const SECTION_IDS: SectionID[] = ["appearance", "interface", "language", "variables", "sync"];

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
  const t = useT();
  const { locale, setLocale } = useI18n();
  const [section, setSection] = useState<SectionID>(initialSection);
  const sections = SECTION_IDS.map((id) => ({ id, label: t("set." + id), blurb: t(`set.${id}Blurb`) }));
  const patch = (next: Partial<Appearance>) => onChange((current) => ({ ...current, ...next }));

  return (
    <Modal title={t("set.title")} onClose={onClose} wide>
      <div className="settings-layout">
        <nav className="settings-nav" role="tablist" aria-orientation="vertical" aria-label={t("set.sections")}>
          {sections.map((s) => (
            <button
              key={s.id}
              id={`settings-tab-${s.id}`}
              role="tab"
              aria-selected={section === s.id}
              aria-controls={`settings-panel-${s.id}`}
              tabIndex={section === s.id ? 0 : -1}
              className={"settings-navitem" + (section === s.id ? " active" : "")}
              onClick={() => setSection(s.id)}
              onKeyDown={(e) => tablistNav(e, SECTION_IDS, section, setSection)}
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
                  <h3 id="theme-heading">{t("set.theme")}</h3>
                  <p>{t("set.themeHelp")}</p>
                </div>
                <span className="setting-badge">{t("set.autosaved")}</span>
              </div>
              <div className="theme-grid" role="radiogroup" aria-label={t("set.theme")}>
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
                <p>{t("set.local")}</p>
                <button className="mini" onClick={() => onChange(DEFAULT_APPEARANCE)}>{t("set.reset")}</button>
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
                title={t("set.density")}
                description={t("set.densityHelp")}
                value={appearance.density}
                choices={[
                  ["comfortable", t("set.comfortable")],
                  ["compact", t("set.compact")],
                ]}
                onChange={(density) => patch({ density })}
              />
              <SettingChoice<CodeSize>
                title={t("set.codeSize")}
                description={t("set.codeSizeHelp")}
                value={appearance.codeSize}
                choices={[
                  ["small", t("set.small")],
                  ["medium", t("set.medium")],
                  ["large", t("set.large")],
                ]}
                onChange={(codeSize) => patch({ codeSize })}
              />
            </section>
          </div>

          <div
            id="settings-panel-language"
            role="tabpanel"
            aria-labelledby="settings-tab-language"
            hidden={section !== "language"}
          >
            <section className="settings-section">
              <div className="settings-heading">
                <div>
                  <h3>{t("set.language")}</h3>
                  <p>{t("set.langHelp")}</p>
                </div>
              </div>
              <div className="lang-grid" role="radiogroup" aria-label={t("set.language")}>
                {LOCALES.map((l) => (
                  <button
                    key={l.id}
                    role="radio"
                    aria-checked={locale === l.id}
                    tabIndex={locale === l.id ? 0 : -1}
                    className={"lang-card" + (locale === l.id ? " active" : "")}
                    onClick={() => setLocale(l.id)}
                    onKeyDown={(e) =>
                      radioNav(e, LOCALES.map((x) => x.id), locale, (id) => setLocale(id as Locale))
                    }
                  >
                    <b>{l.label}</b>
                    <small>{l.english}</small>
                  </button>
                ))}
              </div>
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
  const t = useT();
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
      setMsg(t("set.syncConfigured"));
    } catch (e) {
      setMsg(String(e));
    } finally {
      setBusy(false);
    }
  };

  const syncNow = async () => {
    setBusy(true);
    setMsg(t("note.syncing"));
    try {
      const report = await api.SyncNow();
      setMsg(report.detail || (report.committed ? t("note.committed") : t("note.nothingToSync")));
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
          <h3 id="sync-heading">{t("set.syncHeading")}</h3>
          <p>{t("set.syncHelp")}</p>
        </div>
        {st?.configured && (
          <span className="setting-badge">
            {st.branch}
            {st.dirty > 0 ? ` · ${t("set.changed", { n: st.dirty })}` : ` · ${t("set.clean")}`}
          </span>
        )}
      </div>
      {st !== null && !st.gitInstalled ? (
        <p className="hint">{t("set.syncNoGit")}</p>
      ) : (
        <>
          <div className="sync-row">
            <input
              className="mono"
              placeholder="git@github.com:you/volley-collections.git"
              aria-label={t("set.syncRemote")}
              value={remote}
              onChange={(e) => setRemote(e.target.value)}
            />
            <button className="mini" disabled={busy} onClick={setup}>
              {st?.configured ? t("set.syncUpdate") : t("set.syncSetup")}
            </button>
            <button className="primary" disabled={busy || !st?.configured} onClick={syncNow}>
              {t("set.syncNow")}
            </button>
          </div>
          <p className="hint">
            {t("set.syncSecrets")}
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
