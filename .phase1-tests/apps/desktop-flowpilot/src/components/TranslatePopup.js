"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.TranslatePopup = TranslatePopup;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const config_1 = require("@/config");
function runnerFetch(path) {
    return fetch(new URL(path, config_1.RUNNER_URL).toString(), { cache: "no-store" });
}
const VIEWPORT_MARGIN = 12;
const MIN_TEXT_HEIGHT = 120;
function TranslatePopup({ containerRef }) {
    const [anchor, setAnchor] = (0, react_1.useState)(null);
    const [placement, setPlacement] = (0, react_1.useState)(null);
    const [dragOffset, setDragOffset] = (0, react_1.useState)({ x: 0, y: 0 });
    const [pending, setPending] = (0, react_1.useState)("");
    const [result, setResult] = (0, react_1.useState)(null);
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [error, setError] = (0, react_1.useState)(null);
    const [copied, setCopied] = (0, react_1.useState)(false);
    const [dragging, setDragging] = (0, react_1.useState)(false);
    const popoverRef = (0, react_1.useRef)(null);
    const headerRef = (0, react_1.useRef)(null);
    const dragStateRef = (0, react_1.useRef)(null);
    const pointerStartedInsidePopupRef = (0, react_1.useRef)(false);
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
    (0, react_1.useEffect)(() => {
        const onMouseUp = (event) => {
            if (pointerStartedInsidePopupRef.current) {
                pointerStartedInsidePopupRef.current = false;
                return;
            }
            const root = popoverRef.current;
            if (root && root.contains(event.target))
                return;
            window.setTimeout(() => {
                const sel = window.getSelection();
                if (!sel || sel.isCollapsed)
                    return;
                const text = sel.toString().trim();
                if (!text)
                    return;
                const container = containerRef.current;
                if (!container)
                    return;
                const range = sel.getRangeAt(0);
                const root = popoverRef.current;
                if (root && root.contains(range.commonAncestorContainer))
                    return;
                if (!container.contains(range.commonAncestorContainer))
                    return;
                const rect = range.getBoundingClientRect();
                setPending(text);
                setAnchor({ x: rect.left + rect.width / 2, top: rect.top, bottom: rect.bottom });
                setPlacement(null);
                setDragOffset({ x: 0, y: 0 });
                setResult(null);
                setError(null);
            }, 10);
        };
        const onPointerDown = (event) => {
            const root = popoverRef.current;
            if (root && root.contains(event.target))
                return;
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
    (0, react_1.useLayoutEffect)(() => {
        if (!anchor)
            return;
        const el = popoverRef.current;
        if (!el)
            return;
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
            if (prev && prev.left === left && prev.top === top && prev.textMaxHeight === textMaxHeight)
                return prev;
            return { left, top, textMaxHeight };
        });
    }, [anchor, pending, result, loading, error]);
    (0, react_1.useEffect)(() => {
        if (!anchor)
            return;
        const onPointerMove = (event) => {
            const dragState = dragStateRef.current;
            const placementState = placement;
            const popover = popoverRef.current;
            if (!dragState || !placementState || !popover)
                return;
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
        if (!pending)
            return;
        setLoading(true);
        setError(null);
        try {
            const resp = await runnerFetch(`/translate?q=${encodeURIComponent(pending)}&target=vi`);
            if (!resp.ok) {
                const body = await resp.json();
                throw new Error(body.error ?? `HTTP ${resp.status}`);
            }
            const data = await resp.json();
            setResult(data);
        }
        catch (err) {
            setError(err instanceof Error ? err.message : String(err));
        }
        finally {
            setLoading(false);
        }
    };
    const copyResult = async () => {
        if (!result)
            return;
        try {
            await navigator.clipboard.writeText(result.translatedText);
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1400);
        }
        catch {
            // ignore
        }
    };
    if (!anchor)
        return null;
    const style = placement
        ? { left: placement.left + dragOffset.x, top: placement.top + dragOffset.y }
        : { left: anchor.x, top: anchor.bottom + 8, transform: "translateX(-50%)" };
    const beginDrag = (event) => {
        if (!placement)
            return;
        if (event.target.closest("button"))
            return;
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
    return ((0, jsx_runtime_1.jsxs)("div", { ref: popoverRef, className: "translate-popover", style: style, onPointerDown: (event) => {
            pointerStartedInsidePopupRef.current = true;
            event.stopPropagation();
        }, children: [!result && !loading && !error && ((0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-chip-btn", onClick: translate, children: "\uD83C\uDF10 Translate to Vietnamese" })), loading && (0, jsx_runtime_1.jsx)("div", { className: "translate-loading", children: "Translating\u2026" }), error && ((0, jsx_runtime_1.jsxs)("div", { className: "translate-error", children: [(0, jsx_runtime_1.jsx)("span", { className: "translate-error-msg", children: error }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-dismiss", onClick: dismiss, "aria-label": "Dismiss", children: "\u2715" })] })), result && ((0, jsx_runtime_1.jsxs)("div", { className: "translate-result", children: [(0, jsx_runtime_1.jsxs)("div", { ref: headerRef, className: `translate-result-header translate-drag-handle${dragging ? " is-dragging" : ""}`, onPointerDown: beginDrag, children: [(0, jsx_runtime_1.jsxs)("span", { className: "translate-result-lang", children: [(result.source || "en").toUpperCase(), " \u2192 ", result.target.toUpperCase()] }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-copy-btn", onClick: copyResult, title: copied ? "Copied" : "Copy translation", children: copied ? "✓" : "⧉" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-dismiss", onClick: dismiss, "aria-label": "Dismiss", children: "\u2715" })] }), (0, jsx_runtime_1.jsx)("div", { className: "translate-result-text", style: { maxHeight: placement ? placement.textMaxHeight : undefined }, children: result.translatedText })] }))] }));
}
