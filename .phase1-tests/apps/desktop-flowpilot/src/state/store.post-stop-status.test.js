"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
function stoppedSnap() {
    return {
        parentRunId: "run-1",
        runs: [
            {
                runId: "child-1",
                agentName: "reviewer",
                role: "reviewer",
                status: "cancelled",
                parentRunId: "run-1",
                createdAt: "2026-07-21T00:00:00Z",
            },
        ],
        edges: [],
        busMessages: [],
        loopState: { status: "stopped", round: 0, roundCap: 3, gateReason: "stopped" },
    };
}
// BUG-308 residual: after Stop, loop stays "stopped" while plain-chat follow-up
// completes. Header/history must show Completed, not stay Cancelled.
(0, node_test_1.default)("stopped loop preserves completed after post-Stop chat follow-up", () => {
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("completed", stoppedSnap()), "completed");
});
(0, node_test_1.default)("stopped loop preserves failed after post-Stop chat follow-up", () => {
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("failed", stoppedSnap()), "failed");
});
(0, node_test_1.default)("stopped loop still forces cancelled when chat has not continued (BUG-248)", () => {
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", stoppedSnap()), "cancelled");
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("cancelled", stoppedSnap()), "cancelled");
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("idle", stoppedSnap()), "cancelled");
});
