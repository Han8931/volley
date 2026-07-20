// ResultsView — the load-test history browser: every auto-saved run,
// filterable by profile, with a p99 trend across runs, per-run deltas
// against the previous run of the same profile, and the full k6-style
// analysis for the selected run.

import { useCallback, useEffect, useMemo, useState } from "react";
import { api, formatDuration, type RunResult } from "./api";
import { appConfirm } from "./dialogs";
import { LatencyChart } from "./ui";

export default function ResultsView({
  onBack,
  onNote,
}: {
  onBack: () => void;
  onNote: (s: string) => void;
}) {
  const [results, setResults] = useState<RunResult[]>([]);
  const [profile, setProfile] = useState<string>(""); // "" = all
  const [sel, setSel] = useState(0);

  const refresh = useCallback(() => {
    api
      .ListResults()
      .then((rs) => {
        setResults(rs);
        setSel((s) => Math.min(s, Math.max(0, rs.length - 1)));
      })
      .catch((e) => onNote(String(e)));
  }, [onNote]);
  useEffect(refresh, [refresh]);

  const profiles = useMemo(() => [...new Set(results.map((r) => r.profile))].sort(), [results]);
  const filtered = useMemo(
    () => (profile === "" ? results : results.filter((r) => r.profile === profile)),
    [results, profile],
  );
  const current = filtered[Math.min(sel, filtered.length - 1)];

  // The previous run of the same profile — the natural comparison baseline.
  const previous = useMemo(() => {
    if (!current) return undefined;
    const later = results.filter(
      (r) => r.profile === current.profile && r.startedAt < current.startedAt,
    );
    return later[0]; // results are newest-first, so the first older one
  }, [results, current]);

  // Trend: p99 per run, oldest → newest. Only meaningful within ONE profile
  // — connecting runs of different shapes would imply a comparison that
  // isn't real — so it needs a profile filter.
  const trend = useMemo(
    () => (profile === "" ? [] : [...filtered].reverse().map((r) => r.p99Ms)),
    [filtered, profile],
  );

  const remove = async (r: RunResult) => {
    if (!(await appConfirm(`Delete this ${r.profile} run?`, { danger: true }))) return;
    try {
      await api.DeleteResult(r.file);
      refresh();
    } catch (e) {
      onNote(String(e));
    }
  };

  if (results.length === 0) {
    return (
      <div className="results-view">
        <p className="hint">No saved runs yet — every finished load test lands here automatically.</p>
        <div className="row-buttons">
          <button onClick={onBack}>Back</button>
        </div>
      </div>
    );
  }

  return (
    <div className="results-view">
      <div className="results-bar">
        <select aria-label="filter by profile" value={profile} onChange={(e) => { setProfile(e.target.value); setSel(0); }}>
          <option value="">all profiles</option>
          {profiles.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
        <span className="hint">
          {filtered.length} run{filtered.length === 1 ? "" : "s"}
        </span>
      </div>

      {trend.length > 1 ? (
        <>
          <div className="chart-label">
            {profile} · p99 per run (ms) · oldest → newest
          </div>
          <LatencyChart
            values={trend}
            durationMs={trend.length * 1000}
            height={54}
            label={`${profile} p99 per run`}
          />
        </>
      ) : (
        profile === "" &&
        results.length > 1 && (
          <p className="hint">Pick a profile to see its p99 trend across runs.</p>
        )
      )}

      <div className="results-split">
        <div className="results-list" role="listbox" aria-label="saved runs">
          {filtered.map((r, i) => (
            <button
              key={r.file}
              role="option"
              aria-selected={i === sel}
              className={"result-row" + (i === sel ? " active" : "")}
              onClick={() => setSel(i)}
            >
              <span className={"result-mark" + (r.errors > 0 || r.dropped > 0 ? " warn" : "")}>
                {r.stopped ? "◼" : r.errors > 0 ? "✗" : "✓"}
              </span>
              <span className="result-when">{new Date(r.startedAt).toLocaleString()}</span>
              {profile === "" && <span className="result-profile">{r.profile}</span>}
              <span className="result-meta">
                {r.achievedRps.toFixed(1)} rps · p99 {r.p99Ms.toFixed(0)}ms
                {r.errors > 0 ? ` · ${r.errorRate.toFixed(1)}% err` : ""}
              </span>
            </button>
          ))}
        </div>

        {current && (
          <div className="result-detail">
            {previous && <DeltaLine current={current} previous={previous} />}
            <pre className="summary mono">{current.text}</pre>
            <div className="row-buttons">
              <button
                className="mini"
                onClick={() =>
                  navigator.clipboard
                    .writeText(current.text)
                    .then(() => onNote("copied run analysis"))
                    .catch(() => onNote("clipboard unavailable"))
                }
              >
                ⧉ Copy
              </button>
              <button className="mini danger" onClick={() => remove(current)}>
                Delete
              </button>
            </div>
          </div>
        )}
      </div>

      <div className="row-buttons">
        <button onClick={onBack}>Back</button>
      </div>
    </div>
  );
}

// DeltaLine is the one-glance regression check: this run against the
// previous run of the same profile.
function DeltaLine({ current, previous }: { current: RunResult; previous: RunResult }) {
  const d = (cur: number, prev: number, unit: string, downIsGood: boolean) => {
    const diff = cur - prev;
    if (Math.abs(diff) < 0.05) return null;
    const good = downIsGood ? diff < 0 : diff > 0;
    return (
      <span className={good ? "delta good" : "delta bad"}>
        {diff > 0 ? "+" : ""}
        {unit === "ms" ? diff.toFixed(0) : diff.toFixed(1)}
        {unit}
      </span>
    );
  };
  return (
    <div className="delta-line">
      vs previous run ({formatDuration(Date.now() - new Date(previous.startedAt).getTime()) || "moments"} ago):
      {" p99 "}
      {d(current.p99Ms, previous.p99Ms, "ms", true) ?? <span className="delta">±0</span>}
      {" · rps "}
      {d(current.achievedRps, previous.achievedRps, "", false) ?? <span className="delta">±0</span>}
      {" · errors "}
      {d(current.errorRate, previous.errorRate, "%", true) ?? <span className="delta">±0</span>}
    </div>
  );
}
