"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const timelineReducer_1 = require("./timelineReducer");
function initialState() {
    return { status: "running", timeline: [], recoverable: false, pendingApprovals: [], pendingQuestions: [] };
}
function event(providerKey, type, seq) {
    return {
        id: `${providerKey}-${seq}`,
        workflowRunId: `run-${providerKey}`,
        providerSessionId: `session-${providerKey}`,
        providerKey,
        seq,
        occurredAt: "2026-07-20T12:00:00Z",
        type,
        prompt: type === "turn_started" ? "restore this completed flow" : undefined,
    };
}
(0, node_test_1.default)("terminal replay clears Thinking for Codex, Claude, and Grok", () => {
    for (const providerKey of ["codex", "claude", "grok"]) {
        let state = (0, timelineReducer_1.applyTimelineEvent)(initialState(), event(providerKey, "turn_started", 1));
        state = (0, timelineReducer_1.applyTimelineEvent)({ ...initialState(), ...state, timeline: state.timeline ?? [] }, event(providerKey, "message_completed", 2));
        state = (0, timelineReducer_1.applyTimelineEvent)({ ...initialState(), ...state, timeline: state.timeline ?? [] }, event(providerKey, "turn_completed", 3));
        strict_1.default.equal(state.status, "completed", providerKey);
        strict_1.default.equal(state.timeline?.some((item) => item.kind === "thinking"), false, providerKey);
    }
});
