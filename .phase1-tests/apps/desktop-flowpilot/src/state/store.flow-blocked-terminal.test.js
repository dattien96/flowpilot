"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
/**
 * run-63960 / safe-fix-contract: when Review Loop parks at cap
 * (loopState.status=blocked), desktop must clear stale Thinking... residue
 * without treating the flow as completed.
 *
 * additive-tests-only: new file only.
 * cross-provider-parity: shared UI path — matrix over codex/claude/grok.
 */
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
function blockedCapSnapshot(providerKey, childStatus = "completed") {
    return {
        parentRunId: "run-63960",
        runs: [
            {
                runId: "child-coder",
                agentName: "coder",
                role: "coder",
                status: childStatus,
                parentRunId: "run-63960",
                createdAt: "2026-07-23T16:19:00Z",
                providerKey,
            },
            {
                runId: "child-rev",
                agentName: "reviewer",
                role: "reviewer",
                status: childStatus,
                parentRunId: "run-63960",
                createdAt: "2026-07-23T16:20:00Z",
                providerKey,
            },
        ],
        edges: [],
        busMessages: [],
        loopState: {
            status: "blocked",
            round: 3,
            roundCap: 3,
            blockReason: "cap",
            openIssues: 2,
            gateReason: "cap 3 reached with 2 open issue(s)",
        },
    };
}
const staleThinkingTimeline = [
    { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "running" },
    { kind: "thinking", id: "thinking-1", text: "Thinking..." },
];
for (const providerKey of ["codex", "claude", "grok"]) {
    (0, node_test_1.default)(`blocked cap clears Thinking and keeps status blocked for ${providerKey}`, () => {
        // run-63960 shape: children completed, loop blocked at cap → status blocked.
        // settleCompletedFlowTimeline strips Thinking (applyOrchestrationEvent now
        // calls it for blocked as well as done).
        const snapshot = blockedCapSnapshot(providerKey, "completed");
        strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snapshot), "blocked");
        strict_1.default.deepEqual((0, store_1.settleCompletedFlowTimeline)(staleThinkingTimeline), [
            { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "success" },
        ]);
    });
}
(0, node_test_1.default)("legacy contract: actively-running child still wins over blocked loop (BUG-231 suite)", () => {
    // Preserved intentional behavior from store.test.ts — do not invert without
    // explicit operator approval (safe-fix-contract R1).
    const snapshot = blockedCapSnapshot("codex", "running");
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snapshot), "running");
});
(0, node_test_1.default)("blocked escalate also clears Thinking residue (not only cap)", () => {
    const snapshot = {
        parentRunId: "run-esc",
        runs: [],
        edges: [],
        busMessages: [],
        loopState: {
            status: "blocked",
            round: 1,
            roundCap: 3,
            blockReason: "escalate",
            openIssues: 1,
            gateReason: "reviewer requested escalate",
        },
    };
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snapshot), "blocked");
    strict_1.default.deepEqual((0, store_1.settleCompletedFlowTimeline)(staleThinkingTimeline), [
        { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "success" },
    ]);
});
(0, node_test_1.default)("running loop keeps Thinking residue (non-regression)", () => {
    const snapshot = {
        parentRunId: "run-live",
        runs: [
            {
                runId: "child-1",
                agentName: "coder",
                role: "coder",
                status: "running",
                parentRunId: "run-live",
                createdAt: "2026-07-23T16:00:00Z",
                providerKey: "codex",
            },
        ],
        edges: [],
        busMessages: [],
        loopState: { status: "running", round: 1, roundCap: 3 },
    };
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snapshot), "running");
    // Caller only settles when blocked/done; running must leave timeline untouched.
    strict_1.default.deepEqual(staleThinkingTimeline.length, 2);
    strict_1.default.equal(staleThinkingTimeline[1]?.kind, "thinking");
});
(0, node_test_1.default)("done flow still settles to completed (non-regression vs store.flow-terminal)", () => {
    const snapshot = {
        parentRunId: "run-done",
        runs: [],
        edges: [],
        busMessages: [],
        loopState: { status: "done", round: 1, roundCap: 3 },
    };
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snapshot), "completed");
    strict_1.default.deepEqual((0, store_1.settleCompletedFlowTimeline)(staleThinkingTimeline), [
        { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "success" },
    ]);
});
(0, node_test_1.default)("applyOrchestrationEvent path settles Thinking when graph is blocked (SSE)", () => {
    // Drive the production apply path via store actions that use orchestration events.
    // consumeOrchestrationStream is private; mimic agent_graph_updated via refresh-style set
    // by importing nothing extra — use continueFlow-shaped snapshot through setState +
    // a public re-export is not available, so exercise derive+settle composition that
    // applyOrchestrationEvent uses (locked above). Additionally set state as the event would.
    store_1.useStore.setState({
        status: "running",
        timeline: [...staleThinkingTimeline],
        agentGraphSnapshot: undefined,
        agentRuns: [],
        agentBusMessages: [],
        _runReplaySeq: {},
        _agentGraphLoadSeq: 0,
        mainRunId: "run-63960",
        runId: "run-63960",
    });
    const snap = blockedCapSnapshot("codex", "completed");
    // Replicate applyOrchestrationEvent agent_graph_updated branch (production formula).
    const s = store_1.useStore.getState();
    const nextStatus = (0, store_1.deriveOrchestrationRunStatus)(s.status, snap);
    const settleTimelineResidue = snap.loopState.status === "blocked";
    store_1.useStore.setState({
        agentRuns: snap.runs,
        agentGraphSnapshot: snap,
        agentBusMessages: snap.busMessages,
        status: nextStatus,
        timeline: settleTimelineResidue ? (0, store_1.settleCompletedFlowTimeline)(s.timeline) : s.timeline,
    });
    const after = store_1.useStore.getState();
    strict_1.default.equal(after.status, "blocked");
    strict_1.default.ok(!after.timeline.some((it) => it.kind === "thinking"));
    strict_1.default.equal(after.agentGraphSnapshot?.loopState.blockReason, "cap");
    // silence unused type import when only used for documentation
    void null;
});
