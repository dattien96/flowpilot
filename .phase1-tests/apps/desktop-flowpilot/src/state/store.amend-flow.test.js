"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
/**
 * Task-309: amendFlow widens frozen contract after scope drift and resumes.
 * additive-tests-only — new file.
 */
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
async function* emptyStream() { }
const BASE_EVENT = {
    id: "ev-1",
    workflowRunId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    seq: 1,
    occurredAt: "2026-01-01T00:00:00Z",
};
async function* childFocusStream() {
    yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
    yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_completed", finalMessage: "child response" };
}
function loopSnapshot(status, extra = {}) {
    return {
        parentRunId: "current-run",
        runs: [],
        edges: [],
        busMessages: [],
        loopState: { status, round: 1, roundCap: 3, ...extra },
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
        refreshAgentGraph: async () => loopSnapshot("running"),
        pauseAgentLoop: async () => loopSnapshot("paused"),
        resumeAgentLoop: async () => loopSnapshot("running"),
        injectAgentFeedback: async () => loopSnapshot("running"),
        stopAgentLoop: async () => loopSnapshot("stopped"),
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
            status: "completed",
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
function seedStore(client) {
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
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        agentRuns: [],
        agentBusMessages: [],
        activeStepId: "chat-current-run",
        status: "blocked",
        timeline: [],
        artifacts: [],
        pendingApprovals: [],
        pendingQuestions: [],
        gateBlock: undefined,
        latestTokenUsage: undefined,
        lastTurnInput: undefined,
        recoverable: false,
        runHistory: [],
        agentGraphSnapshot: loopSnapshot("blocked", { blockReason: "escalate" }),
        agentSpawnGuideOpen: false,
        _runReplaySeq: {},
        _runSnapshots: {},
        _historyReplaying: false,
        _streamRunSeq: 0,
        _agentGraphLoadSeq: 0,
        workflowStepRuntime: undefined,
        workflowStepRuntimeMeta: undefined,
        workflowStepRuntimeLoading: false,
        _workflowStepRuntimeLoadSeq: 0,
    });
}
(0, node_test_1.default)("amendFlow calls client.amendFlow with paths and applies running snapshot", async () => {
    const seen = {};
    seedStore(makeClient({
        amendFlow: async (parentRunId, paths) => {
            seen.parentRunId = parentRunId;
            seen.paths = paths;
            return loopSnapshot("running");
        },
    }));
    await store_1.useStore.getState().amendFlow(["calc_test.go"]);
    strict_1.default.equal(seen.parentRunId, "current-run");
    strict_1.default.deepEqual(seen.paths, ["calc_test.go"]);
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.status, "running");
});
(0, node_test_1.default)("amendFlow is a no-op when the client does not implement it", async () => {
    seedStore(makeClient({ amendFlow: undefined }));
    await store_1.useStore.getState().amendFlow(["calc_test.go"]);
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.status, "blocked");
});
(0, node_test_1.default)("amendFlow returns to the main run before amending when a child agent is focused", async () => {
    const seen = {};
    seedStore(makeClient({
        focusAgentRun: () => childFocusStream(),
        amendFlow: async (parentRunId, paths) => {
            seen.parentRunId = parentRunId;
            strict_1.default.deepEqual(paths, ["calc_test.go"]);
            return loopSnapshot("running");
        },
    }));
    store_1.useStore.setState({
        timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
    });
    await store_1.useStore.getState().focusAgentRun("child-run");
    await new Promise((resolve) => setTimeout(resolve, 0));
    strict_1.default.equal(store_1.useStore.getState().activeAgentRunId, "child-run");
    await store_1.useStore.getState().amendFlow(["calc_test.go"]);
    strict_1.default.equal(seen.parentRunId, "current-run");
    strict_1.default.equal(store_1.useStore.getState().runId, "current-run");
    strict_1.default.equal(store_1.useStore.getState().activeAgentRunId, undefined);
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.status, "running");
});
