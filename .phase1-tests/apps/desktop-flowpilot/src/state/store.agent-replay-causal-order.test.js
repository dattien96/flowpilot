"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
function event(providerKey, seq, type, occurredAt) {
    return {
        id: `event-${seq}`,
        seq,
        type,
        workflowRunId: "run-1264",
        providerSessionId: "session-1",
        providerKey,
        occurredAt,
        agentName: type === "agent_spawned_by_user" ? `agent-${seq}` : undefined,
        childRunId: type === "agent_spawned_by_user" ? `child-${seq}` : undefined,
        text: type === "message_completed" ? "Both reviewers approved with no conflicts." : undefined,
    };
}
for (const providerKey of ["codex", "claude", "grok"]) {
    (0, node_test_1.default)(`history replay keeps ${providerKey} agent cards ahead of a fallback-timestamp result`, () => {
        const ordered = (0, store_1.orderHistoryReplayEvents)([
            event(providerKey, 1, "turn_started", "2026-07-18T14:46:22.260537Z"),
            event(providerKey, 5, "agent_spawned_by_user", "2026-07-18T14:46:24.311119Z"),
            event(providerKey, 6, "agent_spawned_by_user", "2026-07-18T14:47:10.991123Z"),
            event(providerKey, 7, "agent_spawned_by_user", "2026-07-18T14:47:11.507457Z"),
            event(providerKey, 8, "message_completed", "2026-07-18T14:46:22.260537Z"),
        ]);
        strict_1.default.deepEqual(ordered.map((item) => item.seq), [1, 5, 6, 7, 8]);
    });
}
