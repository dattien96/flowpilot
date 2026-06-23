"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.getOrchestrationBoardEmptyCopy = getOrchestrationBoardEmptyCopy;
exports.getBusMessageLabel = getBusMessageLabel;
exports.OrchestrationBoard = OrchestrationBoard;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
function getOrchestrationBoardEmptyCopy(snapshot, agentRuns = []) {
    if (!snapshot)
        return "No graph snapshot loaded. Refresh to inspect the loop.";
    if (snapshot.runs.length === 0 && agentRuns.length === 0)
        return "No child agents yet.";
    if (snapshot.runs.length === 0)
        return "Snapshot loaded, but no child agents are currently tracked.";
    return "";
}
function getBusMessageLabel(message) {
    return message.queued ? `${message.kind}: ${message.message} (queued)` : `${message.kind}: ${message.message}`;
}
function OrchestrationBoard() {
    const snapshot = (0, store_1.useStore)((s) => s.agentGraphSnapshot);
    const agentRuns = (0, store_1.useStore)((s) => s.agentRuns);
    const activeAgentRunId = (0, store_1.useStore)((s) => s.activeAgentRunId);
    const refresh = (0, store_1.useStore)((s) => s.refreshAgentGraph);
    const refreshRuns = (0, store_1.useStore)((s) => s.refreshAgentRuns);
    const pause = (0, store_1.useStore)((s) => s.pauseAgentLoop);
    const resume = (0, store_1.useStore)((s) => s.resumeAgentLoop);
    const stop = (0, store_1.useStore)((s) => s.stopAgentLoop);
    const focusAgentRun = (0, store_1.useStore)((s) => s.focusAgentRun);
    const injectAgentFeedback = (0, store_1.useStore)((s) => s.injectAgentFeedback);
    const openAgentSpawnGuide = (0, store_1.useStore)((s) => s.openAgentSpawnGuide);
    const mainRunId = (0, store_1.useStore)((s) => s.mainRunId ?? s.runId);
    const [feedback, setFeedback] = (0, react_1.useState)("");
    const focusTarget = (0, react_1.useMemo)(() => snapshot?.runs.find((run) => run.status === "running")?.runId ?? activeAgentRunId, [snapshot, activeAgentRunId]);
    (0, react_1.useEffect)(() => {
        void refresh();
        void refreshRuns();
    }, [mainRunId, refresh, refreshRuns]);
    const emptyCopy = getOrchestrationBoardEmptyCopy(snapshot, agentRuns);
    const canRender = Boolean(snapshot) && emptyCopy.length === 0;
    // Find canonical coder/reviewer runs for side-by-side SVG rendering
    const coderRun = (0, react_1.useMemo)(() => {
        return snapshot?.runs.find((run) => run.agentName.toLowerCase().includes("coder"));
    }, [snapshot]);
    const reviewerRun = (0, react_1.useMemo)(() => {
        return snapshot?.runs.find((run) => run.agentName.toLowerCase().includes("reviewer"));
    }, [snapshot]);
    const loopStatus = snapshot?.loopState.status ?? "idle";
    const loopRound = snapshot?.loopState.round ?? 0;
    const loopRoundCap = snapshot?.loopState.roundCap ?? 3;
    const loopGate = snapshot?.loopState.gateReason;
    return ((0, jsx_runtime_1.jsxs)("section", { className: "board", children: [(0, jsx_runtime_1.jsxs)("header", { className: "board-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("h2", { children: "\u25A6 Orchestration" }), (0, jsx_runtime_1.jsxs)("span", { className: "board-goal", children: ["Goal: ", (0, jsx_runtime_1.jsx)("b", { children: "Land BUG-094 fix, reviewer-approved" })] })] }), snapshot?.loopState && ((0, jsx_runtime_1.jsxs)("span", { className: "status agents font-semibold", children: [(0, jsx_runtime_1.jsx)("span", { className: "pulse" }), "round ", loopRound, " / ", loopRoundCap] }))] }), !canRender ? ((0, jsx_runtime_1.jsx)("div", { className: "board-empty", style: { flex: 1, display: "flex", alignItems: "center", justifyContent: "center", minHeight: "200px" }, children: emptyCopy })) : ((0, jsx_runtime_1.jsxs)("div", { style: { flex: 1, display: "flex", flexDirection: "column", gap: "16px" }, children: [coderRun && reviewerRun ? ((0, jsx_runtime_1.jsxs)("div", { className: "bgrid", children: [(0, jsx_runtime_1.jsxs)("div", { className: "node coder", children: [(0, jsx_runtime_1.jsxs)("h3", { children: [(0, jsx_runtime_1.jsx)("span", { className: `sd ${coderRun.status === "running" ? "run" : (coderRun.status === "waiting_approval" || coderRun.status === "waiting_question") ? "wait" : "done"}` }), coderRun.agentName] }), (0, jsx_runtime_1.jsx)("div", { className: "role", children: "Claude \u00B7 implements the change" }), (0, jsx_runtime_1.jsxs)("div", { className: "state", children: [(0, jsx_runtime_1.jsx)("span", { className: `sd ${coderRun.status === "running" ? "run" : "done"}` }), coderRun.status === "running" ? "running — applying feedback" : coderRun.status] }), coderRun.agentStatus && ((0, jsx_runtime_1.jsxs)("div", { className: "substep", children: ["\u25B8 ", coderRun.agentStatus] })), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "secondary-btn btn-sm", onClick: () => void focusAgentRun(coderRun.runId), children: "Focus chat" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "edge", children: [(0, jsx_runtime_1.jsxs)("svg", { viewBox: "0 0 70 110", preserveAspectRatio: "none", children: [(0, jsx_runtime_1.jsxs)("defs", { children: [(0, jsx_runtime_1.jsx)("marker", { id: "g", markerWidth: "9", markerHeight: "9", refX: "7", refY: "3", orient: "auto", children: (0, jsx_runtime_1.jsx)("path", { d: "M0,0 L7,3 L0,6 Z", fill: "#3fb950" }) }), (0, jsx_runtime_1.jsx)("marker", { id: "v", markerWidth: "9", markerHeight: "9", refX: "7", refY: "3", orient: "auto", children: (0, jsx_runtime_1.jsx)("path", { d: "M0,0 L7,3 L0,6 Z", fill: "#b07cff" }) })] }), (0, jsx_runtime_1.jsx)("path", { d: "M2,34 L66,34", fill: "none", stroke: "#3fb950", strokeWidth: "2", markerEnd: "url(#g)" }), (0, jsx_runtime_1.jsx)("path", { d: "M66,76 L4,76", fill: "none", stroke: "#b07cff", strokeWidth: "2", markerEnd: "url(#v)" })] }), (0, jsx_runtime_1.jsx)("span", { className: "lbl", style: { top: "14px" }, children: "diff \u25B8" }), (0, jsx_runtime_1.jsx)("span", { className: "lbl", style: { top: "64px" }, children: "\u25C2 feedback" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "node reviewer", children: [(0, jsx_runtime_1.jsxs)("h3", { children: [(0, jsx_runtime_1.jsx)("span", { className: `sd ${reviewerRun.status === "running" ? "run" : (reviewerRun.status === "waiting_approval" || reviewerRun.status === "waiting_question") ? "wait" : "done"}` }), reviewerRun.agentName] }), (0, jsx_runtime_1.jsx)("div", { className: "role", children: "Codex \u00B7 gates the change" }), (0, jsx_runtime_1.jsxs)("div", { className: "state", children: [(0, jsx_runtime_1.jsx)("span", { className: `sd ${reviewerRun.status === "running" ? "run" : "done"}` }), reviewerRun.status === "running" ? "running — evaluating diff" : reviewerRun.status] }), loopGate ? ((0, jsx_runtime_1.jsxs)("div", { className: "substep", children: ["round ", loopRound - 1, " verdict: ", (0, jsx_runtime_1.jsx)("span", { style: { color: "var(--warn)", fontWeight: "bold" }, children: loopGate })] })) : reviewerRun.agentStatus ? ((0, jsx_runtime_1.jsxs)("div", { className: "substep", children: ["\u25B8 ", reviewerRun.agentStatus] })) : null, (0, jsx_runtime_1.jsx)("button", { type: "button", className: "secondary-btn btn-sm", onClick: () => void focusAgentRun(reviewerRun.runId), children: "Focus chat" })] })] })) : ((0, jsx_runtime_1.jsx)("div", { className: "board-graph", role: "list", "aria-label": "Agent dependency graph", children: snapshot.runs.map((run) => {
                            const focused = run.runId === activeAgentRunId;
                            const statusDotColor = run.status === "running" ? "var(--ok)" : (run.status === "waiting_approval" || run.status === "waiting_question") ? "var(--warn)" : "var(--accent)";
                            return ((0, jsx_runtime_1.jsxs)("article", { className: `node ${focused ? "coder" : "reviewer"}`, role: "listitem", style: { borderTopColor: statusDotColor }, children: [(0, jsx_runtime_1.jsxs)("div", { className: "board-node-top", children: [(0, jsx_runtime_1.jsx)("strong", { children: run.agentName || run.runId }), (0, jsx_runtime_1.jsx)("span", { children: run.status })] }), (0, jsx_runtime_1.jsx)("div", { className: "role", children: run.role || "agent" }), run.agentStatus && (0, jsx_runtime_1.jsxs)("div", { className: "substep", children: ["\u25B8 ", run.agentStatus] }), (0, jsx_runtime_1.jsx)("div", { className: "board-node-actions", children: (0, jsx_runtime_1.jsx)("button", { type: "button", className: "secondary-btn btn-sm", onClick: () => void focusAgentRun(run.runId), children: "Focus" }) })] }, run.runId));
                        }) })), (0, jsx_runtime_1.jsxs)("div", { className: "board-round", children: ["Iteration ", (0, jsx_runtime_1.jsxs)("b", { children: [loopRound, " of ", loopRoundCap] }), " \u00B7 auto-approve when reviewer returns ", (0, jsx_runtime_1.jsx)("b", { style: { color: "var(--ok)" }, children: "APPROVED" }), " \u2192 handoff \u25B8 main agent commits"] }), (0, jsx_runtime_1.jsxs)("div", { className: "bctl", children: [(0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc primary", onClick: () => void resume(), children: "\u25B8 Resume loop" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc", onClick: () => void pause(), children: "\u23F8 Pause" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc warn", onClick: () => openAgentSpawnGuide("tester"), children: "\u2935 Add tester" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc err", onClick: () => void stop(), children: "\u2A09 Stop all" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "board-compose", children: [(0, jsx_runtime_1.jsx)("input", { type: "text", value: feedback, onChange: (e) => setFeedback(e.target.value), placeholder: "Inject feedback to the active child..." }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "primary-btn", disabled: !focusTarget || feedback.trim().length === 0, onClick: () => focusTarget && void injectAgentFeedback(focusTarget, feedback).then(() => setFeedback("")), children: "Inject feedback" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "log", children: [(0, jsx_runtime_1.jsx)("div", { className: "lh", children: "Agent bus \u00B7 live" }), (0, jsx_runtime_1.jsx)("div", { className: "log-body", children: snapshot.busMessages.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "ln", style: { color: "var(--text-dim)", fontStyle: "italic" }, children: "No bus messages yet." })) : (snapshot.busMessages.map((msg) => {
                                    const time = new Date(msg.occurredAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
                                    const lowerSender = msg.fromRunId ? (snapshot.runs.find(r => r.runId === msg.fromRunId)?.agentName.toLowerCase() ?? "") : "orchestrator";
                                    const senderClass = lowerSender.includes("coder") ? "a-coder" : lowerSender.includes("reviewer") ? "a-rev" : "a-orc";
                                    const senderName = msg.fromRunId ? (snapshot.runs.find(r => r.runId === msg.fromRunId)?.agentName ?? msg.fromRunId) : "orchestrator";
                                    return ((0, jsx_runtime_1.jsxs)("div", { className: "ln", children: [(0, jsx_runtime_1.jsx)("span", { className: "t", children: time }), (0, jsx_runtime_1.jsx)("span", { className: senderClass, children: senderName }), (0, jsx_runtime_1.jsxs)("span", { style: { color: "var(--text)" }, children: ["\u2192 bus: ", msg.message] })] }, msg.id));
                                })) })] })] }))] }));
}
