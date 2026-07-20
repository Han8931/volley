// ShapeEditor — the GUI's answer to the TUI's :loadnew/:loadedit shape
// editor: drag points directly on the plot, or use the numeric controls for
// exact values. Constraints mirror loadtest.Profile.Validate — at least two
// points, the first pinned to 0s, times non-decreasing, rates ≥ 0 — so the
// editor can't produce a shape the engine would reject. Raw JSON stays
// available behind a toggle for hand-tuning.

import { useEffect, useRef, useState } from "react";
import { api, formatDuration, parseDuration, unitFor, type LoadMode, type Profile, type ProfilePoint } from "./api";
import { appConfirm } from "./dialogs";
import { IconReset } from "./icons";
import { useT } from "./i18n";
import { niceMax, ShapeChart } from "./ui";

const CHART_W = 560;
const CHART_H = 170;

export default function ShapeEditor({
  name,
  initial,
  onSaved,
  onCancel,
  onNote,
}: {
  name: string;
  initial: Profile | undefined; // template or existing profile; undefined = fresh constant
  onSaved: () => void;
  onCancel: () => void;
  onNote: (s: string) => void;
}) {
  const [points, setPoints] = useState<ProfilePoint[]>(
    initial?.points?.length ? initial.points.map((p) => ({ ...p })) : [{ atMs: 0, rps: 20 }, { atMs: 30000, rps: 20 }],
  );
  const t = useT();
  const [sel, setSel] = useState(0);
  const [description, setDescription] = useState(initial?.description ?? "");
  const [mode, setMode] = useState<LoadMode>(initial?.mode ?? "");
  const [thinkMs, setThinkMs] = useState(initial?.thinkTimeMs ?? 0);
  const unit = unitFor(mode);
  const [maxRequests, setMaxRequests] = useState(initial?.maxRequests ?? 0);
  const [maxWorkers, setMaxWorkers] = useState(initial?.maxWorkers ?? 0);
  const [showJSON, setShowJSON] = useState(false);
  const [jsonText, setJSONText] = useState("");
  // The shape the editor opened with — a saved profile's stored values, or
  // the starting shape for a new one. Reset restores exactly this.
  const baseline = useRef({
    points: (initial?.points?.length
      ? initial.points.map((p) => ({ ...p }))
      : [{ atMs: 0, rps: 20 }, { atMs: 30000, rps: 20 }]) as ProfilePoint[],
    description: initial?.description ?? "",
    mode: (initial?.mode ?? "") as LoadMode,
    thinkMs: initial?.thinkTimeMs ?? 0,
    maxRequests: initial?.maxRequests ?? 0,
    maxWorkers: initial?.maxWorkers ?? 0,
  });
  const svgRef = useRef<SVGSVGElement | null>(null);
  const dragging = useRef<number | null>(null);

  const durationMs = points[points.length - 1]?.atMs ?? 0;
  const peakRps = Math.max(...points.map((p) => p.rps), 1);
  // The plot's x-domain extends past the last point so the tail can be
  // dragged later — without headroom the last point maps to the right edge
  // and can never move right.
  const domainMs = durationMs + Math.max(5000, Math.round(durationMs * 0.25));

  // clampPoint keeps a moved point valid: times non-decreasing between its
  // neighbors, the first point pinned to 0s, rates non-negative.
  const clampPoint = (i: number, atMs: number, rps: number): ProfilePoint => {
    const lo = i === 0 ? 0 : points[i - 1].atMs;
    const hi = i === points.length - 1 ? Number.MAX_SAFE_INTEGER : points[i + 1].atMs;
    return {
      atMs: i === 0 ? 0 : Math.min(Math.max(Math.round(atMs), lo), hi),
      rps: Math.max(0, Math.round(rps * 10) / 10),
    };
  };

  const setPoint = (i: number, atMs: number, rps: number) =>
    setPoints((ps) => ps.map((p, j) => (j === i ? clampPoint(i, atMs, rps) : p)));

  // Pointer drag: SVG client coords → (atMs, rps) through the chart's scales.
  // The y-scale headroom (niceMax) shifts while dragging upward, which is
  // fine — the point follows the cursor on the next render.
  const onPointerMove = (e: React.PointerEvent) => {
    if (dragging.current === null || !svgRef.current) return;
    const rect = svgRef.current.getBoundingClientRect();
    const fx = (e.clientX - rect.left) / rect.width; // 0..1 across the viewBox
    const fy = (e.clientY - rect.top) / rect.height;
    const padL = 34 / CHART_W;
    const padR = 8 / CHART_W;
    const padT = 8 / CHART_H;
    const padB = 18 / CHART_H;
    const maxY = niceMax(Math.max(peakRps, 1));
    const atMs = ((fx - padL) / (1 - padL - padR)) * domainMs;
    const rps = (1 - (fy - padT) / (1 - padT - padB)) * maxY;
    setPoint(dragging.current, atMs, rps);
  };

  const addPoint = () => {
    // Insert midway between the selection and its next neighbor (or extend
    // the tail by 5s at the same rate — the TUI's `a`).
    const i = sel;
    const cur = points[i];
    const next = points[i + 1];
    const inserted: ProfilePoint = next
      ? { atMs: Math.round((cur.atMs + next.atMs) / 2), rps: Math.round(((cur.rps + next.rps) / 2) * 10) / 10 }
      : { atMs: cur.atMs + 5000, rps: cur.rps };
    const ps = [...points.slice(0, i + 1), inserted, ...points.slice(i + 1)];
    setPoints(ps);
    setSel(i + 1);
  };

  const deletePoint = () => {
    if (points.length <= 2) {
      onNote(t("shape.needTwo"));
      return;
    }
    const ps = points.filter((_, j) => j !== sel);
    if (ps[0].atMs !== 0) ps[0] = { ...ps[0], atMs: 0 };
    setPoints(ps);
    setSel(Math.max(0, sel - 1));
  };

  // Edited away from the opening shape? Reset has nothing to do otherwise.
  const changed =
    JSON.stringify({ points, description, mode, thinkMs, maxRequests, maxWorkers }) !==
    JSON.stringify(baseline.current);

  const reset = async () => {
    const ok = await appConfirm(t("shape.resetConfirm"), {
      body: t(initial ? "shape.resetSaved" : "shape.resetStart"),
      danger: true,
    });
    if (!ok) return;
    const b = baseline.current;
    setPoints(b.points.map((p) => ({ ...p })));
    setDescription(b.description);
    setMode(b.mode);
    setThinkMs(b.thinkMs);
    setMaxRequests(b.maxRequests);
    setMaxWorkers(b.maxWorkers);
    setSel(0);
    setShowJSON(false);
    onNote(t("shape.resetNote", { name }));
  };

  const save = async () => {
    if (durationMs <= 0) {
      onNote(t("shape.needDuration"));
      return;
    }
    try {
      await api.SaveProfile(name, {
        name,
        description,
        mode,
        thinkTimeMs: mode === "users" ? thinkMs : 0,
        points,
        maxRequests: maxRequests || undefined,
        maxWorkers: maxWorkers || undefined,
        peakRps: 0,
        durationMs: 0,
        planned: 0,
      });
      onNote(t("shape.saved", { name }));
      onSaved();
    } catch (e) {
      onNote(t("shape.saveFailed", { err: String(e) }));
    }
  };

  const openJSON = () => {
    setJSONText(
      JSON.stringify(
        {
          name,
          description,
          ...(mode === "users" ? { mode, thinkTime: formatDuration(thinkMs) || "0s" } : {}),
          points: points.map((p) => ({ at: formatDuration(p.atMs) || "0s", rps: p.rps })),
          ...(maxRequests ? { maxRequests } : {}),
          ...(maxWorkers ? { maxWorkers } : {}),
        },
        null,
        2,
      ),
    );
    setShowJSON(true);
  };

  const applyJSON = () => {
    try {
      const parsed = JSON.parse(jsonText) as {
        description?: string;
        mode?: LoadMode;
        thinkTime?: string;
        points?: { at?: string; rps?: number }[];
        maxRequests?: number;
        maxWorkers?: number;
      };
      const ps = (parsed.points ?? []).map((pt) => {
        const ms = parseDuration(pt.at ?? "0s");
        if (ms === null) throw new Error(`bad "at": ${pt.at}`);
        return { atMs: ms, rps: Math.max(0, pt.rps ?? 0) };
      });
      if (ps.length < 2) throw new Error("needs at least two points");
      ps.sort((a, b) => a.atMs - b.atMs);
      ps[0] = { ...ps[0], atMs: 0 };
      setPoints(ps);
      setDescription(parsed.description ?? "");
      setMode(parsed.mode === "users" ? "users" : "");
      setThinkMs(parsed.thinkTime ? (parseDuration(parsed.thinkTime) ?? 0) : 0);
      setMaxRequests(parsed.maxRequests ?? 0);
      setMaxWorkers(parsed.maxWorkers ?? 0);
      setSel(0);
      setShowJSON(false);
    } catch (e) {
      onNote(t("shape.badJSON", { err: String(e) }));
    }
  };

  const p = points[sel];
  // The engine caps planned arrivals at maxRequests (see
  // Profile.PlannedRequests) — the estimate must agree with it.
  const planned = maxRequests > 0 ? Math.min(plannedOf(points), maxRequests) : plannedOf(points);

  return (
    <div className="shape-editor">
      <div className="shape-head">
      <div className="p-meta">
        <b>{name}</b> · peak {Math.max(...points.map((x) => x.rps))} {unit.short} ·{" "}
        {formatDuration(durationMs) || "0s"}
        {mode === "users"
          ? thinkMs > 0
            ? ` · ${t("load.thinkTime", { d: formatDuration(thinkMs) })}`
            : ` · ${t("load.noThink")}`
          : ` · ${t("shape.upTo", { n: planned })}`}
      </div>
        <button
          className="mini reset-btn"
          onClick={reset}
          disabled={!changed}
          title={t(changed ? "shape.resetTitle" : "shape.resetNone")}
        >
          <IconReset size={14} /> {t("shape.reset")}
        </button>
      </div>

      <div
        className="shape-plot"
        onPointerMove={onPointerMove}
        onPointerUp={() => (dragging.current = null)}
        onPointerLeave={() => (dragging.current = null)}
      >
        <ShapeChart points={points} durationMs={durationMs} peakRps={peakRps} showLegend={false} domainMs={domainMs}>
          {(x, y) => (
            <>
              {points.map((pt, i) => (
                <circle
                  key={i}
                  className={"shape-point" + (i === sel ? " selected" : "")}
                  cx={x(pt.atMs)}
                  cy={y(pt.rps)}
                  r={i === sel ? 7 : 5}
                  ref={i === 0 ? (el) => el && (svgRef.current = el.ownerSVGElement) : undefined}
                  onPointerDown={(e) => {
                    (e.target as Element).setPointerCapture?.(e.pointerId);
                    setSel(i);
                    dragging.current = i;
                  }}
                >
                  <title>{`point ${i + 1}: ${formatDuration(pt.atMs) || "0s"} @ ${pt.rps} rps — drag to move`}</title>
                </circle>
              ))}
            </>
          )}
        </ShapeChart>
      </div>

      <div className="shape-controls">
        <fieldset>
          <legend>{t("shape.point", { n: sel + 1, total: points.length })}</legend>
          <label>
            {t("shape.time")}
            <TimeDraftInput
              key={sel} // a fresh draft per selected point
              ms={p.atMs}
              disabled={sel === 0}
              title={t(sel === 0 ? "shape.firstPinned" : "shape.timeHint")}
              onCommit={(ms) => setPoint(sel, ms, p.rps)}
              onBad={() => onNote(t("shape.timeHint"))}
            />
          </label>
          <label>
            {unit.short}
            <input
              type="number"
              min={0}
              step={1}
              value={p.rps}
              onChange={(e) => setPoint(sel, p.atMs, Number(e.target.value))}
            />
          </label>
          <button className="mini" onClick={addPoint}>
            {t("shape.addPoint")}
          </button>
          <button className="mini danger" onClick={deletePoint}>
            {t("shape.deletePoint")}
          </button>
        </fieldset>

        <fieldset>
          <legend>{t("shape.execution")}</legend>
          <label>
            {t("shape.mode")}
            <select
              className="mode-select"
              value={mode}
              title={t("shape.modeTitle")}
              onChange={(e) => setMode(e.target.value as LoadMode)}
            >
              <option value="">{t("shape.modeRate")}</option>
              <option value="users">{t("shape.modeUsers")}</option>
            </select>
          </label>
          {mode === "users" && (
            <label>
              {t("shape.think")}
              <input
                className="mono"
                value={formatDuration(thinkMs) || "0s"}
                title={t("shape.thinkTitle")}
                onChange={(e) => {
                  const ms = parseDuration(e.target.value);
                  if (ms !== null) setThinkMs(ms);
                }}
              />
            </label>
          )}
        </fieldset>

        <fieldset>
          <legend>{t("shape.limits")}</legend>
          <label>
            {t("shape.maxRequests")}
            <input
              type="number"
              min={0}
              value={maxRequests}
              title={t("shape.maxRequestsTitle")}
              onChange={(e) => setMaxRequests(Math.max(0, Number(e.target.value)))}
            />
          </label>
          <label>
            {t("shape.maxWorkers")}
            <input
              type="number"
              min={0}
              value={maxWorkers}
              title={t("shape.maxWorkersTitle")}
              onChange={(e) => setMaxWorkers(Math.max(0, Number(e.target.value)))}
            />
          </label>
          <label className="grow">
            {t("shape.description")}
            <input value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
        </fieldset>
      </div>

      {showJSON && (
        <div className="env-edit">
          <textarea className="mono" value={jsonText} onChange={(e) => setJSONText(e.target.value)} spellCheck={false} />
          <div className="row-buttons">
            <button className="primary" onClick={applyJSON}>
              {t("shape.applyJSON")}
            </button>
            <button onClick={() => setShowJSON(false)}>{t("dlg.cancel")}</button>
          </div>
        </div>
      )}

      <div className="row-buttons">
        <button className="primary" onClick={save}>
          {t("shape.save")}
        </button>
        {!showJSON && (
          <button className="mini" onClick={openJSON}>
            {t("shape.editJSON")}
          </button>
        )}
        <button onClick={onCancel}>{t("dlg.cancel")}</button>
      </div>
    </div>
  );
}

// TimeDraftInput holds the typed text as a draft and commits it on blur or
// Enter — reformatting on every keystroke would turn "5" into "5s" before
// "500ms" could be finished.
function TimeDraftInput({
  ms,
  disabled,
  title,
  onCommit,
  onBad,
}: {
  ms: number;
  disabled: boolean;
  title: string;
  onCommit: (ms: number) => void;
  onBad: () => void;
}) {
  const [draft, setDraft] = useState(formatDuration(ms) || "0s");
  const commit = () => {
    const parsed = parseDuration(draft);
    if (parsed === null) {
      onBad();
      setDraft(formatDuration(ms) || "0s");
      return;
    }
    onCommit(parsed);
  };
  // Reflect outside moves (dragging the point) while not being edited.
  const focused = useRef(false);
  useEffect(() => {
    if (!focused.current) setDraft(formatDuration(ms) || "0s");
  }, [ms]);
  return (
    <input
      className="mono"
      value={draft}
      disabled={disabled}
      title={title}
      onFocus={() => (focused.current = true)}
      onBlur={() => {
        focused.current = false;
        commit();
      }}
      onChange={(e) => setDraft(e.target.value)}
      onKeyDown={(e) => e.key === "Enter" && commit()}
    />
  );
}

// plannedOf mirrors Profile.PlannedRequests: the integral of the shape
// (trapezoids per segment), floored, for the meta line's estimate.
function plannedOf(points: ProfilePoint[]): number {
  let total = 0;
  for (let i = 1; i < points.length; i++) {
    const dt = (points[i].atMs - points[i - 1].atMs) / 1000;
    total += ((points[i - 1].rps + points[i].rps) / 2) * dt;
  }
  return Math.floor(total);
}
