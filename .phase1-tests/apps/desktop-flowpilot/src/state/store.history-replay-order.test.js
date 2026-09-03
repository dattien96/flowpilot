"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
function event(providerKey, seq, occurredAt, type) {
    return {
        id: `evt-${seq}`,
        seq,
        type,
        workflowRunId: "run-2383",
        providerKey,
        occurredAt,
    };
}
for (const providerKey of ["codex", "claude", "grok"]) {
    (0, node_test_1.default)(`history replay orders recovered lifecycle events for ${providerKey}`, () => {
        const ordered = (0, store_1.orderHistoryReplayEvents)([
            event(providerKey, 12, "2026-07-19T14:37:38.334334Z", "message_completed"),
            event(providerKey, 7, "2026-07-19T14:37:40.243014Z", "agent_spawned_by_user"),
            event(providerKey, 17, "2026-07-19T14:41:39.251364Z", "agent_result_injected"),
            event(providerKey, 8, "2026-07-19T14:38:54.766468Z", "agent_spawned_by_user"),
            event(providerKey, 18, "2026-07-19T14:40:22.833513Z", "agent_result_injected"),
        ]);
        strict_1.default.deepEqual(ordered.map((item) => item.seq), [12, 7, 8, 18, 17]);
    });
    (0, node_test_1.default)(`completed replay clears every running tool for ${providerKey}`, () => {
        const timeline = [
            { kind: "tool", id: "search", toolName: "search_tool", status: "running" },
            { kind: "tool", id: "review", toolName: "flowpilot__submit_review_outcome", status: "running" },
            { kind: "thinking", id: "thinking", text: "Thinking..." },
        ];
        strict_1.default.deepEqual((0, store_1.settleCompletedFlowTimeline)(timeline), [
            { kind: "tool", id: "search", toolName: "search_tool", status: "success" },
            { kind: "tool", id: "review", toolName: "flowpilot__submit_review_outcome", status: "success" },
        ]);
    });
}
