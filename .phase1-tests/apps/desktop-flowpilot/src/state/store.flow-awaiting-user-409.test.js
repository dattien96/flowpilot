"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
/**
 * run-63960: after restart, freeform chat while loop is blocked returns
 * flow_awaiting_user 409. Desktop must map that to status=blocked (not Failed)
 * and refresh agent graph so FlowAwaitingUserCard (Continue/Stop) can render.
 *
 * additive-tests-only + cross-provider-parity (shared UI path).
 */
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
const HttpWsRunnerClient_1 = require("../client/HttpWsRunnerClient");
async function* emptyStream() { }
function blockedGraph(parentRunId, providerKey) {
    return {
        parentRunId,
        runs: [
            {
                runId: "child-coder",
                agentName: "coder",
                role: "coder",
                status: "completed",
                parentRunId,
                createdAt: "2026-07-23T16:19:00Z",
                providerKey,
            },
        ],
        edges: [],
        busMessages: [],
        loopState: {
            status: "blocked",
            round: 3,
            roundCap: 3,
            blockReason: "cap",
            openIssues: 2,
            gateReason: "cap 3 reached with 2 open issue(s)",
        },
    };
}
function makeClient(overrides = {}) {
    const base = {
        switchChatProvider: async () => {
            throw new Error("switchChatProvider not implemented in this fixture");
        },
        chatTimeline: async () => ({ chatId: "", legs: [], records: [], nextSeq: 0, truncated: false, degraded: false }),
        listProjects: async () => [],
        listWorkflows: async () => [],
        listSteps: async () => [],
        listProviderAccounts: async () => [],
        listRunHistory: async () => [],
        listRemoteChatSessions: async () => [],
        listAgents: async () => [],
        listAgentRuns: async () => [],
        refreshAgentGraph: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 0, roundCap: 3 },
        }),
        pauseAgentLoop: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "paused", round: 0, roundCap: 3 },
        }),
        resumeAgentLoop: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 0, roundCap: 3 },
        }),
        injectAgentFeedback: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 0, roundCap: 3 },
        }),
        stopAgentLoop: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "stopped", round: 0, roundCap: 3 },
        }),
        spawnAgent: async () => ({
            runId: "agent-1",
            providerSessionId: "session-agent",
            providerKey: "codex",
            status: "completed",
        }),
        startRun: async () => ({
            runId: "new-run",
            providerSessionId: "session-1",
            providerKey: "codex",
            status: "running",
        }),
        resumeRun: async (runId) => ({
            runId,
            providerSessionId: "session-1",
            providerKey: "codex",
            status: "blocked",
        }),
        handoffContext: async (runId, input) => ({
            sourceRunId: runId,
            sourceProviderKey: "codex",
            targetProviderKey: input.targetProviderKey,
            prompt: "[mock handoff]",
            includedTurnCount: 0,
            omittedTurnCount: 0,
            truncated: false,
            handoffMode: "raw",
        }),
        generateChatSummary: async (runId) => ({ runId, generated: true, skipped: false }),
        syncChatRun: async (runId) => ({
            runId,
            sourceMachineId: "mch",
            sourceRunId: runId,
            syncStatus: "synced",
            syncedAt: "2026-07-23T16:00:00Z",
            remotePath: "x",
        }),
        deleteRun: async () => { },
        restoreChatRun: async (input) => ({
            runId: input.sourceRunId,
            sourceMachineId: input.sourceMachineId,
            sourceRunId: input.sourceRunId,
            providerKey: "codex",
            restoreStatus: "restored",
        }),
        sendTurn: () => emptyStream(),
        submitApproval: async () => { },
        answerQuestion: async () => { },
        interrupt: async () => { },
        streamRun: () => emptyStream(),
        focusAgentRun: () => emptyStream(),
        listArtifacts: async () => [],
        listSkills: async () => [],
        connectProviderAccount: async () => { },
        activateProviderAccount: async () => { },
        applyGrokYoloPosture: async () => { },
        openProviderAccountTerminal: async () => { },
        restartStack: async () => { },
        shutdownStack: async () => { },
    };
    return { ...base, ...overrides };
}
function seedBlockedRun(client) {
    store_1.useStore.setState({
        client,
        projects: [],
        workflows: [],
        steps: [],
        skills: [],
        providerAccounts: [],
        supportedModels: [],
        selectedProjectId: "project-1",
        selectedWorkflowId: undefined,
        selectedStepId: undefined,
        launchMode: "workflow",
        chatMode: "normal_chat",
        selectedProvider: "codex",
        selectedModel: undefined,
        reasoningEffort: undefined,
        yoloMode: false,
        chatStartMode: "normal",
        chatSourceDocId: "",
        runId: "run-63960",
        mainRunId: "run-63960",
        activeAgentRunId: undefined,
        agentRuns: [],
        agentBusMessages: [],
        activeStepId: "chat-run-63960",
        status: "blocked",
        timeline: [{ kind: "thinking", id: "thinking-stale", text: "Thinking..." }],
        artifacts: [],
        pendingApprovals: [],
        pendingQuestions: [],
        gateBlock: undefined,
        latestTokenUsage: undefined,
        lastTurnInput: undefined,
        recoverable: false,
        runHistory: [],
        agentGraphSnapshot: undefined,
        agentSpawnGuideOpen: false,
        _runReplaySeq: {},
        _runSnapshots: {},
        _historyReplaying: false,
        _streamRunSeq: 0,
        _agentGraphLoadSeq: 0,
    });
}
for (const providerKey of ["codex", "claude", "grok"]) {
    (0, node_test_1.default)(`sendPrompt maps flow_awaiting_user 409 to blocked + graph refresh for ${providerKey}`, async () => {
        let refreshCalls = 0;
        const graph = blockedGraph("run-63960", providerKey);
        seedBlockedRun(makeClient({
            sendTurn: () => (async function* () {
                throw new HttpWsRunnerClient_1.RunnerApiError(409, "flow_awaiting_user", "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn");
            })(),
            refreshAgentGraph: async () => {
                refreshCalls += 1;
                return graph;
            },
        }));
        await store_1.useStore.getState().sendPrompt("ok");
        // Allow best-effort graph refresh promise to settle.
        await new Promise((r) => setTimeout(r, 30));
        const state = store_1.useStore.getState();
        strict_1.default.equal(state.status, "blocked", "must not map awaiting-user to Failed");
        strict_1.default.equal(state.recoverable, false);
        strict_1.default.ok(!state.timeline.some((it) => it.kind === "thinking"), "stale Thinking must clear");
        strict_1.default.ok(state.timeline.some((it) => it.kind === "system" && it.tone === "warn" && /waiting for your decision/i.test(it.text)), "warn system row should explain awaiting user");
        strict_1.default.ok(refreshCalls >= 1, "must refresh agent graph so Continue/Stop card has loopState");
        strict_1.default.equal(state.agentGraphSnapshot?.loopState.status, "blocked");
        strict_1.default.equal(state.agentGraphSnapshot?.loopState.blockReason, "cap");
    });
}
(0, node_test_1.default)("other 409 codes still map to failed (non-regression)", async () => {
    seedBlockedRun(makeClient({
        sendTurn: () => (async function* () {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "invalid_request", "bad prompt shape");
        })(),
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.status, "failed");
    strict_1.default.ok(state.timeline.some((it) => it.kind === "system" && it.tone === "error"));
});
(0, node_test_1.default)("non-409 errors with decision wording still fail (no false blocked)", async () => {
    seedBlockedRun(makeClient({
        sendTurn: () => (async function* () {
            throw new HttpWsRunnerClient_1.RunnerApiError(500, "internal", "waiting for your decision later");
        })(),
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    strict_1.default.equal(store_1.useStore.getState().status, "failed");
});
(0, node_test_1.default)("openHistoryRun seeds agent graph early for blocked resume", async () => {
    let refreshCalls = 0;
    const graph = blockedGraph("run-63960", "codex");
    const client = makeClient({
        resumeRun: async (runId) => ({
            runId,
            providerSessionId: "session-1",
            providerKey: "codex",
            status: "blocked",
        }),
        refreshAgentGraph: async () => {
            refreshCalls += 1;
            return graph;
        },
        streamRun: () => emptyStream(),
    });
    store_1.useStore.setState({
        client,
        runHistory: [
            {
                runId: "run-63960",
                projectId: "project-1",
                status: "blocked",
                providerKey: "codex",
                startedAt: "2026-07-23T16:00:00Z",
                updatedAt: "2026-07-23T16:30:00Z",
                runKind: "chat",
                subMode: "bug",
            },
        ],
        selectedProjectId: "project-1",
        runId: undefined,
        mainRunId: undefined,
        agentGraphSnapshot: undefined,
        status: "idle",
        timeline: [],
        _runReplaySeq: {},
        _runSnapshots: {},
        _historyReplaying: false,
        _streamRunSeq: 0,
        _agentGraphLoadSeq: 0,
    });
    await store_1.useStore.getState().openHistoryRun("run-63960");
    await new Promise((r) => setTimeout(r, 40));
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.status, "blocked");
    strict_1.default.ok(refreshCalls >= 1, "early refreshAgentGraph on history open");
    strict_1.default.equal(state.agentGraphSnapshot?.loopState.status, "blocked");
});
(0, node_test_1.default)("late blocked HTTP graph refresh does not overwrite post-Continue graph", async () => {
    let resolveRefresh;
    const refreshPromise = new Promise((r) => {
        resolveRefresh = r;
    });
    seedBlockedRun(makeClient({
        sendTurn: () => (async function* () {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "flow_awaiting_user", "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn");
        })(),
        refreshAgentGraph: async () => refreshPromise,
        continueFlow: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 4, roundCap: 6 },
        }),
    }));
    const sendP = store_1.useStore.getState().sendPrompt("ok");
    await sendP;
    // Continue bumps graph load seq before late refresh resolves.
    await store_1.useStore.getState().continueFlow("more rounds");
    resolveRefresh(blockedGraph("run-63960", "codex"));
    await new Promise((r) => setTimeout(r, 30));
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.agentGraphSnapshot?.loopState.status, "running");
    strict_1.default.notEqual(state.agentGraphSnapshot?.loopState.status, "blocked");
});
(0, node_test_1.default)("after flow_awaiting_user, stop remains available and seals loop", async () => {
    seedBlockedRun(makeClient({
        sendTurn: () => (async function* () {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "flow_awaiting_user", "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn");
        })(),
        refreshAgentGraph: async () => blockedGraph("run-63960", "codex"),
        stopAgentLoop: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "stopped", round: 3, roundCap: 3, gateReason: "stopped" },
        }),
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    await new Promise((r) => setTimeout(r, 30));
    strict_1.default.equal(store_1.useStore.getState().status, "blocked");
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.status, "blocked");
    await store_1.useStore.getState().stop();
    const after = store_1.useStore.getState();
    strict_1.default.equal(after.status, "cancelled");
    strict_1.default.equal(after.agentGraphSnapshot?.loopState.status, "stopped");
    strict_1.default.ok(!after.timeline.some((it) => it.kind === "thinking"));
});
(0, node_test_1.default)("after flow_awaiting_user, continueFlow unparks to running", async () => {
    seedBlockedRun(makeClient({
        sendTurn: () => (async function* () {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "flow_awaiting_user", "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn");
        })(),
        refreshAgentGraph: async () => blockedGraph("run-63960", "codex"),
        continueFlow: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 4, roundCap: 5 },
        }),
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    await new Promise((r) => setTimeout(r, 30));
    await store_1.useStore.getState().continueFlow("extend");
    const after = store_1.useStore.getState();
    strict_1.default.equal(after.agentGraphSnapshot?.loopState.status, "running");
});
