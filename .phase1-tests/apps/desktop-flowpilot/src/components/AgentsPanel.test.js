"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const agentDependencies_1 = require("./agentDependencies");
const runs = [
    {
        runId: "run-reviewer",
        agentName: "reviewer",
        role: "reviewer",
        status: "completed",
        createdAt: "2026-06-20T00:00:00Z",
    },
    {
        runId: "run-tester",
        agentName: "tester",
        role: "tester",
        status: "starting",
        createdAt: "2026-06-20T00:00:01Z",
    },
];
(0, node_test_1.default)("formatDependencyLabels resolves dependency run ids to agent names", () => {
    strict_1.default.deepEqual((0, agentDependencies_1.formatDependencyLabels)(["run-reviewer", "missing-run"], runs), ["reviewer", "missing-run"]);
});
