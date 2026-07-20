// dialogs.tsx — in-app replacements for window.prompt / window.confirm, so
// every flow uses styled, accessible application dialogs. Call appPrompt /
// appConfirm from anywhere; DialogHost (mounted once in App) renders them.

import { useEffect, useRef, useState } from "react";
import { useT } from "./i18n";
import { Modal } from "./ui";

interface PromptReq {
  kind: "prompt";
  id: number;
  title: string;
  label?: string;
  initial?: string;
  placeholder?: string;
  resolve: (v: string | null) => void;
}

interface ConfirmReq {
  kind: "confirm";
  id: number;
  title: string;
  body?: string;
  danger?: boolean;
  resolve: (ok: boolean) => void;
}

type Req = PromptReq | ConfirmReq;

let enqueue: ((r: Omit<Req, "id">) => void) | null = null;

export function appPrompt(
  title: string,
  opts: { label?: string; initial?: string; placeholder?: string } = {},
): Promise<string | null> {
  return new Promise((resolve) => {
    if (!enqueue) return resolve(window.prompt(title, opts.initial ?? "")); // host not mounted: degrade
    enqueue({ kind: "prompt", title, resolve, ...opts });
  });
}

export function appConfirm(title: string, opts: { body?: string; danger?: boolean } = {}): Promise<boolean> {
  return new Promise((resolve) => {
    if (!enqueue) return resolve(window.confirm(title));
    enqueue({ kind: "confirm", title, resolve, ...opts });
  });
}

export function DialogHost() {
  const [queue, setQueue] = useState<Req[]>([]);
  useEffect(() => {
    let nextId = 1;
    // A stable per-request id keys the rendered dialog — keying by queue
    // length would remount (and wipe) the open prompt when another request
    // queues up behind it.
    enqueue = (r) => setQueue((q) => [...q, { ...r, id: nextId++ } as Req]);
    return () => {
      enqueue = null;
    };
  }, []);

  const req = queue[0];
  if (!req) return null;
  const done = () => setQueue((q) => q.slice(1));

  if (req.kind === "confirm") {
    return <ConfirmView key={req.id} req={req} done={done} />;
  }
  return <PromptView key={req.id} req={req} done={done} />;
}

function ConfirmView({ req, done }: { req: ConfirmReq; done: () => void }) {
  const t = useT();
  const answer = (ok: boolean) => {
    req.resolve(ok);
    done();
  };
  return (
    <Modal title={req.title} onClose={() => answer(false)} narrow>
      {req.body && <p className="dialog-body">{req.body}</p>}
      <div className="row-buttons">
        <button className={req.danger ? "danger-solid" : "primary"} autoFocus onClick={() => answer(true)}>
          {req.danger ? t("dlg.delete") : t("dlg.ok")}
        </button>
        <button onClick={() => answer(false)}>{t("dlg.cancel")}</button>
      </div>
    </Modal>
  );
}

function PromptView({ req, done }: { req: PromptReq; done: () => void }) {
  const t = useT();
  const [value, setValue] = useState(req.initial ?? "");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => input.current?.select(), []);
  const answer = (v: string | null) => {
    req.resolve(v);
    done();
  };
  return (
    <Modal title={req.title} onClose={() => answer(null)} narrow>
      {req.label && (
        <label className="dialog-body" htmlFor="dialog-input">
          {req.label}
        </label>
      )}
      <input
        id="dialog-input"
        ref={input}
        className="dialog-input mono"
        autoFocus
        value={value}
        placeholder={req.placeholder}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && answer(value)}
      />
      <div className="row-buttons">
        <button className="primary" onClick={() => answer(value)}>
          {t("dlg.ok")}
        </button>
        <button onClick={() => answer(null)}>{t("dlg.cancel")}</button>
      </div>
    </Modal>
  );
}
