// ui.tsx — shared pieces: the accessible modal overlay and the SVG charts
// the load-test views draw. Modal traps focus while open (role="dialog",
// aria-modal) and restores it on close. Charts carry axes, tick labels, a
// legend, and per-second hover tooltips.

import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { formatDuration, type ProfilePoint } from "./api";

const FOCUSABLE = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

export function Modal({
  title,
  onClose,
  children,
  wide,
  narrow,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
  narrow?: boolean;
}) {
  const box = useRef<HTMLDivElement>(null);
  const titleId = useId();

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    // Initial focus: the first focusable control, else the dialog itself.
    const first = box.current?.querySelector<HTMLElement>(FOCUSABLE);
    (first ?? box.current)?.focus();

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key !== "Tab" || !box.current) return;
      // Focus trap: Tab cycles inside the dialog.
      const items = Array.from(box.current.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        (el) => el.offsetParent !== null,
      );
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      if (e.shiftKey && (active === first || active === box.current)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      previous?.focus();
    };
  }, [onClose]);

  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div
        ref={box}
        className={"modal" + (wide ? " wide" : "") + (narrow ? " narrow" : "")}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
      >
        <div className="modal-head">
          <span id={titleId}>{title}</span>
          <button className="modal-x" aria-label="close dialog" onClick={onClose}>
            ×
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// Chart palette: violet = achieved (data series), gray = target (reference
// line), amber #D97706 = latency — validated against the dark surface with
// the dataviz six-checks script (lightness band, CVD, contrast all pass).
export const CHART_LATENCY = "#D97706";

const PAD_L = 34; // room for y tick labels
const PAD_R = 8;
const PAD_T = 8;
const PAD_B = 18; // room for x tick labels

export function niceMax(v: number): number {
  if (v <= 0) return 1;
  const mag = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * mag >= v) return m * mag;
  }
  return 10 * mag;
}

function xTicks(durationMs: number): number[] {
  if (durationMs <= 0) return [0];
  const step = niceMax(durationMs / 5);
  const out: number[] = [];
  for (let t = 0; t <= durationMs + 1; t += step) out.push(Math.min(t, durationMs));
  if (out[out.length - 1] !== durationMs) out.push(durationMs);
  return out;
}

// ShapeChart plots a profile's target line over time, with optional achieved
// completions/second bars sharing the axes. Interactive extras (point drag)
// are layered on by ShapeEditor via the children render-prop.
export function ShapeChart({
  points,
  durationMs,
  peakRps,
  bars,
  width = 560,
  height = 170,
  progress,
  showLegend,
  children,
}: {
  points: ProfilePoint[];
  durationMs: number;
  peakRps: number;
  bars?: number[];
  width?: number;
  height?: number;
  progress?: number; // 0..1 elapsed marker
  showLegend?: boolean;
  children?: (x: (ms: number) => number, y: (rps: number) => number, maxY: number) => ReactNode;
}) {
  const [hover, setHover] = useState<number | null>(null); // hovered second
  const w = width - PAD_L - PAD_R;
  const h = height - PAD_T - PAD_B;
  const maxY = niceMax(Math.max(peakRps, ...(bars ?? [0])));
  const x = (ms: number) => PAD_L + (durationMs > 0 ? (ms / durationMs) * w : 0);
  const y = (rps: number) => PAD_T + h - (rps / maxY) * h;

  const line = points.map((p, i) => `${i === 0 ? "M" : "L"}${x(p.atMs).toFixed(1)},${y(p.rps).toFixed(1)}`).join(" ");
  const secs = Math.max(1, Math.ceil(durationMs / 1000));
  const barW = Math.max(1, w / secs - 2); // 2px surface gap between bars

  return (
    <div className="chart-wrap">
      {(showLegend ?? Boolean(bars)) && (
        <div className="chart-legend" aria-hidden="true">
          <span>
            <i className="swatch bar-swatch" /> achieved/s
          </span>
          <span>
            <i className="swatch line-swatch" /> target
          </span>
        </div>
      )}
      <svg
        className="chart"
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label={`load shape, peak ${peakRps} requests per second over ${formatDuration(durationMs)}`}
        onMouseLeave={() => setHover(null)}
      >
        {/* recessive grid + axes */}
        {[0, maxY / 2, maxY].map((v) => (
          <g key={v}>
            <line className="grid" x1={PAD_L} y1={y(v)} x2={width - PAD_R} y2={y(v)} />
            <text className="tick" x={PAD_L - 4} y={y(v) + 3} textAnchor="end">
              {v}
            </text>
          </g>
        ))}
        {xTicks(durationMs).map((t) => (
          <text key={t} className="tick" x={x(t)} y={height - 5} textAnchor="middle">
            {formatDuration(t) || "0"}
          </text>
        ))}

        {bars?.map((v, i) =>
          v > 0 ? (
            <rect
              key={i}
              className={"bar" + (hover === i ? " hot" : "")}
              x={x(i * 1000) + 1}
              y={y(v)}
              width={barW}
              height={PAD_T + h - y(v)}
              rx={1.5}
              onMouseEnter={() => setHover(i)}
            >
              <title>{`${i}–${i + 1}s: ${v} completed`}</title>
            </rect>
          ) : null,
        )}
        <path className="target-line" d={line} />
        {progress !== undefined && progress < 1 && (
          <line className="progress-mark" x1={x(progress * durationMs)} y1={PAD_T} x2={x(progress * durationMs)} y2={PAD_T + h} />
        )}
        {children?.(x, y, maxY)}
      </svg>
    </div>
  );
}

// LatencyChart is the per-second mean latency strip under the main chart —
// a single series, titled by its label, so no legend box.
export function LatencyChart({
  values,
  durationMs,
  width = 560,
  height = 64,
}: {
  values: number[]; // mean latency ms per 1s bucket
  durationMs: number;
  width?: number;
  height?: number;
}) {
  const w = width - PAD_L - PAD_R;
  const h = height - PAD_T - 6;
  const secs = Math.max(1, Math.ceil(durationMs / 1000), values.length);
  const maxY = niceMax(Math.max(...values, 1));
  const pts = values
    .map((v, i) => `${(PAD_L + ((i + 0.5) / secs) * w).toFixed(1)},${(PAD_T + h - (v / maxY) * h).toFixed(1)}`)
    .join(" ");
  return (
    <svg
      className="chart latency"
      viewBox={`0 0 ${width} ${height}`}
      role="img"
      aria-label={`mean latency per second, up to ${Math.round(maxY)} milliseconds`}
    >
      <line className="grid" x1={PAD_L} y1={PAD_T} x2={width - PAD_R} y2={PAD_T} />
      <text className="tick" x={PAD_L - 4} y={PAD_T + 3} textAnchor="end">
        {maxY >= 1000 ? `${maxY / 1000}s` : `${maxY}ms`}
      </text>
      <line className="grid" x1={PAD_L} y1={PAD_T + h} x2={width - PAD_R} y2={PAD_T + h} />
      <polyline className="latency-line" points={pts}>
        <title>mean latency per second</title>
      </polyline>
    </svg>
  );
}
