"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const timelineReducer_1 = require("./timelineReducer");
function state() {
    return {
        status: "running",
        timeline: [],
        recoverable: false,
        pendingApprovals: [],
        pendingQuestions: [],
    };
}
function event(overrides) {
    return {
        id: "event-1",
        workflowRunId: "run-1",
        providerSessionId: "thread-1",
        providerKey: "codex",
        seq: 1,
        occurredAt: "2026-07-18T14:00:00Z",
        type: "turn_completed",
        finalMessage: "done",
        ...overrides,
    };
}
(0, node_test_1.default)("agent lifecycle replays as one completed agent card instead of prose rows", () => {
    const spawned = (0, timelineReducer_1.applyTimelineEvent)(state(), event({ type: "agent_spawned_by_user", id: "agent-spawn", agentName: "coder", childRunId: "child-coder" }));
    const completed = (0, timelineReducer_1.applyTimelineEvent)({ ...state(), ...spawned, timeline: spawned.timeline ?? [] }, event({
        type: "agent_result_injected",
        id: "agent-result",
        agentName: "coder",
        childRunId: "child-coder",
        finalMessage: "implemented",
    }));
    strict_1.default.deepEqual(completed.timeline, [
        { kind: "agent", id: "agent-spawn", agentName: "coder", childRunId: "child-coder", finalMessage: "implemented" },
    ]);
});
