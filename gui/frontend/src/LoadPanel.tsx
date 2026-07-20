// LoadPanel — shaped load testing, mirroring the TUI flow: pick a profile
// (live shape preview), confirm exactly what will be fired at which target,
// watch the run (progress, counters, charts), then read the k6-style
// analysis. Profiles are edited in the graphical ShapeEditor (raw JSON is a
// toggle inside it).

import { useCallback, useEffect, useRef, useState } from "react";
import { api, formatDuration, type LoadRun, type Profile, type RequestDef } from "./api";
import { appConfirm, appPrompt } from "./dialogs";
import ResultsView from "./ResultsView";
import ShapeEditor from "./ShapeEditor";
import { LatencyChart, Modal, ShapeChart } from "./ui";

type Stage = "picker" | "edit" | "confirm" | "run" | "results";

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
  const [editName, setEditName] = useState("");
  const [editBase, setEditBase] = useState<Profile | undefined>(undefined);
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
    if (run && !run.done && stage === "run") {
      if (!(await appConfirm("Stop the load test?", { body: "A run is still in progress." }))) return;
      await api.StopLoadTest();
    }
    if (run?.done) await api.DismissLoadTest();
    onClose();
  };

  const p = profiles[sel];

  return (
    <Modal title="Load test" onClose={close} wide>
      {stage === "picker" && (
        <div className="load-picker">
          <div className="profile-list" role="listbox" aria-label="load profiles">
            {profiles.map((pr, i) => (
              <button
                key={pr.name}
                role="option"
                aria-selected={i === sel}
                className={i === sel ? "profile active" : "profile"}
                onClick={() => setSel(i)}
              >
                <span className="p-name">{pr.name}</span>
                <span className="p-desc">{pr.description}</span>
              </button>
            ))}
            <div className="profile-actions">
              <button className="mini" onClick={() => setStage("results")} title="past runs and p99 trend">
                History
              </button>
              <button
                className="mini"
                onClick={async () => {
                  const name = await appPrompt("New load profile", {
                    label: "Profile name — starts from the selected shape",
                    placeholder: "my-spike",
                  });
                  if (!name) return;
                  if (profiles.some((x) => x.name === name)) {
                    onNote(`${name} already exists — select it and press edit`);
                    return;
                  }
                  setEditName(name);
                  setEditBase(p);
                  setStage("edit");
                }}
              >
                New
              </button>
              {p && (
                <>
                  <button
                    className="mini"
                    onClick={() => {
                      setEditName(p.name);
                      setEditBase(p);
                      setStage("edit");
                    }}
                  >
                    Edit shape
                  </button>
                  <button
                    className="mini danger push-right"
                    onClick={async () => {
                      if (await appConfirm(`Delete profile ${p.name}?`, { danger: true })) {
                        await api.DeleteProfile(p.name);
                        refresh();
                      }
                    }}
                  >
                    Delete
                  </button>
                </>
              )}
            </div>
          </div>
          <div className="profile-preview">
            {p && (
              <>
                <ShapeChart points={p.points} durationMs={p.durationMs} peakRps={p.peakRps} showLegend={false} />
                <div className="p-meta">
                  peak {p.peakRps} rps · {formatDuration(p.durationMs)} · {p.planned} req total
                  {p.maxWorkers ? ` · ≤${p.maxWorkers} workers` : ""}
                </div>
                <button className="primary" onClick={() => setStage("confirm")}>
                  Run this profile
                </button>
              </>
            )}
          </div>
        </div>
      )}

      {stage === "edit" && (
        <ShapeEditor
          name={editName}
          initial={editBase}
          onSaved={() => {
            refresh();
            setStage("picker");
          }}
          onCancel={() => setStage("picker")}
          onNote={onNote}
        />
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
              Fire
            </button>
            <button onClick={() => setStage("picker")}>Back</button>
          </div>
        </div>
      )}

      {stage === "run" && (
        <RunView
          run={run}
          onStop={() => api.StopLoadTest()}
          onRerun={rerun}
          onClose={close}
          onNote={onNote}
          onResults={() => setStage("results")}
        />
      )}

      {stage === "results" && <ResultsView onBack={() => setStage("picker")} onNote={onNote} />}
    </Modal>
  );
}

function RunView({
  run,
  onStop,
  onRerun,
  onClose,
  onNote,
  onResults,
}: {
  run: LoadRun | null;
  onStop: () => void;
  onRerun: () => void;
  onClose: () => void;
  onNote: (s: string) => void;
  onResults: () => void;
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
            ⧉ Copy analysis
          </button>
        )}
      </div>

      {!run.done && (
        <div
          className="progress"
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(frac * 100)}
        >
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
        <ShapeChart
          points={p.points}
          durationMs={p.durationMs}
          peakRps={p.peakRps}
          bars={run.buckets.map((b) => b.completed)}
          progress={run.done ? undefined : frac}
          showLegend
        />
        <div className="chart-label">latency, mean per second · max {run.maxMs.toFixed(0)}ms</div>
        <LatencyChart values={run.buckets.map((b) => b.meanLatencyMs)} durationMs={p.durationMs} />
      </div>

      {run.savedAs && <div className="hint">saved {run.savedAs}</div>}
      {run.saveError && <div className="hint err">save failed: {run.saveError}</div>}

      <div className="row-buttons">
        {run.done ? (
          <>
            <button className="primary" onClick={onRerun}>
              Run again
            </button>
            <button className="mini" onClick={onResults}>
              History
            </button>
            <button onClick={onClose}>Close</button>
          </>
        ) : (
          <button className="danger" onClick={onStop}>
            Stop
          </button>
        )}
      </div>
    </div>
  );
}
