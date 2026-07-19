// LoadPanel — shaped load testing, mirroring the TUI flow: pick a profile
// (live shape preview), confirm exactly what will be fired at which target,
// watch the run (progress, counters, charts), then read the k6-style
// analysis. Profiles are editable as JSON (the :loadeditor equivalent).

import { useCallback, useEffect, useRef, useState } from "react";
import { api, formatDuration, type LoadRun, type Profile, type RequestDef } from "./api";
import { LatencyChart, Modal, ShapeChart } from "./ui";

type Stage = "picker" | "confirm" | "run";

export default function LoadPanel({
  req,
  targetUrl,
  onClose,
  onNote,
}: {
  req: RequestDef;
  targetUrl: string; // resolved preview of where requests will go
  onClose: () => void;
  onNote: (s: string) => void;
}) {
  const [stage, setStage] = useState<Stage>("picker");
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [sel, setSel] = useState(0);
  const [run, setRun] = useState<LoadRun | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [editText, setEditText] = useState("");
  const timer = useRef<number | undefined>(undefined);

  const refresh = useCallback(() => {
    api
      .ListProfiles()
      .then((ps) => {
        setProfiles(ps);
        setSel((s) => Math.min(s, Math.max(0, ps.length - 1)));
      })
      .catch((e) => onNote(String(e)));
  }, [onNote]);
  useEffect(refresh, [refresh]);

  // A run might already be live (panel re-opened): jump straight to it.
  useEffect(() => {
    api.PollLoadTest().then((st) => {
      if (st.running) {
        setRun(st);
        setStage("run");
      }
    });
  }, []);

  const poll = useCallback(() => {
    api.PollLoadTest().then((st) => {
      setRun(st);
      if (st.done && timer.current !== undefined) {
        window.clearInterval(timer.current);
        timer.current = undefined;
      }
    });
  }, []);

  useEffect(() => {
    if (stage !== "run") return;
    poll();
    timer.current = window.setInterval(poll, 400);
    return () => {
      if (timer.current !== undefined) window.clearInterval(timer.current);
      timer.current = undefined;
    };
  }, [stage, poll]);

  const start = async () => {
    const p = profiles[sel];
    if (!p) return;
    try {
      await api.StartLoadTest(p.name, req);
      setRun(null);
      setStage("run");
    } catch (e) {
      onNote(String(e));
    }
  };

  const rerun = async () => {
    await api.DismissLoadTest();
    await start();
  };

  const close = async () => {
    if (run && !run.done) {
      if (!window.confirm("A load test is running — stop it?")) return;
      await api.StopLoadTest();
    }
    if (run?.done) await api.DismissLoadTest();
    onClose();
  };

  const openEditor = (name: string, p?: Profile) => {
    const src = p ?? profiles.find((x) => x.name === name);
    const body = {
      name,
      description: src?.description ?? "",
      points: (src?.points ?? [{ atMs: 0, rps: 20 }, { atMs: 30000, rps: 20 }]).map((pt) => ({
        at: formatDuration(pt.atMs) || "0s",
        rps: pt.rps,
      })),
      ...(src?.maxRequests ? { maxRequests: src.maxRequests } : {}),
      ...(src?.maxWorkers ? { maxWorkers: src.maxWorkers } : {}),
    };
    setEditing(name);
    setEditText(JSON.stringify(body, null, 2));
  };

  const saveProfile = async () => {
    if (editing === null) return;
    try {
      const parsed = JSON.parse(editText) as {
        name?: string;
        description?: string;
        points?: { at?: string; rps?: number }[];
        maxRequests?: number;
        maxWorkers?: number;
      };
      const points = (parsed.points ?? []).map((pt) => {
        const m = /^([0-9]*\.?[0-9]+)(ms|s|m)$/.exec((pt.at ?? "0s").trim());
        if (!m) throw new Error(`bad "at": ${pt.at}`);
        const f = m[2] === "ms" ? 1 : m[2] === "s" ? 1000 : 60000;
        return { atMs: Math.round(parseFloat(m[1]) * f), rps: pt.rps ?? 0 };
      });
      await api.SaveProfile(editing, {
        name: editing,
        description: parsed.description,
        points,
        maxRequests: parsed.maxRequests,
        maxWorkers: parsed.maxWorkers,
        peakRps: 0,
        durationMs: 0,
        planned: 0,
      });
      onNote(`saved load profile ${editing}`);
      setEditing(null);
      refresh();
    } catch (e) {
      onNote(`profile save failed: ${String(e)}`);
    }
  };

  const p = profiles[sel];

  return (
    <Modal title="LOAD TEST" onClose={close} wide>
      {stage === "picker" && (
        <div className="load-picker">
          <div className="profile-list">
            {profiles.map((pr, i) => (
              <button key={pr.name} className={i === sel ? "profile active" : "profile"} onClick={() => setSel(i)}>
                <span className="p-name">{pr.name}</span>
                <span className="p-desc">{pr.description}</span>
              </button>
            ))}
            <div className="row-buttons">
              <button
                className="mini"
                onClick={() => {
                  const name = window.prompt("New profile name:", "");
                  if (name) openEditor(name, p); // start from the selection, like :loadnew <name> <template>
                }}
              >
                + new
              </button>
              {p && (
                <>
                  <button className="mini" onClick={() => openEditor(p.name)}>
                    edit JSON
                  </button>
                  <button
                    className="mini danger"
                    onClick={async () => {
                      if (window.confirm(`Delete profile ${p.name}?`)) {
                        await api.DeleteProfile(p.name);
                        refresh();
                      }
                    }}
                  >
                    delete
                  </button>
                </>
              )}
            </div>
          </div>
          <div className="profile-preview">
            {p && (
              <>
                <ShapeChart points={p.points} durationMs={p.durationMs} peakRps={p.peakRps} />
                <div className="p-meta">
                  peak {p.peakRps} rps · {formatDuration(p.durationMs)} · {p.planned} req total
                  {p.maxWorkers ? ` · ≤${p.maxWorkers} workers` : ""}
                </div>
                <button className="primary" onClick={() => setStage("confirm")}>
                  run against the current request
                </button>
              </>
            )}
          </div>
        </div>
      )}

      {stage === "confirm" && p && (
        <div className="load-confirm">
          <p>
            Run <b>{p.name}</b> — peak <b>{p.peakRps} rps</b>, up to <b>{p.planned}</b> requests over{" "}
            <b>{formatDuration(p.durationMs)}</b>
          </p>
          <p className="target">
            against <span className="mono">{req.method} {targetUrl}</span>
          </p>
          <p className="hint">A spike aimed at the wrong URL is the classic load-testing footgun.</p>
          <div className="row-buttons">
            <button className="primary" onClick={start}>
              fire
            </button>
            <button onClick={() => setStage("picker")}>back</button>
          </div>
        </div>
      )}

      {stage === "run" && <RunView run={run} onStop={() => api.StopLoadTest()} onRerun={rerun} onClose={close} onNote={onNote} />}

      {editing !== null && (
        <div className="env-edit">
          <h3>{editing}.json</h3>
          <textarea className="mono" value={editText} onChange={(e) => setEditText(e.target.value)} spellCheck={false} />
          <div className="row-buttons">
            <button className="primary" onClick={saveProfile}>
              save
            </button>
            <button onClick={() => setEditing(null)}>cancel</button>
          </div>
        </div>
      )}
    </Modal>
  );
}

function RunView({
  run,
  onStop,
  onRerun,
  onClose,
  onNote,
}: {
  run: LoadRun | null;
  onStop: () => void;
  onRerun: () => void;
  onClose: () => void;
  onNote: (s: string) => void;
}) {
  if (!run || !run.running) return <div className="load-run">starting…</div>;

  const p = run.profile;
  const frac = p.durationMs > 0 ? Math.min(1, run.elapsedMs / p.durationMs) : 0;
  const stateLabel = run.done ? (run.stopped ? "stopped" : "done") : "running";

  return (
    <div className="load-run">
      <div className="run-head">
        <span className="p-name">{p.name}</span>
        <span className={"run-state " + stateLabel}>{stateLabel}</span>
        <span className="p-meta">
          {formatDuration(run.elapsedMs) || "0s"} / {formatDuration(p.durationMs)}
        </span>
        {run.done && run.summaryText && (
          <button
            className="mini"
            onClick={() => {
              navigator.clipboard
                .writeText(run.summaryText!)
                .then(() => onNote("copied run analysis to clipboard"))
                .catch(() => onNote("clipboard unavailable"));
            }}
          >
            ⧉ copy analysis
          </button>
        )}
      </div>

      {!run.done && (
        <div className="progress">
          <div className="progress-fill" style={{ width: `${frac * 100}%` }} />
        </div>
      )}

      {run.done && run.summaryText ? (
        <pre className="summary mono">{run.summaryText}</pre>
      ) : (
        <div className="counters">
          <span>
            ok <b className="ok">{run.ok}</b>
          </span>
          <span>
            err <b className={run.errors > 0 ? "err" : ""}>{run.errors}</b>
          </span>
          <span>
            cancel <b>{run.canceled}</b>
          </span>
          <span>
            drop <b className={run.dropped > 0 ? "warn" : ""}>{run.dropped}</b>
          </span>
          <span>
            in-flight <b>{run.inFlight}/{run.maxWorkers}</b>
          </span>
          <span>
            rps <b>{run.achievedRps.toFixed(1)}</b> achieved · <b>{run.targetNowRps.toFixed(1)}</b> target now
          </span>
          <span>
            p50 <b>{run.p50Ms.toFixed(0)}ms</b> · p95 <b>{run.p95Ms.toFixed(0)}ms</b> · p99{" "}
            <b>{run.p99Ms.toFixed(0)}ms</b> · max <b>{run.maxMs.toFixed(0)}ms</b>
          </span>
        </div>
      )}

      <div className="chart-block">
        <div className="chart-label">target ─ vs achieved ▮ (per second)</div>
        <ShapeChart
          points={p.points}
          durationMs={p.durationMs}
          peakRps={p.peakRps}
          bars={run.buckets.map((b) => b.completed)}
          progress={run.done ? undefined : frac}
        />
        <div className="chart-label">latency (mean/s) · max {run.maxMs.toFixed(0)}ms</div>
        <LatencyChart values={run.buckets.map((b) => b.meanLatencyMs)} durationMs={p.durationMs} />
      </div>

      {run.savedAs && <div className="hint">saved {run.savedAs}</div>}
      {run.saveError && <div className="hint err">save failed: {run.saveError}</div>}

      <div className="row-buttons">
        {run.done ? (
          <>
            <button className="primary" onClick={onRerun}>
              run again
            </button>
            <button onClick={onClose}>close</button>
          </>
        ) : (
          <button className="danger" onClick={onStop}>
            stop
          </button>
        )}
      </div>
    </div>
  );
}
