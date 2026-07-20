// api.ts — a thin, hand-typed bridge to the Go bindings. Wails injects
// window.go.main.App at runtime; typing it here keeps the frontend build
// independent of the wails-generated wailsjs/ directory.

export interface Header {
  name: string;
  value: string;
  enabled: boolean;
}

export interface KV {
  key: string;
  value: string;
  enabled: boolean;
}

export interface Auth {
  type: "" | "bearer" | "basic" | "apikey";
  token?: string;
  username?: string;
  password?: string;
  key?: string;
  value?: string;
  inQuery?: boolean;
}

export interface RequestDef {
  method: string;
  url: string;
  headers: Header[];
  query: KV[];
  body: string;
  auth: Auth;
  timeoutMs: number; // 0 = engine default
}

export interface ResponseDef {
  status: string;
  statusCode: number;
  proto: string;
  headers: Header[];
  body: string;
  durationMs: number;
  size: number;
  truncated: boolean;
  error?: string;
  finalUrl: string;
}

export interface TreeItem {
  name: string; // slash-separated, e.g. "auth/login"
  isDir: boolean;
  method?: string;
}

export interface EnvState {
  active: string; // "" = none
  names: string[];
}

export interface ProfilePoint {
  atMs: number;
  rps: number;
}

export interface Profile {
  name: string;
  description?: string;
  points: ProfilePoint[];
  maxRequests?: number;
  maxWorkers?: number;
  peakRps: number;
  durationMs: number;
  planned: number;
}

export interface Bucket {
  completed: number;
  meanLatencyMs: number;
}

export interface LoadRun {
  running: boolean;
  done: boolean;
  profile: Profile;
  elapsedMs: number;
  sent: number;
  completed: number;
  ok: number;
  errors: number;
  canceled: number;
  dropped: number;
  inFlight: number;
  maxWorkers: number;
  achievedRps: number;
  targetNowRps: number;
  p50Ms: number;
  p90Ms: number;
  p95Ms: number;
  p99Ms: number;
  maxMs: number;
  meanMs: number;
  buckets: Bucket[];
  stopped: boolean;
  summaryText?: string;
  savedAs?: string;
  saveError?: string;
}

export interface CurlImport {
  request: RequestDef;
  warnings: string[];
}

export type CodeFormat = "curl" | "wget" | "httpie";
export const CODE_FORMATS: CodeFormat[] = ["curl", "wget", "httpie"];

export interface SyncState {
  gitInstalled: boolean;
  configured: boolean;
  remote: string;
  branch: string;
  dirty: number;
  root: string;
}

export interface SyncReport {
  committed: boolean;
  pushed: boolean;
  detail: string;
}

export interface RunResult {
  file: string;
  profile: string;
  method: string;
  url: string;
  startedAt: string; // RFC 3339
  elapsedMs: number;
  stopped: boolean;
  planned: number;
  sent: number;
  completed: number;
  ok: number;
  errors: number;
  canceled: number;
  dropped: number;
  peakRps: number;
  achievedRps: number;
  errorRate: number; // percent
  p50Ms: number;
  p90Ms: number;
  p95Ms: number;
  p99Ms: number;
  maxMs: number;
  text: string; // rendered k6-style analysis
}

interface Bindings {
  Send(req: RequestDef): Promise<ResponseDef>;
  Unresolved(req: RequestDef): Promise<string[]>;
  BuiltURL(req: RequestDef): Promise<string>;
  ListRequests(): Promise<TreeItem[]>;
  LoadRequest(name: string): Promise<RequestDef>;
  SaveRequest(name: string, req: RequestDef): Promise<void>;
  DeleteRequest(name: string): Promise<void>;
  RenameRequest(oldName: string, newName: string): Promise<void>;
  CopyRequest(oldName: string, newName: string): Promise<void>;
  CreateGroup(name: string): Promise<void>;
  DeleteGroup(name: string): Promise<void>;
  RenameGroup(oldName: string, newName: string): Promise<void>;
  ImportCurl(cmd: string): Promise<CurlImport>;
  ExportCurl(req: RequestDef): Promise<string>;
  GenerateCode(format: CodeFormat, req: RequestDef): Promise<string>;
  SyncStatus(): Promise<SyncState>;
  SyncSetup(remote: string): Promise<SyncState>;
  SyncNow(): Promise<SyncReport>;
  SessionVars(): Promise<Record<string, string>>;
  SetSessionVar(name: string, value: string): Promise<void>;
  Environments(): Promise<EnvState>;
  UseEnvironment(name: string): Promise<EnvState>;
  GetEnvironment(name: string): Promise<Record<string, string>>;
  SaveEnvironment(name: string, vals: Record<string, string>): Promise<EnvState>;
  DeleteEnvironment(name: string): Promise<EnvState>;
  ListProfiles(): Promise<Profile[]>;
  SaveProfile(name: string, p: Profile): Promise<void>;
  DeleteProfile(name: string): Promise<void>;
  ListResults(): Promise<RunResult[]>;
  DeleteResult(file: string): Promise<void>;
  StartLoadTest(profileName: string, req: RequestDef): Promise<void>;
  StopLoadTest(): Promise<void>;
  DismissLoadTest(): Promise<void>;
  PollLoadTest(): Promise<LoadRun>;
}

declare global {
  interface Window {
    go: { main: { App: Bindings } };
  }
}

export const api: Bindings = new Proxy({} as Bindings, {
  get(_t, prop: string) {
    // Resolved per call so a hot-reloaded frontend finds the live runtime.
    return (...args: unknown[]) =>
      (window.go.main.App as unknown as Record<string, (...a: unknown[]) => unknown>)[prop](...args);
  },
});

export const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];

export function blankRequest(): RequestDef {
  return {
    method: "GET",
    url: "",
    headers: [],
    query: [],
    body: "",
    auth: { type: "" },
    timeoutMs: 0,
  };
}

// parseDuration turns "10s" / "500ms" / "2m" / "1.5s" into milliseconds
// (null = unparseable). Bare numbers are seconds, matching what people type.
export function parseDuration(s: string): number | null {
  const t = s.trim().toLowerCase();
  if (t === "") return 0;
  const m = /^([0-9]*\.?[0-9]+)\s*(ms|s|m|h)?$/.exec(t);
  if (!m) return null;
  const n = parseFloat(m[1]);
  const unit = m[2] ?? "s";
  const factor = unit === "ms" ? 1 : unit === "s" ? 1000 : unit === "m" ? 60000 : 3600000;
  return Math.round(n * factor);
}

// formatDuration renders milliseconds the way the TUI prints durations.
export function formatDuration(ms: number): string {
  if (ms <= 0) return "";
  if (ms % 60000 === 0) return `${ms / 60000}m`;
  if (ms % 1000 === 0) return `${ms / 1000}s`;
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}
