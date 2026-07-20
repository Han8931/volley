// highlight.tsx — JSON syntax highlighting for the request body editor and
// the response viewer, matching what the TUI colors.
//
// The tokenizer is lexical, not a parser: half-typed JSON must still color
// sensibly while you edit. Token colors reuse the theme's own palette
// (accent/ok/info/warn/dim), so highlighting follows the selected theme with
// no extra per-theme tokens.

import { Fragment, useLayoutEffect, useRef, type ReactNode } from "react";

// One regex, alternation ordered so a quoted string followed by ":" wins as
// a key before the plain-string branch can claim it.
const TOKEN =
  /("(?:[^"\\]|\\.)*"\s*:)|("(?:[^"\\]|\\.)*")|(\btrue\b|\bfalse\b|\bnull\b)|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|([{}[\],:])/g;

// looksJSON gates highlighting on the shape people actually paste into a
// body — an object or array. Plain text and XML stay unstyled rather than
// getting speckled by the JSON rules.
export function looksJSON(text: string): boolean {
  const t = text.trimStart();
  return t.startsWith("{") || t.startsWith("[");
}

export function highlightJSON(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let m: RegExpExecArray | null;
  TOKEN.lastIndex = 0;
  while ((m = TOKEN.exec(text)) !== null) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const [tok, key, str, lit, num, punct] = m;
    const cls = key ? "t-key" : str ? "t-str" : lit ? "t-lit" : num ? "t-num" : punct ? "t-punct" : "";
    out.push(
      <span className={cls} key={m.index}>
        {tok}
      </span>,
    );
    last = m.index + tok.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

// Highlighted renders read-only text (the response body).
export function Highlighted({ text, className }: { text: string; className?: string }) {
  return <pre className={className}>{looksJSON(text) ? highlightJSON(text) : text}</pre>;
}

// CodeArea is an editable textarea with a highlighted layer painted behind
// it: the textarea's own text is transparent (its caret and selection are
// not), and the two layers share font metrics and padding so glyphs line up
// exactly. Scrolling is mirrored onto the backdrop.
export function CodeArea({
  value,
  onChange,
  placeholder,
  ariaLabel,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  ariaLabel?: string;
}) {
  const back = useRef<HTMLPreElement>(null);
  const area = useRef<HTMLTextAreaElement>(null);

  // Keep the backdrop aligned when the value changes under us (loading a
  // saved request, curl import) — not just on user scroll.
  useLayoutEffect(() => {
    if (back.current && area.current) {
      back.current.scrollTop = area.current.scrollTop;
      back.current.scrollLeft = area.current.scrollLeft;
    }
  }, [value]);

  const sync = () => {
    if (back.current && area.current) {
      back.current.scrollTop = area.current.scrollTop;
      back.current.scrollLeft = area.current.scrollLeft;
    }
  };

  return (
    <div className="code-area">
      <pre className="code-hl" ref={back} aria-hidden="true">
        {looksJSON(value) ? highlightJSON(value) : value}
        {/* a trailing newline keeps the last line visible while scrolled */}
        <Fragment>{"\n"}</Fragment>
      </pre>
      <textarea
        ref={area}
        className="body code-input"
        aria-label={ariaLabel}
        placeholder={placeholder}
        value={value}
        spellCheck={false}
        onScroll={sync}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}
