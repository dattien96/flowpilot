"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.getOrchestrationBoardEmptyCopy = getOrchestrationBoardEmptyCopy;
exports.getBusMessageLabel = getBusMessageLabel;
exports.OrchestrationBoard = OrchestrationBoard;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
function nodeStatusClass(status) {
    if (status === "running")
        return "run";
    if (status === "waiting_approval" || status === "waiting_question")
        return "wait";
    return "done";
}
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
    const submitReviewOutcome = (0, store_1.useStore)((s) => s.submitReviewOutcome);
    const extendCap = (0, store_1.useStore)((s) => s.extendCap);
    const mainRunId = (0, store_1.useStore)((s) => s.mainRunId ?? s.runId);
    const [feedback, setFeedback] = (0, react_1.useState)("");
    const focusTarget = (0, react_1.useMemo)(() => snapshot?.runs.find((run) => run.status === "running")?.runId ?? activeAgentRunId, [snapshot, activeAgentRunId]);
    (0, react_1.useEffect)(() => {
        void refresh();
        void refreshRuns();
    }, [mainRunId, refresh, refreshRuns]);
    const emptyCopy = getOrchestrationBoardEmptyCopy(snapshot, agentRuns);
    const canRender = Boolean(snapshot) && emptyCopy.length === 0;
    const loopState = snapshot?.loopState;
    const loopStatus = loopState?.status ?? "idle";
    const loopRound = loopState?.round ?? 0;
    const loopCap = loopState?.cap ?? loopState?.roundCap ?? 3;
    const loopGate = loopState?.gateReason;
    const openIssues = loopState?.openIssues;
    const isBlocked = loopStatus === "blocked";
    return ((0, jsx_runtime_1.jsxs)("section", { className: "board", children: [(0, jsx_runtime_1.jsxs)("header", { className: "board-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("h2", { children: "\u25A6 Orchestration" }), loopState?.activeNode && ((0, jsx_runtime_1.jsxs)("span", { className: "board-goal", children: ["Active: ", (0, jsx_runtime_1.jsx)("b", { children: loopState.activeNode })] }))] }), loopState && ((0, jsx_runtime_1.jsxs)("span", { className: `status agents font-semibold${isBlocked ? " warn" : ""}`, children: [(0, jsx_runtime_1.jsx)("span", { className: "pulse" }), "round ", loopRound, " / ", loopCap, openIssues !== undefined && openIssues > 0 && ((0, jsx_runtime_1.jsxs)("span", { style: { marginLeft: "8px", color: "var(--warn)" }, children: [openIssues, " open"] })), loopState.mode && ((0, jsx_runtime_1.jsx)("span", { style: { marginLeft: "8px", opacity: 0.6 }, children: loopState.mode }))] }))] }), !canRender ? ((0, jsx_runtime_1.jsx)("div", { className: "board-empty", style: { flex: 1, display: "flex", alignItems: "center", justifyContent: "center", minHeight: "200px" }, children: emptyCopy })) : ((0, jsx_runtime_1.jsxs)("div", { style: { flex: 1, display: "flex", flexDirection: "column", gap: "16px" }, children: [(0, jsx_runtime_1.jsx)("div", { className: "board-graph", role: "list", "aria-label": "Agent dependency graph", children: snapshot.runs.map((run) => {
                            const focused = run.runId === activeAgentRunId;
                            const sc = nodeStatusClass(run.status);
                            const statusDotColor = sc === "run" ? "var(--ok)" : sc === "wait" ? "var(--warn)" : "var(--accent)";
                            return ((0, jsx_runtime_1.jsxs)("article", { className: `node ${focused ? "coder" : "reviewer"}`, role: "listitem", style: { borderTopColor: statusDotColor }, children: [(0, jsx_runtime_1.jsxs)("div", { className: "board-node-top", children: [(0, jsx_runtime_1.jsxs)("h3", { style: { margin: 0 }, children: [(0, jsx_runtime_1.jsx)("span", { className: `sd ${sc}` }), run.agentName || run.runId] }), (0, jsx_runtime_1.jsx)("span", { style: { fontSize: "0.75rem", opacity: 0.7 }, children: run.status })] }), run.role && run.role !== run.agentName && (0, jsx_runtime_1.jsx)("div", { className: "role", children: run.role }), run.agentStatus && (0, jsx_runtime_1.jsxs)("div", { className: "substep", children: ["\u25B8 ", run.agentStatus] }), loopGate && run.role?.toLowerCase().includes("reviewer") && ((0, jsx_runtime_1.jsxs)("div", { className: "substep", children: ["verdict: ", (0, jsx_runtime_1.jsx)("span", { style: { color: "var(--warn)", fontWeight: "bold" }, children: loopGate })] })), (0, jsx_runtime_1.jsx)("div", { className: "board-node-actions", children: (0, jsx_runtime_1.jsx)("button", { type: "button", className: "secondary-btn btn-sm", onClick: () => void focusAgentRun(run.runId), children: "Focus" }) })] }, run.runId));
                        }) }), (0, jsx_runtime_1.jsxs)("div", { className: "board-round", children: ["Round ", (0, jsx_runtime_1.jsxs)("b", { children: [loopRound, " / ", loopCap] }), openIssues !== undefined && (0, jsx_runtime_1.jsxs)("span", { children: [" \u00B7 ", (0, jsx_runtime_1.jsxs)("b", { style: { color: openIssues > 0 ? "var(--warn)" : "var(--ok)" }, children: [openIssues, " open issue", openIssues !== 1 ? "s" : ""] })] }), (0, jsx_runtime_1.jsxs)("span", { style: { marginLeft: "8px", opacity: 0.7 }, children: ["status: ", loopStatus] })] }), isBlocked && ((0, jsx_runtime_1.jsxs)("div", { className: "board-blocked", style: { padding: "10px 12px", background: "var(--warn-bg, rgba(255,180,0,0.12))", borderRadius: "6px", display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" }, children: [(0, jsx_runtime_1.jsx)("span", { style: { color: "var(--warn)", fontWeight: "bold" }, children: "\u26A0 Blocked" }), loopGate && (0, jsx_runtime_1.jsx)("span", { style: { opacity: 0.8 }, children: loopGate }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc warn", onClick: () => void extendCap(), children: "Extend cap +2" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc primary", onClick: () => void submitReviewOutcome("approved"), children: "Accept & approve" })] })), (0, jsx_runtime_1.jsxs)("div", { className: "bctl", children: [(0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc primary", onClick: () => void resume(), children: "\u25B8 Resume loop" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc", onClick: () => void pause(), children: "\u23F8 Pause" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc warn", onClick: () => openAgentSpawnGuide("tester"), children: "\u2935 Add tester" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc err", onClick: () => void stop(), children: "\u2A09 Stop all" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "board-compose", children: [(0, jsx_runtime_1.jsx)("input", { type: "text", value: feedback, onChange: (e) => setFeedback(e.target.value), placeholder: "Inject feedback to the active child..." }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "primary-btn", disabled: !focusTarget || feedback.trim().length === 0, onClick: () => focusTarget && void injectAgentFeedback(focusTarget, feedback).then(() => setFeedback("")), children: "Inject feedback" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "log", children: [(0, jsx_runtime_1.jsx)("div", { className: "lh", children: "Agent bus \u00B7 live" }), (0, jsx_runtime_1.jsx)("div", { className: "log-body", children: snapshot.busMessages.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "ln", style: { color: "var(--text-dim)", fontStyle: "italic" }, children: "No bus messages yet." })) : (snapshot.busMessages.map((msg) => {
                                    const time = new Date(msg.occurredAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
                                    const lowerSender = msg.fromRunId ? (snapshot.runs.find(r => r.runId === msg.fromRunId)?.agentName.toLowerCase() ?? "") : "orchestrator";
                                    const senderClass = lowerSender.includes("coder") ? "a-coder" : lowerSender.includes("reviewer") ? "a-rev" : "a-orc";
                                    const senderName = msg.fromRunId ? (snapshot.runs.find(r => r.runId === msg.fromRunId)?.agentName ?? msg.fromRunId) : "orchestrator";
                                    return ((0, jsx_runtime_1.jsxs)("div", { className: "ln", children: [(0, jsx_runtime_1.jsx)("span", { className: "t", children: time }), (0, jsx_runtime_1.jsx)("span", { className: senderClass, children: senderName }), (0, jsx_runtime_1.jsxs)("span", { style: { color: "var(--text)" }, children: ["\u2192 bus: ", msg.message] })] }, msg.id));
                                })) })] })] }))] }));
}
