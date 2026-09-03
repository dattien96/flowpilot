"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.flowStepShowsModel = flowStepShowsModel;
exports.FlowStepTimeline = FlowStepTimeline;
const jsx_runtime_1 = require("react/jsx-runtime");
function visualState(status) {
    switch (status) {
        case "RUNNING":
            return "running";
        case "DONE":
            return "done";
        case "WAITING_USER_APPROVAL":
            return "approval";
        case "CANCELED":
            return "cancelled";
        case "FAILED":
            return "error";
        case "PENDING":
        case "SKIPPED":
        default:
            return "idle";
    }
}
const STATE_GLYPH = {
    idle: "",
    running: "",
    done: "✓",
    approval: "!",
    error: "✕",
    cancelled: "∅",
};
const STATE_LABEL = {
    PENDING: "pending",
    RUNNING: "running",
    WAITING_USER_APPROVAL: "waiting for approval",
    CANCELED: "cancelled",
    DONE: "done",
    FAILED: "failed",
    SKIPPED: "skipped",
};
// A node's agentRef is stored as either a bare agent name or a full definition
// path (the Agent-ref dropdown stores agent.path to disambiguate same-named
// files across sources). The timeline only needs the human-readable name, which
// is what the runtime resolves the ref to — so collapse a path to its base name
// without extension, matching the clean name shown in the Agents panel.
function agentRefLabel(agentRef) {
    const base = agentRef.split(/[\\/]/).pop() ?? agentRef;
    const dot = base.lastIndexOf(".");
    return dot > 0 ? base.slice(0, dot) : base;
}
/**
 * BUG-290: only an agent.delegate node ever spawns a provider turn
 * (BehaviorScopeInline, flow_executor.go) — every other behavior (e.g.
 * context.produce) has no model of its own, so the step-timeline must not
 * show the run's inherited model as if it belonged to that step. A step with
 * no behaviorId predates node_id (BUG-155) and is itself an agent.delegate
 * node, so it still shows its model.
 */
function flowStepShowsModel(behaviorId) {
    return !behaviorId || behaviorId === "agent.delegate";
}
function stepName(step) {
    // nodeId is the flow-graph node id ("coder", "reviewer_correctness"); stepType
    // for a CP-42 flow-engine node is a shared generic dispatch category
    // ("flow-agent-delegate") identical across every node on the same behavior, so
    // it is only a fallback for steps that predate node_id (BUG-155).
    return step.nodeId || step.stepType || step.stepId;
}
function FlowStepTimeline({ steps, compact = false, runProvider, runModel, }) {
    return ((0, jsx_runtime_1.jsx)("ol", { className: `flow-timeline ${compact ? "flow-timeline-compact" : ""}`, children: steps.map((step, index) => {
            const state = visualState(step.status);
            const isCurrent = state === "running" || state === "approval";
            const isLast = index === steps.length - 1;
            const lineState = state === "done" ? "done" : state === "running" ? "running" : "idle";
            const provider = step.provider || runProvider;
            const model = flowStepShowsModel(step.behaviorId) ? step.model || runModel : undefined;
            return ((0, jsx_runtime_1.jsxs)("li", { className: `flow-timeline-item fti-${state} ${isCurrent ? "fti-current" : ""}`, title: compact ? `${stepName(step)} — ${STATE_LABEL[step.status]}` : undefined, children: [(0, jsx_runtime_1.jsxs)("div", { className: "fti-track", children: [(0, jsx_runtime_1.jsx)("span", { className: "fti-icon", children: state === "done" || state === "error" || state === "cancelled" ? STATE_GLYPH[state] : index + 1 }), !isLast && (0, jsx_runtime_1.jsx)("span", { className: `fti-line fti-line-${lineState}` })] }), !compact && ((0, jsx_runtime_1.jsxs)("div", { className: "fti-body", children: [(0, jsx_runtime_1.jsx)("div", { className: "fti-title", children: stepName(step) }), (0, jsx_runtime_1.jsxs)("div", { className: "fti-desc", children: [step.rejectionNote || STATE_LABEL[step.status], step.retryCount > 0 && (0, jsx_runtime_1.jsxs)("span", { className: "wsr-retry-badge", children: ["Retry ", step.retryCount] })] }), (provider || model || step.agentRef) && ((0, jsx_runtime_1.jsxs)("div", { className: "fti-meta", children: [provider && (0, jsx_runtime_1.jsx)("span", { className: `pill-prov prov-${provider}`, children: provider.toUpperCase() }), model && (0, jsx_runtime_1.jsx)("span", { className: "ac-model", children: model }), step.agentRef && (0, jsx_runtime_1.jsxs)("span", { title: step.agentRef, children: ["agent: ", agentRefLabel(step.agentRef)] })] }))] }))] }, step.stepId));
        }) }));
}
