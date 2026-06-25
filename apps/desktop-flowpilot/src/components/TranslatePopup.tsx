import { useEffect, useRef, useState } from "react";
import { RUNNER_URL } from "@/config";

interface TranslateResult {
  translatedText: string;
  source: string;
  target: string;
}

function runnerFetch(path: string): Promise<Response> {
  return fetch(new URL(path, RUNNER_URL).toString(), { cache: "no-store" });
}

interface Props {
  containerRef: React.RefObject<HTMLDivElement | null>;
}

export function TranslatePopup({ containerRef }: Props): React.ReactElement | null {
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);
  const [pending, setPending] = useState("");
  const [result, setResult] = useState<TranslateResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

  const dismiss = () => {
    setAnchor(null);
    setResult(null);
    setError(null);
    setPending("");
  };

  useEffect(() => {
    const onMouseUp = () => {
      window.setTimeout(() => {
        const sel = window.getSelection();
        if (!sel || sel.isCollapsed) return;
        const text = sel.toString().trim();
        if (!text) return;
        const container = containerRef.current;
        if (!container) return;
        const range = sel.getRangeAt(0);
        if (!container.contains(range.commonAncestorContainer)) return;
        const rect = range.getBoundingClientRect();
        setPending(text);
        setAnchor({ x: rect.left + rect.width / 2, y: rect.bottom + 8 });
        setResult(null);
        setError(null);
      }, 10);
    };

    const onMouseDown = (e: MouseEvent) => {
      if (popoverRef.current && popoverRef.current.contains(e.target as Node)) return;
      dismiss();
    };

    document.addEventListener("mouseup", onMouseUp);
    document.addEventListener("mousedown", onMouseDown);
    return () => {
      document.removeEventListener("mouseup", onMouseUp);
      document.removeEventListener("mousedown", onMouseDown);
    };
  }, [containerRef]);

  const translate = async () => {
    if (!pending) return;
    setLoading(true);
    setError(null);
    try {
      const resp = await runnerFetch(`/translate?q=${encodeURIComponent(pending)}&target=vi`);
      if (!resp.ok) {
        const body = await resp.json() as { error?: string };
        throw new Error(body.error ?? `HTTP ${resp.status}`);
      }
      const data = await resp.json() as TranslateResult;
      setResult(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  const copyResult = async () => {
    if (!result) return;
    try {
      await navigator.clipboard.writeText(result.translatedText);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1400);
    } catch {
      // ignore
    }
  };

  if (!anchor) return null;

  return (
    <div
      ref={popoverRef}
      className="translate-popover"
      style={{ left: anchor.x, top: anchor.y }}
    >
      {!result && !loading && !error && (
        <button type="button" className="translate-chip-btn" onClick={translate}>
          🌐 Translate to Vietnamese
        </button>
      )}
      {loading && <div className="translate-loading">Translating…</div>}
      {error && (
        <div className="translate-error">
          <span className="translate-error-msg">{error}</span>
          <button type="button" className="translate-dismiss" onClick={dismiss} aria-label="Dismiss">✕</button>
        </div>
      )}
      {result && (
        <div className="translate-result">
          <div className="translate-result-header">
            <span className="translate-result-lang">
              {(result.source || "en").toUpperCase()} → {result.target.toUpperCase()}
            </span>
            <button type="button" className="translate-copy-btn" onClick={copyResult} title={copied ? "Copied" : "Copy translation"}>
              {copied ? "✓" : "⧉"}
            </button>
            <button type="button" className="translate-dismiss" onClick={dismiss} aria-label="Dismiss">✕</button>
          </div>
          <div className="translate-result-text">{result.translatedText}</div>
        </div>
      )}
    </div>
  );
}
