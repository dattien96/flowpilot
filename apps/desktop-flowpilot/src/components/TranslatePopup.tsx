import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { RUNNER_URL } from "@/config";
import { CheckIcon, CloseIcon, CopyIcon, GlobeIcon } from "@/components/icons";

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

interface Anchor {
  x: number;
  top: number;
  bottom: number;
}

interface Placement {
  left: number;
  top: number;
  textMaxHeight: number;
}

interface DragOffset {
  x: number;
  y: number;
}

const VIEWPORT_MARGIN = 12;
const MIN_TEXT_HEIGHT = 120;

export function TranslatePopup({ containerRef }: Props): React.ReactElement | null {
  const [anchor, setAnchor] = useState<Anchor | null>(null);
  const [placement, setPlacement] = useState<Placement | null>(null);
  const [dragOffset, setDragOffset] = useState<DragOffset>({ x: 0, y: 0 });
  const [pending, setPending] = useState("");
  const [result, setResult] = useState<TranslateResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [dragging, setDragging] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);
  const headerRef = useRef<HTMLDivElement>(null);
  const dragStateRef = useRef<{ startX: number; startY: number; originX: number; originY: number } | null>(null);
  const pointerStartedInsidePopupRef = useRef(false);

  const dismiss = () => {
    setAnchor(null);
    setPlacement(null);
    setDragOffset({ x: 0, y: 0 });
    setResult(null);
    setError(null);
    setPending("");
    setDragging(false);
    dragStateRef.current = null;
  };

  useEffect(() => {
    const onMouseUp = (event: MouseEvent) => {
      if (pointerStartedInsidePopupRef.current) {
        pointerStartedInsidePopupRef.current = false;
        return;
      }
      const root = popoverRef.current;
      if (root && root.contains(event.target as Node)) return;
      window.setTimeout(() => {
        const sel = window.getSelection();
        if (!sel || sel.isCollapsed) return;
        const text = sel.toString().trim();
        if (!text) return;
        const container = containerRef.current;
        if (!container) return;
        const range = sel.getRangeAt(0);
        const root = popoverRef.current;
        if (root && root.contains(range.commonAncestorContainer)) return;
        if (!container.contains(range.commonAncestorContainer)) return;
        const rect = range.getBoundingClientRect();
        setPending(text);
        setAnchor({ x: rect.left + rect.width / 2, top: rect.top, bottom: rect.bottom });
        setPlacement(null);
        setDragOffset({ x: 0, y: 0 });
        setResult(null);
        setError(null);
      }, 10);
    };

    const onPointerDown = (event: PointerEvent) => {
      const root = popoverRef.current;
      if (root && root.contains(event.target as Node)) return;
      dismiss();
    };

    const onPointerUp = () => {
      window.setTimeout(() => {
        pointerStartedInsidePopupRef.current = false;
      }, 0);
    };

    document.addEventListener("mouseup", onMouseUp);
    window.addEventListener("pointerdown", onPointerDown);
    window.addEventListener("pointerup", onPointerUp);
    window.addEventListener("pointercancel", onPointerUp);
    return () => {
      document.removeEventListener("mouseup", onMouseUp);
      window.removeEventListener("pointerdown", onPointerDown);
      window.removeEventListener("pointerup", onPointerUp);
      window.removeEventListener("pointercancel", onPointerUp);
    };
  }, [containerRef]);

  useLayoutEffect(() => {
    if (!anchor) return;
    const el = popoverRef.current;
    if (!el) return;

    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const width = el.offsetWidth;
    const height = el.offsetHeight;

    let left = anchor.x - width / 2;
    left = Math.min(Math.max(left, VIEWPORT_MARGIN), Math.max(VIEWPORT_MARGIN, vw - width - VIEWPORT_MARGIN));

    const spaceBelow = vh - anchor.bottom - VIEWPORT_MARGIN - 8;
    const spaceAbove = anchor.top - VIEWPORT_MARGIN - 8;
    const fitsBelow = height <= spaceBelow || spaceBelow >= spaceAbove;
    const boxMax = Math.max(MIN_TEXT_HEIGHT, fitsBelow ? spaceBelow : spaceAbove);
    const top = fitsBelow
      ? anchor.bottom + 8
      : Math.max(VIEWPORT_MARGIN, anchor.top - Math.min(height, boxMax) - 8);

    const headerHeight = headerRef.current?.offsetHeight ?? 0;
    const textMaxHeight = Math.max(MIN_TEXT_HEIGHT, boxMax - headerHeight);

    setPlacement((prev) => {
      if (prev && prev.left === left && prev.top === top && prev.textMaxHeight === textMaxHeight) return prev;
      return { left, top, textMaxHeight };
    });
  }, [anchor, pending, result, loading, error]);

  useEffect(() => {
    if (!anchor) return;

    const onPointerMove = (event: PointerEvent) => {
      const dragState = dragStateRef.current;
      const placementState = placement;
      const popover = popoverRef.current;
      if (!dragState || !placementState || !popover) return;

      const width = popover.offsetWidth;
      const height = popover.offsetHeight;
      const minLeft = VIEWPORT_MARGIN - placementState.left;
      const maxLeft = window.innerWidth - VIEWPORT_MARGIN - width - placementState.left;
      const minTop = VIEWPORT_MARGIN - placementState.top;
      const maxTop = window.innerHeight - VIEWPORT_MARGIN - height - placementState.top;
      const nextX = dragState.originX + (event.clientX - dragState.startX);
      const nextY = dragState.originY + (event.clientY - dragState.startY);

      setDragOffset({
        x: Math.min(Math.max(nextX, minLeft), maxLeft),
        y: Math.min(Math.max(nextY, minTop), maxTop),
      });
    };

    const stopDragging = () => {
      dragStateRef.current = null;
      setDragging(false);
    };

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", stopDragging);
    window.addEventListener("pointercancel", stopDragging);
    return () => {
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerup", stopDragging);
      window.removeEventListener("pointercancel", stopDragging);
    };
  }, [anchor, placement]);

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

  const style = placement
    ? { left: placement.left + dragOffset.x, top: placement.top + dragOffset.y }
    : { left: anchor.x, top: anchor.bottom + 8, transform: "translateX(-50%)" };

  const beginDrag = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!placement) return;
    if ((event.target as HTMLElement).closest("button")) return;
    event.preventDefault();
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    dragStateRef.current = {
      startX: event.clientX,
      startY: event.clientY,
      originX: dragOffset.x,
      originY: dragOffset.y,
    };
    setDragging(true);
  };

  return (
    <div
      ref={popoverRef}
      className="translate-popover"
      style={style}
      onPointerDown={(event) => {
        pointerStartedInsidePopupRef.current = true;
        event.stopPropagation();
      }}
    >
      {!result && !loading && !error && (
        <button type="button" className="translate-chip-btn" onClick={translate}>
          <GlobeIcon size={12} /> Translate to Vietnamese
        </button>
      )}
      {loading && <div className="translate-loading">Translating…</div>}
      {error && (
        <div className="translate-error">
          <span className="translate-error-msg">{error}</span>
          <button type="button" className="translate-dismiss" onClick={dismiss} aria-label="Dismiss"><CloseIcon size={11} /></button>
        </div>
      )}
      {result && (
        <div className="translate-result">
          <div
            ref={headerRef}
            className={`translate-result-header translate-drag-handle${dragging ? " is-dragging" : ""}`}
            onPointerDown={beginDrag}
          >
            <span className="translate-result-lang">
              {(result.source || "en").toUpperCase()} → {result.target.toUpperCase()}
            </span>
            <button type="button" className="translate-copy-btn" onClick={copyResult} title={copied ? "Copied" : "Copy translation"}>
              {copied ? <CheckIcon size={11} /> : <CopyIcon size={11} />}
            </button>
            <button type="button" className="translate-dismiss" onClick={dismiss} aria-label="Dismiss"><CloseIcon size={11} /></button>
          </div>
          <div className="translate-result-text" style={{ maxHeight: placement ? placement.textMaxHeight : undefined }}>
            {result.translatedText}
          </div>
        </div>
      )}
    </div>
  );
}
