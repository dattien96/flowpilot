"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const Timeline_1 = require("./Timeline");
function run(runId, status) {
    return {
        runId,
        agentName: "coder",
        role: "coder",
        status,
        createdAt: "2026-07-19T14:00:00Z",
    };
}
(0, node_test_1.default)("live agent banner excludes a child already represented by its lifecycle card", () => {
    const timeline = [
        { kind: "agent", id: "agent-card-1", agentName: "coder", childRunId: "child-visible" },
    ];
    strict_1.default.deepEqual((0, Timeline_1.liveAgentRunsWithoutVisibleCard)([run("child-visible", "running"), run("child-paged-out", "waiting_approval"), run("child-done", "completed")], timeline).map((item) => item.runId), ["child-paged-out"]);
});
// The "back to main agent" crumb's green pulsing "live child run" badge must
// track the focused child's OWN status, not just "a child happens to be
// focused" — before this it kept pulsing green forever even after the child
// actually completed, disagreeing with the static "done" card the same run
// already shows in the Agents sidebar (AgentsPanel's active/closed split).
(0, node_test_1.default)("isFocusedChildLive is true while the focused child is still running or waiting", () => {
    strict_1.default.equal((0, Timeline_1.isFocusedChildLive)(run("child-1", "running")), true);
    strict_1.default.equal((0, Timeline_1.isFocusedChildLive)(run("child-1", "waiting_approval")), true);
    strict_1.default.equal((0, Timeline_1.isFocusedChildLive)(run("child-1", "waiting_question")), true);
});
(0, node_test_1.default)("isFocusedChildLive is false once the focused child reaches a terminal status", () => {
    strict_1.default.equal((0, Timeline_1.isFocusedChildLive)(run("child-1", "completed")), false);
    strict_1.default.equal((0, Timeline_1.isFocusedChildLive)(run("child-1", "failed")), false);
    strict_1.default.equal((0, Timeline_1.isFocusedChildLive)(run("child-1", "cancelled")), false);
});
(0, node_test_1.default)("isFocusedChildLive defaults to live when the run summary has not loaded yet", () => {
    strict_1.default.equal((0, Timeline_1.isFocusedChildLive)(undefined), true);
});
