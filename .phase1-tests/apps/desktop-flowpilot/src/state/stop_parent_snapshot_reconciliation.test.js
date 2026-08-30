"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
const HttpWsRunnerClient_1 = require("../client/HttpWsRunnerClient");
const snapshot = (status) => ({
    timeline: [{ kind: "thinking", id: `thinking-${status}`, text: "Thinking..." }],
    artifacts: [],
    status,
    pendingApprovals: [],
    pendingQuestions: [],
    recoverable: true,
});
(0, node_test_1.default)("Stop from a child-focused flow reconciles every saved run snapshot to the terminal graph", async () => {
    const calls = [];
    const current = store_1.useStore.getState();
    const client = {
        ...current.client,
        stopAgentLoop: async (runId) => {
            calls.push(`loop:${runId}`);
            return {
                parentRunId: runId,
                runs: [
                    { runId: "child-cancelled", agentName: "coder", role: "coder", status: "cancelled", parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
                    { runId: "child-completed", agentName: "reviewer", role: "reviewer", status: "completed", parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
                    { runId: "child-failed", agentName: "reviewer", role: "reviewer", status: "failed", parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
                    // Models the asynchronous finishTurn window: the loop is stopped but
                    // a stale graph can still report a child as active for one event.
                    { runId: "child-stale-running", agentName: "reviewer", role: "reviewer", status: "running", parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
                    { runId: "child-stale-approval", agentName: "reviewer", role: "reviewer", status: "waiting_approval", parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
                    { runId: "child-stale-question", agentName: "reviewer", role: "reviewer", status: "waiting_question", parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
                ],
                edges: [],
                busMessages: [],
                loopState: { status: "stopped", round: 1, roundCap: 3 },
            };
        },
        interrupt: async (runId) => {
            calls.push(`interrupt:${runId}`);
        },
    };
    store_1.useStore.setState({
        client,
        chatMode: "normal_chat",
        runId: "child-cancelled",
        mainRunId: "parent-run",
        activeAgentRunId: "child-cancelled",
        status: "running",
        timeline: [{ kind: "thinking", id: "focused-child-thinking", text: "Thinking..." }],
        agentRuns: [{ runId: "child-cancelled", agentName: "coder", role: "coder", status: "running", parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
        agentGraphSnapshot: {
            parentRunId: "parent-run",
            runs: [{ runId: "child-cancelled", agentName: "coder", role: "coder", status: "running", parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 1, roundCap: 3 },
        },
        _runSnapshots: {
            "parent-run": snapshot("running"),
            "child-cancelled": snapshot("running"),
            "child-completed": snapshot("completed"),
            "child-failed": snapshot("failed"),
            "child-stale-running": snapshot("running"),
            "child-stale-approval": snapshot("waiting_approval"),
            "child-stale-question": snapshot("waiting_question"),
        },
    });
    await store_1.useStore.getState().stop();
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["parent-run"]?.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["child-cancelled"]?.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["child-completed"]?.status, "completed");
    strict_1.default.equal(state._runSnapshots["child-failed"]?.status, "failed");
    strict_1.default.equal(state._runSnapshots["child-stale-running"]?.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["child-stale-approval"]?.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["child-stale-question"]?.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["parent-run"]?.timeline.some((item) => item.kind === "thinking"), false);
    strict_1.default.equal(state._runSnapshots["child-cancelled"]?.recoverable, false);
    strict_1.default.deepEqual(calls, [
        "loop:parent-run",
        "interrupt:parent-run",
        "interrupt:child-cancelled",
        "interrupt:child-completed",
        "interrupt:child-failed",
        "interrupt:child-stale-running",
        "interrupt:child-stale-approval",
        "interrupt:child-stale-question",
    ]);
});
(0, node_test_1.default)("Stop reconciles saved snapshots when the runner returns a stopped graph in a durable-checkpoint error", async () => {
    const current = store_1.useStore.getState();
    const embeddedSnapshot = {
        parentRunId: "parent-run",
        runs: [{ runId: "child-run", agentName: "coder", role: "coder", status: "cancelled", parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
        edges: [],
        busMessages: [],
        loopState: { status: "stopped", round: 1, roundCap: 3 },
    };
    const client = {
        ...current.client,
        stopAgentLoop: async () => {
            throw new HttpWsRunnerClient_1.RunnerApiError(500, "persist_failed", "RAM stop succeeded but checkpoint failed", embeddedSnapshot);
        },
        interrupt: async () => { },
    };
    store_1.useStore.setState({
        client,
        chatMode: "normal_chat",
        runId: "child-run",
        mainRunId: "parent-run",
        activeAgentRunId: "child-run",
        status: "running",
        agentRuns: [{ runId: "child-run", agentName: "coder", role: "coder", status: "running", parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
        agentGraphSnapshot: { ...embeddedSnapshot, loopState: { status: "running", round: 1, roundCap: 3 } },
        _runSnapshots: {
            "parent-run": snapshot("running"),
            "child-run": snapshot("running"),
        },
    });
    await store_1.useStore.getState().stop();
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["parent-run"]?.status, "cancelled");
    strict_1.default.equal(state._runSnapshots["child-run"]?.status, "cancelled");
    strict_1.default.equal(state.agentGraphSnapshot?.loopState.status, "stopped");
});
