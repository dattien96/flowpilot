"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
/**
 * run-24377: after resume, hub turn_started/message_completed from the durable
 * turn log often have empty occurredAt while agent_spawned_by_user carries child
 * wall-clock starts. The old sorter put timed events first → agent cards above
 * the original "fix bug" prompt.
 *
 * cross-provider-parity: Case 1 — same sorter for all providers.
 * additive-tests-only: new file only.
 */
function evt(providerKey, seq, type, occurredAt, extra = {}) {
    return {
        id: `e-${seq}`,
        seq,
        type,
        workflowRunId: "run-24377",
        providerKey,
        occurredAt,
        ...extra,
    };
}
for (const providerKey of ["codex", "claude", "grok"]) {
    (0, node_test_1.default)(`history replay keeps untimed hub prompt before timed agents (${providerKey})`, () => {
        const ordered = (0, store_1.orderHistoryReplayEvents)([
            evt(providerKey, 1, "turn_started", "", { prompt: "fix bug 1 + 1 != 2" }),
            evt(providerKey, 2, "agent_spawned_by_user", "2026-07-21T14:48:57.806588Z", {
                agentName: "coder",
                childRunId: "run-24382",
            }),
            evt(providerKey, 3, "agent_result_injected", "2026-07-21T14:50:00Z", {
                childRunId: "run-24382",
                finalMessage: "done",
            }),
            evt(providerKey, 4, "message_completed", "", { text: "Round 0: changes requested" }),
            evt(providerKey, 5, "turn_started", "", { prompt: "done rồi hả, trả lời ok or not." }),
        ]);
        strict_1.default.deepEqual(ordered.map((item) => item.seq), [1, 2, 3, 4, 5], "must preserve Seq when any occurredAt is missing — not dump timed agents first");
        strict_1.default.equal(ordered[0].type, "turn_started");
        strict_1.default.equal(ordered[0].prompt, "fix bug 1 + 1 != 2");
    });
}
