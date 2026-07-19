// ui.tsx — small shared pieces: the modal overlay and the SVG charts the
// load-test views draw (the GUI's answer to the TUI's sparklines).

import { type ReactNode, useEffect } from "react";
import type { ProfilePoint } from "./api";

export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={"modal" + (wide ? " wide" : "")}>
        <div className="modal-head">
          <span>{title}</span>
          <button className="modal-x" onClick={onClose}>
            ×
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// ShapeChart plots a profile's target line — the picker preview and the run
// view's backdrop. With buckets it overlays achieved completions per second
// as bars, sharing the x (time) axis.
export function ShapeChart({
  points,
  durationMs,
  peakRps,
  bars,
  width = 520,
  height = 120,
  progress,
}: {
  points: ProfilePoint[];
  durationMs: number;
  peakRps: number;
  bars?: number[]; // achieved completions per 1s bucket
  width?: number;
  height?: number;
  progress?: number; // 0..1 elapsed marker
}) {
  const pad = 6;
  const w = width - pad * 2;
  const h = height - pad * 2;
  const maxY = Math.max(peakRps, ...(bars ?? [0])) || 1;
  const x = (ms: number) => pad + (durationMs > 0 ? (ms / durationMs) * w : 0);
  const y = (rps: number) => pad + h - (rps / maxY) * h;

  const line = points.map((p, i) => `${i === 0 ? "M" : "L"}${x(p.atMs).toFixed(1)},${y(p.rps).toFixed(1)}`).join(" ");
  const secs = Math.max(1, Math.ceil(durationMs / 1000));
  const barW = Math.max(1, w / secs - 1);

  return (
    <svg className="chart" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      {bars?.map((v, i) =>
        v > 0 ? (
          <rect
            key={i}
            className="bar"
            x={x(i * 1000)}
            y={y(v)}
            width={barW}
            height={pad + h - y(v)}
          />
        ) : null,
      )}
      <path className="target-line" d={line} />
      {progress !== undefined && progress < 1 && (
        <line className="progress-mark" x1={pad + progress * w} y1={pad} x2={pad + progress * w} y2={pad + h} />
      )}
    </svg>
  );
}

// LatencyChart is the per-second mean latency strip under the main chart.
export function LatencyChart({
  values, // mean latency ms per 1s bucket
  durationMs,
  width = 520,
  height = 46,
}: {
  values: number[];
  durationMs: number;
  width?: number;
  height?: number;
}) {
  const pad = 4;
  const w = width - pad * 2;
  const h = height - pad * 2;
  const secs = Math.max(1, Math.ceil(durationMs / 1000), values.length);
  const maxY = Math.max(...values, 1);
  const pts = values
    .map((v, i) => `${(pad + ((i + 0.5) / secs) * w).toFixed(1)},${(pad + h - (v / maxY) * h).toFixed(1)}`)
    .join(" ");
  return (
    <svg className="chart latency" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      <polyline className="latency-line" points={pts} />
    </svg>
  );
}
