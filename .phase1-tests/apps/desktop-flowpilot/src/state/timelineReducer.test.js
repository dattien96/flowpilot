"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const timelineReducer_1 = require("./timelineReducer");
function baseEvent(overrides) {
    return {
        id: "evt-1",
        workflowRunId: "run-1",
        providerSessionId: "thread-1",
        providerKey: "codex",
        seq: 1,
        occurredAt: "2026-06-13T10:00:00.000Z",
        type: "turn_completed",
        finalMessage: "done",
        ...overrides,
    };
}
function thinkingState(timeline) {
    return {
        status: "running",
        timeline,
        recoverable: false,
        _streamingAssistantId: "assistant-1",
    };
}
(0, node_test_1.default)("tool_completed keeps the live thinking row at the end of the timeline", () => {
    const state = thinkingState([
        { kind: "prompt", id: "prompt-1", text: "Fix it" },
        { kind: "assistant", id: "assistant-1", text: "Looking...", finalized: false },
        { kind: "tool", id: "tool-1", toolName: "search", status: "running", input: { q: "bug" } },
        { kind: "thinking", id: "thinking-1", text: "Thinking..." },
    ]);
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({
        type: "tool_completed",
        toolName: "search",
        status: "success",
        output: { matches: 2 },
    }));
    const nextTimeline = next.timeline ?? [];
    strict_1.default.equal(nextTimeline.at(-1)?.kind, "thinking");
    const tool = nextTimeline.find((item) => item.kind === "tool");
    strict_1.default.deepEqual(tool, {
        kind: "tool",
        id: "tool-1",
        toolName: "search",
        status: "success",
        input: { q: "bug" },
        output: { matches: 2 },
    });
});
(0, node_test_1.default)("turn_completed removes the thinking row once the answer is done", () => {
    const state = thinkingState([
        { kind: "prompt", id: "prompt-1", text: "Fix it" },
        { kind: "assistant", id: "assistant-1", text: "All set", finalized: true },
        { kind: "thinking", id: "thinking-1", text: "Thinking..." },
    ]);
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({
        type: "turn_completed",
        finalMessage: "All set",
    }));
    strict_1.default.equal(next.timeline?.some((item) => item.kind === "thinking"), false);
});
(0, node_test_1.default)("run events only apply to the currently active run", () => {
    strict_1.default.equal((0, timelineReducer_1.shouldApplyRunEvent)("run-claude", "run-claude"), true);
    strict_1.default.equal((0, timelineReducer_1.shouldApplyRunEvent)("run-claude", "run-codex"), false);
    strict_1.default.equal((0, timelineReducer_1.shouldApplyRunEvent)(undefined, "run-codex"), false);
});
// Approval gate tests
const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }] };
function approvalState(extras = {}) {
    return {
        status: "running",
        timeline: [],
        recoverable: false,
        ...extras,
    };
}
(0, node_test_1.default)("permission_required adds approval card to timeline and sets pendingApproval", () => {
    const state = approvalState();
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "permission_required", approvalId: "appr-1", provider: "codex", details: approvalDetails }));
    const card = next.timeline?.find((it) => it.kind === "approval");
    strict_1.default.ok(card, "approval card should be added");
    strict_1.default.equal(card.decision, undefined);
    strict_1.default.deepEqual(next.pendingApproval, { approvalId: "appr-1", details: approvalDetails });
    strict_1.default.equal(next.status, "waiting_approval");
});
(0, node_test_1.default)("history replay: tool_completed after permission_required stamps card resolved and clears pendingApproval", () => {
    // Simulates the state after permission_required was replayed from history
    const state = approvalState({
        status: "waiting_approval",
        timeline: [
            { kind: "tool", id: "tool-1", toolName: "mcp__search", status: "running" },
            { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        ],
        pendingApproval: { approvalId: "appr-1", details: approvalDetails },
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "tool_completed", toolName: "mcp__search", status: "success" }));
    const card = next.timeline?.find((it) => it.kind === "approval");
    strict_1.default.ok(card, "approval card should still be in timeline");
    strict_1.default.equal(card?.decision, "resolved", "card should be stamped as resolved");
    strict_1.default.equal(next.pendingApproval, undefined, "pendingApproval should be cleared");
    strict_1.default.equal(next.status, "running");
});
(0, node_test_1.default)("history replay: turn_completed after permission_required stamps card resolved and clears pendingApproval", () => {
    const state = approvalState({
        status: "waiting_approval",
        timeline: [
            { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        ],
        pendingApproval: { approvalId: "appr-1", details: approvalDetails },
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "turn_completed", finalMessage: "done" }));
    const card = next.timeline?.find((it) => it.kind === "approval");
    strict_1.default.equal(card?.decision, "resolved", "card should be stamped as resolved");
    strict_1.default.equal(next.pendingApproval, undefined, "pendingApproval should be cleared");
    strict_1.default.equal(next.status, "completed");
});
(0, node_test_1.default)("live run: no stale detection when pendingApproval is undefined before tool_completed", () => {
    // In a live run approve() clears pendingApproval synchronously, so by the time
    // any server event arrives pendingApproval is already undefined.
    const state = approvalState({
        status: "running",
        timeline: [
            { kind: "tool", id: "tool-1", toolName: "mcp__search", status: "running" },
            { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails, decision: "approve" },
        ],
        pendingApproval: undefined,
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "tool_completed", toolName: "mcp__search", status: "success" }));
    // pendingApproval was already undefined — should remain undefined, no side effects
    strict_1.default.equal(next.pendingApproval, undefined);
    // The already-resolved card should still have its decision intact
    const card = next.timeline?.find((it) => it.kind === "approval");
    strict_1.default.equal(card?.decision, "approve", "existing decision should not be changed");
});
(0, node_test_1.default)("new permission_required while previous pendingApproval is set stamps the first and sets the second", () => {
    // Two consecutive approval gates in replay: first is stale, second permission_required
    // fires — staleApproval stamps first card; ...extra from the switch overrides
    // pendingApproval back to the new approval value.
    const state = approvalState({
        status: "waiting_approval",
        timeline: [
            { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        ],
        pendingApproval: { approvalId: "appr-1", details: approvalDetails },
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "permission_required", approvalId: "appr-2", provider: "codex", details: approvalDetails }));
    const firstCard = next.timeline?.find((it) => it.kind === "approval" && it.approvalId === "appr-1");
    strict_1.default.equal(firstCard?.decision, "resolved", "first approval card should be stamped by stale detection");
    strict_1.default.deepEqual(next.pendingApproval, { approvalId: "appr-2", details: approvalDetails }, "pendingApproval should point to the new approval");
});
// Question stale detection tests
const questionOptions = [
    { label: "Python", value: "Python" },
    { label: "TypeScript", value: "TypeScript" },
];
(0, node_test_1.default)("user_question_required adds question card and sets pendingQuestion", () => {
    const state = approvalState();
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "user_question_required", questionId: "q-1", prompt: "Pick one", options: questionOptions }));
    const card = next.timeline?.find((it) => it.kind === "question");
    strict_1.default.ok(card, "question card should be added");
    strict_1.default.equal(card.answer, undefined);
    strict_1.default.ok(next.pendingQuestion, "pendingQuestion should be set");
    strict_1.default.equal(next.status, "waiting_question");
});
(0, node_test_1.default)("history replay: follow-up event after user_question_required stamps card as answered and clears pendingQuestion", () => {
    const state = approvalState({
        status: "waiting_question",
        timeline: [
            { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
        ],
        pendingQuestion: { questionId: "q-1", prompt: "Pick one", options: questionOptions },
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "turn_completed", finalMessage: "done" }));
    const card = next.timeline?.find((it) => it.kind === "question");
    strict_1.default.equal(card?.answer, "answered", "question card should be stamped as answered");
    strict_1.default.equal(next.pendingQuestion, undefined, "pendingQuestion should be cleared");
});
(0, node_test_1.default)("history replay: permission_required after user_question_required stamps question and sets new approval", () => {
    // Question was answered before an approval gate fired — permission_required
    // should trigger stale question detection (no type guard prevents it).
    const state = approvalState({
        status: "waiting_question",
        timeline: [
            { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
        ],
        pendingQuestion: { questionId: "q-1", prompt: "Pick one", options: questionOptions },
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "permission_required", approvalId: "appr-1", provider: "codex", details: approvalDetails }));
    const qCard = next.timeline?.find((it) => it.kind === "question");
    strict_1.default.equal(qCard?.answer, "answered", "question card should be stamped when approval gate fires after it");
    strict_1.default.equal(next.pendingQuestion, undefined, "pendingQuestion should be cleared");
    strict_1.default.ok(next.pendingApproval, "pendingApproval should be set for the new approval");
});
(0, node_test_1.default)("live run: no stale detection for question when pendingQuestion is already undefined", () => {
    // answer() clears pendingQuestion synchronously, so it is undefined by the time
    // any server event arrives during a live run.
    const state = approvalState({
        status: "running",
        timeline: [
            { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions, answer: "Python" },
        ],
        pendingQuestion: undefined,
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "turn_completed", finalMessage: "done" }));
    const card = next.timeline?.find((it) => it.kind === "question");
    strict_1.default.equal(card?.answer, "Python", "existing answer should not be overwritten");
    strict_1.default.equal(next.pendingQuestion, undefined);
});
// UC2: history replay — denied approval (BUG-074 regression guard)
// The user denied an approval in a prior session; when history is replayed the
// event stream only contains permission_required (no decision event was persisted).
// The card must be stamped "resolved" — a neutral sentinel — never "approved" even
// though the run did continue after the gate.
(0, node_test_1.default)("history replay: denied approval is stamped resolved not approved (BUG-074)", () => {
    const state = approvalState({
        status: "waiting_approval",
        timeline: [
            { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        ],
        pendingApproval: { approvalId: "appr-1", details: approvalDetails },
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "tool_completed", toolName: "mcp__search", status: "success" }));
    const card = next.timeline?.find((it) => it.kind === "approval");
    strict_1.default.equal(card?.decision, "resolved", "sentinel must be neutral resolved");
    strict_1.default.notEqual(card?.decision, "approved", "must not claim approved — original decision may have been deny");
    strict_1.default.notEqual(card?.decision, "deny", "must not claim deny — decision is not persisted in the stream");
    strict_1.default.equal(next.pendingApproval, undefined, "pendingApproval should be cleared");
});
// Transcript replay tests
(0, node_test_1.default)("history replay: turn_started{prompt} adds a prompt bubble before the assistant response", () => {
    // Simulates the event sequence emitted by loadClaudeTranscriptEvents /
    // loadCodexTranscriptEvents on resume: a user prompt event followed by
    // the assistant message. Both must appear in the timeline in correct order.
    const empty = { status: "idle", timeline: [], recoverable: false };
    const afterPrompt = (0, timelineReducer_1.applyTimelineEvent)(empty, baseEvent({ type: "turn_started", providerTurnId: "replay-prompt-1", prompt: "hello there" }));
    const afterAssistant = (0, timelineReducer_1.applyTimelineEvent)({ ...empty, ...afterPrompt }, baseEvent({ type: "message_completed", text: "Hi! How can I help?" }));
    const timeline = afterAssistant.timeline ?? [];
    const promptItem = timeline.find((it) => it.kind === "prompt");
    const assistantItem = timeline.find((it) => it.kind === "assistant");
    strict_1.default.ok(promptItem, "prompt bubble should be present");
    strict_1.default.ok(assistantItem, "assistant bubble should be present");
    strict_1.default.equal(promptItem.text, "hello there");
    strict_1.default.equal(assistantItem.text, "Hi! How can I help?");
    const promptIdx = timeline.indexOf(promptItem);
    const assistantIdx = timeline.indexOf(assistantItem);
    strict_1.default.ok(promptIdx < assistantIdx, "prompt bubble must precede assistant bubble");
});
(0, node_test_1.default)("history replay: hasPendingPrompt prevents double-render when turn_started{prompt} fires on live turn", () => {
    // During a live turn the desktop already pushed the prompt optimistically.
    // If turn_started also carries prompt (it does not today, but guard must hold),
    // the second push must be de-duped.
    const stateWithPrompt = {
        status: "running",
        timeline: [{ kind: "prompt", id: "prompt-0", text: "hello there" }],
        recoverable: false,
    };
    const after = (0, timelineReducer_1.applyTimelineEvent)(stateWithPrompt, baseEvent({ type: "turn_started", providerTurnId: "replay-prompt-1", prompt: "hello there" }));
    const prompts = (after.timeline ?? []).filter((it) => it.kind === "prompt");
    strict_1.default.equal(prompts.length, 1, "must not double-render an already-present prompt");
});
(0, node_test_1.default)("history replay: turn_completed after replay removes thinking row", () => {
    // seedTranscriptFromDisk appends a synthetic turn_completed to close the
    // trailing Thinking... row that finalize() would inject after message_completed.
    const empty = { status: "idle", timeline: [], recoverable: false };
    let state = { ...empty };
    for (const e of [
        baseEvent({ type: "turn_started", providerTurnId: "replay-prompt-1", prompt: "hi" }),
        baseEvent({ type: "message_completed", text: "hello" }),
        baseEvent({ type: "turn_completed", finalMessage: "hello" }),
    ]) {
        state = { ...state, ...(0, timelineReducer_1.applyTimelineEvent)(state, e) };
    }
    strict_1.default.equal(state.timeline.some((it) => it.kind === "thinking"), false, "no Thinking... row after turn_completed");
    strict_1.default.equal(state.status, "completed");
});
// UC5 complement: live deny — deny decision is preserved, stale detection does not fire
// When the user clicks Deny in a live run, approve()/deny() stamps decision: "deny"
// and clears pendingApproval synchronously. The next server event must NOT overwrite
// the real decision with the "resolved" sentinel.
(0, node_test_1.default)("live run: deny decision is preserved after subsequent events (BUG-074)", () => {
    const state = approvalState({
        status: "running",
        timeline: [
            { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails, decision: "deny" },
        ],
        pendingApproval: undefined,
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)(state, baseEvent({ type: "turn_completed", finalMessage: "done" }));
    const card = next.timeline?.find((it) => it.kind === "approval");
    strict_1.default.equal(card?.decision, "deny", "live deny decision must not be overwritten by stale detection");
    strict_1.default.equal(next.pendingApproval, undefined);
});
