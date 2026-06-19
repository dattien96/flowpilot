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
    return ((0, jsx_runtime_1.jsxs)("section", { className: "orchestration-board", children: [(0, jsx_runtime_1.jsxs)("header", { className: "board-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "board-kicker", children: "Orchestration" }), (0, jsx_runtime_1.jsx)("h3", { children: "Dependency graph" }), (0, jsx_runtime_1.jsx)("p", { children: snapshot ? `${snapshot.loopState.status} · round ${snapshot.loopState.round}/${snapshot.loopState.roundCap}` : "Refresh to inspect the loop." }), snapshot?.loopState.gateReason && (0, jsx_runtime_1.jsxs)("div", { className: "board-gate", children: ["Gate: ", snapshot.loopState.gateReason] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "board-actions", children: [(0, jsx_runtime_1.jsx)("button", { type: "button", onClick: () => void refresh(), children: "Refresh graph" }), (0, jsx_runtime_1.jsx)("button", { type: "button", onClick: () => void refreshRuns(), children: "Refresh runs" }), (0, jsx_runtime_1.jsx)("button", { type: "button", onClick: () => void pause(), children: "Pause" }), (0, jsx_runtime_1.jsx)("button", { type: "button", onClick: () => void resume(), children: "Resume" }), (0, jsx_runtime_1.jsx)("button", { type: "button", onClick: () => void stop(), children: "Stop" }), (0, jsx_runtime_1.jsx)("button", { type: "button", onClick: () => openAgentSpawnGuide("coder"), children: "Add agent" })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "board-compose", children: [(0, jsx_runtime_1.jsx)("input", { value: feedback, onChange: (e) => setFeedback(e.target.value), placeholder: "Inject feedback to the active child..." }), (0, jsx_runtime_1.jsx)("button", { type: "button", disabled: !focusTarget || feedback.trim().length === 0, onClick: () => focusTarget && void injectAgentFeedback(focusTarget, feedback).then(() => setFeedback("")), children: "Inject feedback" })] }), canRender ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)("div", { className: "board-graph", role: "list", "aria-label": "Agent dependency graph", children: snapshot.runs.map((run) => {
                            const focused = run.runId === activeAgentRunId;
                            return ((0, jsx_runtime_1.jsxs)("article", { className: `board-node ${focused ? "active" : ""}`, role: "listitem", children: [(0, jsx_runtime_1.jsxs)("div", { className: "board-node-top", children: [(0, jsx_runtime_1.jsx)("strong", { children: run.agentName || run.runId }), (0, jsx_runtime_1.jsx)("span", { children: run.status })] }), (0, jsx_runtime_1.jsx)("div", { className: "board-node-role", children: run.role || "agent" }), (0, jsx_runtime_1.jsx)("div", { className: "board-node-meta", children: run.createdAt }), (0, jsx_runtime_1.jsx)("div", { className: "board-node-actions", children: (0, jsx_runtime_1.jsx)("button", { type: "button", onClick: () => void focusAgentRun(run.runId), children: "Focus" }) })] }, run.runId));
                        }) }), (0, jsx_runtime_1.jsx)("div", { className: "board-edges", children: snapshot.edges.length === 0 ? (0, jsx_runtime_1.jsx)("p", { children: "No dependency edges yet." }) : snapshot.edges.map((edge, idx) => (0, jsx_runtime_1.jsxs)("div", { children: [edge.kind, ": ", edge.fromRunId, " \u2192 ", edge.toRunId] }, `${edge.fromRunId}-${edge.toRunId}-${idx}`)) }), (0, jsx_runtime_1.jsxs)("div", { className: "board-bus", children: [(0, jsx_runtime_1.jsx)("h4", { children: "Bus log" }), snapshot.busMessages.length === 0 ? (0, jsx_runtime_1.jsx)("p", { children: "No bus messages yet." }) : snapshot.busMessages.map((msg) => (0, jsx_runtime_1.jsx)("div", { className: `bus-pill ${msg.queued ? "queued" : ""}`, children: getBusMessageLabel(msg) }, msg.id))] }), (0, jsx_runtime_1.jsxs)("div", { className: "board-foot", children: [(0, jsx_runtime_1.jsxs)("span", { children: ["Children: ", agentRuns.length] }), (0, jsx_runtime_1.jsxs)("span", { children: ["Focus: ", focusTarget ?? "none"] })] })] })) : ((0, jsx_runtime_1.jsx)("div", { className: "board-empty", children: emptyCopy }))] }));
}
