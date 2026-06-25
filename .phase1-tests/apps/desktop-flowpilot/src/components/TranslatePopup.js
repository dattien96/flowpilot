"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.TranslatePopup = TranslatePopup;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const config_1 = require("@/config");
function runnerFetch(path) {
    return fetch(new URL(path, config_1.RUNNER_URL).toString(), { cache: "no-store" });
}
function TranslatePopup({ containerRef }) {
    const [anchor, setAnchor] = (0, react_1.useState)(null);
    const [pending, setPending] = (0, react_1.useState)("");
    const [result, setResult] = (0, react_1.useState)(null);
    const [loading, setLoading] = (0, react_1.useState)(false);
    const [error, setError] = (0, react_1.useState)(null);
    const [copied, setCopied] = (0, react_1.useState)(false);
    const popoverRef = (0, react_1.useRef)(null);
    const dismiss = () => {
        setAnchor(null);
        setResult(null);
        setError(null);
        setPending("");
    };
    (0, react_1.useEffect)(() => {
        const onMouseUp = () => {
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
                if (!container.contains(range.commonAncestorContainer))
                    return;
                const rect = range.getBoundingClientRect();
                setPending(text);
                setAnchor({ x: rect.left + rect.width / 2, y: rect.bottom + 8 });
                setResult(null);
                setError(null);
            }, 10);
        };
        const onMouseDown = (e) => {
            if (popoverRef.current && popoverRef.current.contains(e.target))
                return;
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
    return ((0, jsx_runtime_1.jsxs)("div", { ref: popoverRef, className: "translate-popover", style: { left: anchor.x, top: anchor.y }, children: [!result && !loading && !error && ((0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-chip-btn", onClick: translate, children: "\uD83C\uDF10 Translate to Vietnamese" })), loading && (0, jsx_runtime_1.jsx)("div", { className: "translate-loading", children: "Translating\u2026" }), error && ((0, jsx_runtime_1.jsxs)("div", { className: "translate-error", children: [(0, jsx_runtime_1.jsx)("span", { className: "translate-error-msg", children: error }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-dismiss", onClick: dismiss, "aria-label": "Dismiss", children: "\u2715" })] })), result && ((0, jsx_runtime_1.jsxs)("div", { className: "translate-result", children: [(0, jsx_runtime_1.jsxs)("div", { className: "translate-result-header", children: [(0, jsx_runtime_1.jsxs)("span", { className: "translate-result-lang", children: [(result.source || "en").toUpperCase(), " \u2192 ", result.target.toUpperCase()] }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-copy-btn", onClick: copyResult, title: copied ? "Copied" : "Copy translation", children: copied ? "✓" : "⧉" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "translate-dismiss", onClick: dismiss, "aria-label": "Dismiss", children: "\u2715" })] }), (0, jsx_runtime_1.jsx)("div", { className: "translate-result-text", children: result.translatedText })] }))] }));
}
