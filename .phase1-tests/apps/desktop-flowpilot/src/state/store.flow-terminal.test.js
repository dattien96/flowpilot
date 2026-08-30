"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
for (const providerKey of ["codex", "claude", "grok"]) {
    (0, node_test_1.default)(`done flow settles terminal timeline residue for ${providerKey}`, () => {
        const snapshot = {
            parentRunId: "run-flow",
            runs: [
                {
                    runId: "child-1",
                    agentName: "reviewer",
                    role: "reviewer",
                    status: "running",
                    parentRunId: "run-flow",
                    createdAt: "2026-07-19T14:00:00Z",
                    providerKey,
                },
            ],
            edges: [],
            busMessages: [],
            loopState: { status: "done", round: 1, roundCap: 3 },
        };
        const timeline = [
            { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "running" },
            { kind: "thinking", id: "thinking-1", text: "Thinking..." },
        ];
        strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snapshot), "completed");
        strict_1.default.deepEqual((0, store_1.settleCompletedFlowTimeline)(timeline), [
            { kind: "tool", id: "tool-1", toolName: "flowpilot__submit_review_outcome", status: "success" },
        ]);
    });
}
