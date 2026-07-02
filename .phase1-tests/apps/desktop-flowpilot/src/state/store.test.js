"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
const timelineReducer_1 = require("./timelineReducer");
const OrchestrationBoard_1 = require("@/components/OrchestrationBoard");
const Timeline_1 = require("@/components/Timeline");
const ChatInput_1 = require("@/components/ChatInput");
const MockRunnerClient_1 = require("../client/MockRunnerClient");
const HttpWsRunnerClient_1 = require("../client/HttpWsRunnerClient");
async function* emptyStream() { }
function makeClient(overrides = {}) {
    const base = {
        listProjects: async () => [],
        listWorkflows: async () => [],
        listSteps: async () => [],
        listProviderAccounts: async () => [],
        listRunHistory: async () => [],
        listRemoteChatSessions: async () => [],
        listAgents: async () => [],
        listAgentRuns: async () => [],
        refreshAgentGraph: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } }),
        pauseAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "paused", round: 0, roundCap: 3 } }),
        resumeAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } }),
        injectAgentFeedback: async (_parentRunId, _toRunId, message) => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [{ id: "bus-1", parentRunId: "current-run", kind: "user-feedback", message, queued: true, occurredAt: "2026-01-01T00:00:00Z" }], loopState: { status: "running", round: 0, roundCap: 3 } }),
        stopAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "stopped", round: 0, roundCap: 3 } }),
        spawnAgent: async () => ({ runId: "agent-1", providerSessionId: "session-agent", providerKey: "codex", status: "completed" }),
        startRun: async () => ({ runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running" }),
        resumeRun: async (runId) => ({ runId, providerSessionId: "session-1", providerKey: "codex", status: "completed" }),
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
        syncChatRun: async (runId) => ({ runId, sourceMachineId: "mch_sync", sourceRunId: runId, syncStatus: "synced", syncedAt: "2026-06-17T10:10:00Z", remotePath: "chat-sessions/runs/mch_sync/" + runId + "/manifest.json" }),
        deleteRun: async () => { },
        restoreChatRun: async (input) => ({ runId: input.sourceRunId, sourceMachineId: input.sourceMachineId, sourceRunId: input.sourceRunId, providerKey: "codex", restoreStatus: "restored" }),
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
        openProviderAccountTerminal: async () => { },
        restartStack: async () => { },
        shutdownStack: async () => { },
    };
    return { ...base, ...overrides };
}
function deferred() {
    let resolve;
    let reject;
    const promise = new Promise((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
}
function seedStore(client, runHistory) {
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
        selectedProvider: "claude",
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
        status: "running",
        timeline: [{ kind: "prompt", id: "prompt-1", text: "keep current timeline" }],
        artifacts: [],
        runHistory,
        historyLoading: false,
        historyLoadError: undefined,
        pendingApprovals: [],
        pendingQuestions: [],
        lastTurnInput: undefined,
        latestTokenUsage: undefined,
        recoverable: false,
        scenario: "normal",
        pendingAccountSwitch: undefined,
        accountSwitchLoading: false,
        pendingProviderSwitch: undefined,
        providerSwitchLoading: false,
        _accountSwitchTriedIds: [],
        _streamingAssistantId: undefined,
        _historyLoadSeq: 0,
        _streamRunSeq: 0,
        _runSnapshots: {},
        _runReplaySeq: {},
    });
}
function makeAccount(id, providerKey, overrides = {}) {
    return {
        id,
        providerKey,
        displayName: `Account ${id}`,
        displayLabel: `account-${id}`,
        homePath: `/home/${id}`,
        authStorePath: null,
        slotIndex: 0,
        authStatus: "connected",
        isActive: false,
        createdAt: "2026-01-01T00:00:00Z",
        lastAuthenticatedAt: null,
        accountEmail: `${id}@example.com`,
        accountName: null,
        usageSummary: null,
        remaining5hPercent: 80,
        remaining7dPercent: 80,
        remaining5hResetAt: null,
        remaining7dResetAt: null,
        usageSource: "provider_api",
        accessTokenExpiresAt: null,
        refreshTokenExpiresAt: null,
        refreshTokenExpiryNote: null,
        usageDetailLines: [],
        ...overrides,
    };
}
const BASE_EVENT = {
    id: "ev-1",
    workflowRunId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    seq: 1,
    occurredAt: "2026-01-01T00:00:00Z",
};
async function* turnFailedStream(error, recoverable, workflowRunId = "run-1") {
    yield { ...BASE_EVENT, workflowRunId, type: "turn_failed", error, recoverable };
}
async function* childFocusStream() {
    yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
    yield { ...BASE_EVENT, workflowRunId: "child-run", type: "message_delta", text: "child response" };
    yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_completed", finalMessage: "child response" };
}
async function* pendingChildStream(gate) {
    yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
    await gate;
    yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_completed", finalMessage: "done" };
}
async function* cursorChildStream() {
    yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 1, type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
    yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 2, type: "message_delta", text: "old child chunk" };
    yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 3, type: "message_delta", text: "new child chunk" };
}
(0, node_test_1.default)("openHistoryRun marks unavailable history entries on typed resume errors", async () => {
    seedStore(makeClient({
        resumeRun: async () => {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "session_unavailable", "session data not found on this machine");
        },
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runId, "current-run");
    strict_1.default.deepEqual(state.timeline, [{ kind: "prompt", id: "prompt-1", text: "keep current timeline" }]);
    strict_1.default.equal(state.runHistory[0]?.unavailableReason, "session data not found on this machine");
});
(0, node_test_1.default)("selectProject resets the active chat run when switching projects", async () => {
    seedStore(makeClient({ listSkills: async () => [] }), []);
    store_1.useStore.setState({
        selectedProjectId: "project-1",
        runId: "current-run",
        mainRunId: "current-run",
        activeStepId: "chat-current-run",
        status: "running",
        timeline: [{ kind: "prompt", id: "prompt-1", text: "old project prompt" }],
        artifacts: [{ id: "artifact-1", runId: "current-run", name: "Artifact", kind: "summary", createdAt: "2026-01-01T00:00:00Z" }],
        pendingApprovals: [
            {
                approvalId: "approval-1",
                details: { command: "echo hi", decisions: [] },
            },
        ],
    });
    await store_1.useStore.getState().selectProject("project-2");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.selectedProjectId, "project-2");
    strict_1.default.equal(state.runId, undefined);
    strict_1.default.equal(state.mainRunId, undefined);
    strict_1.default.equal(state.activeStepId, undefined);
    strict_1.default.equal(state.status, "idle");
    strict_1.default.deepEqual(state.timeline, []);
    strict_1.default.deepEqual(state.artifacts, []);
    strict_1.default.deepEqual(state.pendingApprovals, []);
});
(0, node_test_1.default)("setChatStartMode clears flowRef and builtin orchestration options on any mode change", () => {
    seedStore(makeClient(), []);
    store_1.useStore.setState({ flowRef: "flowpilot-core-flow-pack/review-loop", builtinOrchestrationOptions: [
            { flowRef: "flowpilot-core-flow-pack/review-loop", label: "Review Loop", description: "" },
        ] });
    store_1.useStore.getState().setChatStartMode("normal");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.chatStartMode, "normal");
    strict_1.default.equal(state.flowRef, undefined);
    strict_1.default.deepEqual(state.builtinOrchestrationOptions, []);
});
(0, node_test_1.default)("setChatStartMode loads builtin orchestration options when entering bugfix", async () => {
    seedStore(makeClient({
        listBuiltinOrchestrationOptions: async (subMode) => {
            strict_1.default.equal(subMode, "bug");
            return [{ flowRef: "flowpilot-core-flow-pack/review-loop", label: "Review Loop", description: "review until clean" }];
        },
    }), []);
    store_1.useStore.getState().setChatStartMode("bugfix");
    // loadBuiltinOrchestrationOptions is fired-and-forgotten from setChatStartMode; await a tick.
    await Promise.resolve();
    await Promise.resolve();
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.builtinOrchestrationOptions.length, 1);
    strict_1.default.equal(state.builtinOrchestrationOptions[0]?.flowRef, "flowpilot-core-flow-pack/review-loop");
});
(0, node_test_1.default)("openHistoryRun treats active-account-not-signed-in as unavailable instead of replacing the current run", async () => {
    seedStore(makeClient({
        resumeRun: async () => {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "account_not_signed_in", "can't open — the active account isn't signed in");
        },
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runId, "current-run");
    strict_1.default.equal(state.runHistory[0]?.unavailableReason, "can't open — the active account isn't signed in");
});
(0, node_test_1.default)("openHistoryRun clears unavailableReason after a successful open", async () => {
    const handle = {
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "completed",
        stepId: "chat-run-1",
    };
    seedStore(makeClient({
        resumeRun: async () => handle,
        streamRun: () => emptyStream(),
        listSkills: async () => [],
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
            unavailableReason: "old reason",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runId, "run-1");
    strict_1.default.equal(state.activeStepId, "chat-run-1");
    strict_1.default.equal(state.selectedProvider, "codex");
    strict_1.default.equal(state.runHistory[0]?.unavailableReason, undefined);
});
(0, node_test_1.default)("openHistoryRun returns after attaching an open-ended history stream", async () => {
    const streamStarted = deferred();
    const handle = {
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "running",
        stepId: "chat-run-1",
    };
    async function* hangingHistoryStream() {
        streamStarted.resolve();
        yield { ...BASE_EVENT, type: "turn_started", providerTurnId: "turn-1", prompt: "old prompt" };
        await new Promise(() => { });
    }
    seedStore(makeClient({
        resumeRun: async () => handle,
        streamRun: () => hangingHistoryStream(),
        listSkills: async () => [],
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "running",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await Promise.race([
        store_1.useStore.getState().openHistoryRun("run-1"),
        new Promise((_, reject) => setTimeout(() => reject(new Error("openHistoryRun did not return")), 100)),
    ]);
    await Promise.race([
        streamStarted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
    ]);
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runId, "run-1");
    strict_1.default.equal(state.status, "running");
});
(0, node_test_1.default)("openHistoryRun does not leave Thinking visible for a completed history replay without a terminal event", async () => {
    const streamStarted = deferred();
    const handle = {
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "completed",
        stepId: "chat-run-1",
    };
    async function* incompleteCompletedHistoryStream() {
        streamStarted.resolve();
        yield { ...BASE_EVENT, type: "turn_started", providerTurnId: "turn-1", prompt: "old prompt" };
        await new Promise(() => { });
    }
    seedStore(makeClient({
        resumeRun: async () => handle,
        streamRun: () => incompleteCompletedHistoryStream(),
        listSkills: async () => [],
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    await Promise.race([
        streamStarted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
    ]);
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.status, "completed");
    strict_1.default.equal(state.timeline.some((item) => item.kind === "thinking"), false);
});
(0, node_test_1.default)("openHistoryRun keeps an approval gate open when the replay ends on permission_required", async () => {
    const streamStarted = deferred();
    const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }] };
    const handle = {
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "cancelled",
        stepId: "chat-run-1",
    };
    async function* approvalHistoryStream() {
        streamStarted.resolve();
        yield {
            ...BASE_EVENT,
            type: "permission_required",
            approvalId: "appr-1",
            provider: "codex",
            details: approvalDetails,
        };
    }
    seedStore(makeClient({
        resumeRun: async () => handle,
        streamRun: () => approvalHistoryStream(),
        listSkills: async () => [],
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "cancelled",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    await Promise.race([
        streamStarted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
    ]);
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.status, "waiting_approval");
    strict_1.default.deepEqual(state.pendingApprovals, [{ approvalId: "appr-1", details: approvalDetails }]);
    const card = state.timeline.find((item) => item.kind === "approval");
    strict_1.default.equal(card?.decision, undefined, "approval card should remain actionable");
});
(0, node_test_1.default)("sendPrompt aborts an open-ended history replay stream before sending", async () => {
    const historyStreamStarted = deferred();
    const historyStreamAborted = deferred();
    const handle = {
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "running",
        stepId: "chat-run-1",
    };
    async function* hangingHistoryStream(_runId, _afterSeq = 0, signal) {
        historyStreamStarted.resolve();
        yield { ...BASE_EVENT, type: "turn_started", providerTurnId: "turn-1", prompt: "old prompt" };
        await new Promise((resolve) => {
            if (signal?.aborted) {
                historyStreamAborted.resolve();
                resolve();
                return;
            }
            signal?.addEventListener("abort", () => {
                historyStreamAborted.resolve();
                resolve();
            }, { once: true });
        });
    }
    async function* completedSendTurn(input) {
        yield { ...BASE_EVENT, seq: 2, type: "turn_started", providerTurnId: "turn-2", prompt: input.prompt };
        yield { ...BASE_EVENT, seq: 3, type: "turn_completed", finalMessage: "done" };
    }
    seedStore(makeClient({
        resumeRun: async () => handle,
        streamRun: hangingHistoryStream,
        sendTurn: completedSendTurn,
        listSkills: async () => [],
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "running",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    await Promise.race([
        historyStreamStarted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
    ]);
    await store_1.useStore.getState().sendPrompt("new prompt");
    await Promise.race([
        historyStreamAborted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("history stream was not aborted")), 100)),
    ]);
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runId, "run-1");
    strict_1.default.equal(state.status, "completed");
});
(0, node_test_1.default)("sendPrompt blocks direct prompting while a child transcript is focused", async () => {
    let startCalls = 0;
    let sendCalls = 0;
    seedStore(makeClient({
        startRun: async () => {
            startCalls += 1;
            return { runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running" };
        },
        sendTurn: async function* () {
            sendCalls += 1;
        },
    }), []);
    store_1.useStore.setState({
        runId: "child-run",
        mainRunId: "current-run",
        activeAgentRunId: "child-run",
        activeStepId: "chat-current-run",
        status: "completed",
    });
    await store_1.useStore.getState().sendPrompt("do not send");
    strict_1.default.equal(startCalls, 0);
    strict_1.default.equal(sendCalls, 0);
    const last = store_1.useStore.getState().timeline.at(-1);
    strict_1.default.equal(last?.kind, "system");
    strict_1.default.equal(last?.kind === "system" ? last.text : "", "Child transcript is read-only. Return to the main chat to send prompts.");
});
(0, node_test_1.default)("sendPrompt sends declared task intent only on the first chat turn", async () => {
    const seen = [];
    async function* completedSendTurn(input) {
        seen.push(input);
        yield { ...BASE_EVENT, seq: 2 + seen.length * 2, type: "turn_started", providerTurnId: `turn-${seen.length}`, prompt: input.prompt };
        yield { ...BASE_EVENT, seq: 3 + seen.length * 2, type: "turn_completed", finalMessage: "done" };
    }
    seedStore(makeClient({
        startRun: async () => ({ runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running", stepId: "chat-new-run" }),
        sendTurn: completedSendTurn,
        listRunHistory: async () => [],
    }), []);
    store_1.useStore.setState({
        runId: undefined,
        mainRunId: undefined,
        activeStepId: undefined,
        status: "idle",
        timeline: [],
        selectedProvider: "codex",
        chatStartMode: "task",
        chatSourceDocId: "Task-114",
    });
    await store_1.useStore.getState().sendPrompt("first");
    await store_1.useStore.getState().sendPrompt("second");
    strict_1.default.equal(seen.length, 2);
    strict_1.default.equal(seen[0]?.changeType, "task");
    strict_1.default.equal(seen[0]?.sourceDocId, "Task-114");
    strict_1.default.equal(seen[1]?.changeType, undefined);
    strict_1.default.equal(seen[1]?.sourceDocId, undefined);
});
(0, node_test_1.default)("openHistoryRun selects the resumed provider default model", async () => {
    const handle = {
        runId: "run-claude",
        providerSessionId: "session-claude",
        providerKey: "claude",
        status: "completed",
        stepId: "chat-run-claude",
    };
    seedStore(makeClient({
        resumeRun: async () => handle,
        streamRun: () => emptyStream(),
        listSkills: async () => [],
    }), [
        {
            runId: "run-claude",
            projectId: "project-1",
            providerKey: "claude",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    store_1.useStore.setState({
        selectedProvider: "codex",
        selectedModel: "o4-mini",
        supportedModels: [
            {
                id: "model-codex",
                providerKey: "codex",
                modelId: "o4-mini",
                displayName: "o4-mini",
                isEnabled: true,
                sortOrder: 0,
                source: "test",
                detectionMethod: null,
                detectedCliVersion: null,
                lastDetectedAt: null,
                createdAt: "2026-06-19T00:00:00Z",
                updatedAt: "2026-06-19T00:00:00Z",
            },
            {
                id: "model-claude",
                providerKey: "claude",
                modelId: "claude-sonnet-4",
                displayName: "Claude Sonnet 4",
                isEnabled: true,
                sortOrder: 0,
                source: "test",
                detectionMethod: null,
                detectedCliVersion: null,
                lastDetectedAt: null,
                createdAt: "2026-06-19T00:00:00Z",
                updatedAt: "2026-06-19T00:00:00Z",
            },
        ],
    });
    await store_1.useStore.getState().openHistoryRun("run-claude");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.selectedProvider, "claude");
    strict_1.default.equal(state.selectedModel, "claude-sonnet-4");
});
(0, node_test_1.default)("selectProvider defaults Codex to a full model before mini", () => {
    seedStore(makeClient(), []);
    store_1.useStore.setState({
        selectedProvider: "claude",
        selectedModel: undefined,
        supportedModels: [
            {
                id: "model-mini",
                providerKey: "codex",
                modelId: "gpt-5.4-mini",
                displayName: "gpt-5.4-mini",
                isEnabled: true,
                sortOrder: 0,
                source: "test",
                detectionMethod: null,
                detectedCliVersion: null,
                lastDetectedAt: null,
                createdAt: "2026-06-19T00:00:00Z",
                updatedAt: "2026-06-19T00:00:00Z",
            },
            {
                id: "model-full",
                providerKey: "codex",
                modelId: "gpt-5.5",
                displayName: "gpt-5.5",
                isEnabled: true,
                sortOrder: 1,
                source: "test",
                detectionMethod: null,
                detectedCliVersion: null,
                lastDetectedAt: null,
                createdAt: "2026-06-19T00:00:00Z",
                updatedAt: "2026-06-19T00:00:00Z",
            },
        ],
    });
    store_1.useStore.getState().selectProvider("codex");
    strict_1.default.equal(store_1.useStore.getState().selectedModel, "gpt-5.5");
});
(0, node_test_1.default)("refreshAgentGraph stores orchestration snapshot from the client", async () => {
    seedStore(makeClient(), []);
    await store_1.useStore.getState().refreshAgentGraph();
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.roundCap, 3);
});
(0, node_test_1.default)("refreshAgentGraph keeps agent bus messages aligned with the refreshed snapshot", async () => {
    const busMessage = {
        id: "bus-graph",
        parentRunId: "current-run",
        kind: "handoff",
        message: "ready-for-review",
        queued: true,
        occurredAt: "2026-01-01T00:00:00Z",
    };
    seedStore(makeClient({
        refreshAgentGraph: async () => ({
            parentRunId: "current-run",
            runs: [],
            edges: [],
            busMessages: [busMessage],
            loopState: { status: "paused", round: 2, roundCap: 3, gateReason: "waiting on child" },
        }),
    }), []);
    await store_1.useStore.getState().refreshAgentGraph();
    strict_1.default.deepEqual(store_1.useStore.getState().agentBusMessages, [busMessage]);
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.gateReason, "waiting on child");
});
(0, node_test_1.default)("sendPrompt applies live agent graph and bus SSE updates", async () => {
    const stream = async function* () {
        yield {
            ...BASE_EVENT,
            type: "agent_graph_updated",
            agentGraphSnapshot: {
                parentRunId: "current-run",
                runs: [],
                edges: [],
                busMessages: [],
                loopState: { status: "running", round: 1, roundCap: 3 },
            },
        };
        yield {
            ...BASE_EVENT,
            type: "agent_bus_message",
            agentBusMessage: {
                id: "bus-1",
                parentRunId: "current-run",
                kind: "handoff",
                message: "ready-for-review",
                queued: false,
                occurredAt: "2026-01-01T00:00:00Z",
            },
        };
        yield { ...BASE_EVENT, type: "turn_completed", finalMessage: "ok" };
    };
    seedStore(makeClient({
        sendTurn: () => stream(),
        startRun: async () => ({ runId: "current-run", providerSessionId: "session-1", providerKey: "codex", status: "running", stepId: "chat-current-run" }),
    }), []);
    await store_1.useStore.getState().sendPrompt("hello");
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.round, 1);
    strict_1.default.equal(store_1.useStore.getState().agentBusMessages.at(-1)?.message, "ready-for-review");
});
(0, node_test_1.default)("refreshAgentRuns loads child summaries for the active main run", async () => {
    const agentRuns = [
        {
            runId: "agent-1",
            agentName: "architect",
            role: "architecture",
            status: "running",
            parentRunId: "current-run",
            createdAt: "2026-06-17T10:01:00Z",
            agentStatus: "running",
        },
    ];
    seedStore(makeClient({
        listAgentRuns: async () => agentRuns,
    }), []);
    await store_1.useStore.getState().refreshAgentRuns();
    strict_1.default.deepEqual(store_1.useStore.getState().agentRuns, agentRuns);
});
(0, node_test_1.default)("focusAgentRun caches the main timeline and backToMainRun restores it", async () => {
    seedStore(makeClient({
        focusAgentRun: () => childFocusStream(),
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
        status: "running",
        artifacts: [],
    });
    await store_1.useStore.getState().focusAgentRun("child-run");
    await new Promise((resolve) => setTimeout(resolve, 0));
    strict_1.default.equal(store_1.useStore.getState().runId, "child-run");
    strict_1.default.equal(store_1.useStore.getState().activeAgentRunId, "child-run");
    strict_1.default.ok(store_1.useStore.getState().timeline.some((item) => item.kind === "assistant"));
    store_1.useStore.getState().backToMainRun();
    strict_1.default.equal(store_1.useStore.getState().runId, "current-run");
    strict_1.default.equal(store_1.useStore.getState().activeAgentRunId, undefined);
    strict_1.default.deepEqual(store_1.useStore.getState().timeline, [{ kind: "prompt", id: "prompt-main", text: "main timeline" }]);
});
(0, node_test_1.default)("focusAgentRun resumes the child run before opening its event stream", async () => {
    let resumed = false;
    seedStore(makeClient({
        resumeRun: async (runId) => {
            strict_1.default.equal(runId, "child-run");
            resumed = true;
            return { runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" };
        },
        focusAgentRun: async function* () {
            strict_1.default.equal(resumed, true);
            yield* childFocusStream();
        },
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
        status: "running",
        artifacts: [],
    });
    await store_1.useStore.getState().focusAgentRun("child-run");
    await new Promise((resolve) => setTimeout(resolve, 0));
    strict_1.default.equal(resumed, true);
    strict_1.default.equal(store_1.useStore.getState().runId, "child-run");
    strict_1.default.ok(store_1.useStore.getState().timeline.some((item) => item.kind === "assistant"));
});
(0, node_test_1.default)("focusAgentRun replays from start when no child snapshot is cached", async () => {
    seedStore(makeClient({
        focusAgentRun: () => cursorChildStream(),
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
        status: "running",
        artifacts: [],
        _runReplaySeq: { "child-run": 2 },
    });
    await store_1.useStore.getState().focusAgentRun("child-run");
    await new Promise((resolve) => setTimeout(resolve, 0));
    const assistantTexts = store_1.useStore.getState().timeline
        .filter((item) => item.kind === "assistant")
        .map((item) => (item.kind === "assistant" ? item.text : ""));
    strict_1.default.deepEqual(assistantTexts, ["old child chunknew child chunk"]);
});
(0, node_test_1.default)("focusAgentRun resumes from the last replay cursor when a child snapshot is cached", async () => {
    seedStore(makeClient({
        focusAgentRun: () => cursorChildStream(),
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
        status: "running",
        artifacts: [],
        _runReplaySeq: { "child-run": 2 },
        _runSnapshots: {
            "child-run": {
                timeline: [],
                artifacts: [],
                status: "running",
                pendingApprovals: [],
                pendingQuestions: [],
                recoverable: false,
                lastEventSeq: 2,
            },
        },
    });
    await store_1.useStore.getState().focusAgentRun("child-run");
    await new Promise((resolve) => setTimeout(resolve, 0));
    const assistantTexts = store_1.useStore.getState().timeline
        .filter((item) => item.kind === "assistant")
        .map((item) => (item.kind === "assistant" ? item.text : ""));
    strict_1.default.deepEqual(assistantTexts, ["new child chunk"]);
});
(0, node_test_1.default)("focusAgentRun returns before a child stream finishes", async () => {
    const gate = deferred();
    seedStore(makeClient({
        focusAgentRun: () => pendingChildStream(gate.promise),
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
        status: "running",
        artifacts: [],
    });
    const focusPromise = store_1.useStore.getState().focusAgentRun("child-run");
    await Promise.race([focusPromise, Promise.resolve()]);
    strict_1.default.equal(store_1.useStore.getState().runId, "child-run");
    strict_1.default.equal(store_1.useStore.getState().activeAgentRunId, "child-run");
    gate.resolve();
    await focusPromise;
});
(0, node_test_1.default)("focusAgentRun ignores replay events that belong to the main run", async () => {
    seedStore(makeClient({
        resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
        focusAgentRun: async function* () {
            yield { ...BASE_EVENT, workflowRunId: "current-run", seq: 1, type: "message_delta", text: "wrong main replay" };
            yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 2, type: "message_delta", text: "child replay" };
        },
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
        status: "completed",
        artifacts: [],
    });
    await store_1.useStore.getState().focusAgentRun("child-run");
    await new Promise((resolve) => setTimeout(resolve, 0));
    const assistantTexts = store_1.useStore.getState().timeline
        .filter((item) => item.kind === "assistant")
        .map((item) => (item.kind === "assistant" ? item.text : ""));
    strict_1.default.deepEqual(assistantTexts, ["child replay"]);
    strict_1.default.equal(store_1.useStore.getState().timeline.some((item) => item.kind === "thinking"), false);
});
(0, node_test_1.default)("focusAgentRun clears stale parent assistant accumulator before child replay", async () => {
    seedStore(makeClient({
        resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
        focusAgentRun: async function* () {
            yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 1, type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
            yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 2, type: "message_delta", text: "CHILD_AGENT_DONE" };
            yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 3, type: "turn_completed", finalMessage: "CHILD_AGENT_DONE" };
        },
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "assistant", id: "main-assistant", text: "parent response", finalized: false }],
        status: "running",
        artifacts: [],
        _streamingAssistantId: "main-assistant",
    });
    await store_1.useStore.getState().focusAgentRun("child-run");
    await new Promise((resolve) => setTimeout(resolve, 0));
    const state = store_1.useStore.getState();
    strict_1.default.equal(state._streamingAssistantId, undefined);
    strict_1.default.equal(state.timeline.some((item) => item.kind === "assistant" && item.text.includes("parent response")), false);
    strict_1.default.deepEqual(state.timeline.filter((item) => item.kind === "assistant").map((item) => item.text), ["CHILD_AGENT_DONE"]);
});
(0, node_test_1.default)("applyTimelineEvent starts a new assistant bubble for a new turn", () => {
    const base = {
        status: "running",
        recoverable: false,
        pendingApprovals: [],
        pendingQuestions: [],
        _streamingAssistantId: "assistant-main",
        timeline: [{ kind: "assistant", id: "assistant-main", text: "parent response", finalized: false }],
    };
    const started = (0, timelineReducer_1.applyTimelineEvent)(base, {
        ...BASE_EVENT,
        workflowRunId: "current-run",
        providerTurnId: "turn-2",
        type: "turn_started",
        prompt: "second prompt",
    });
    const next = (0, timelineReducer_1.applyTimelineEvent)({ ...base, ...started }, {
        ...BASE_EVENT,
        workflowRunId: "current-run",
        providerTurnId: "turn-2",
        id: "assistant-child",
        type: "message_delta",
        text: "new response",
    });
    const assistantTexts = next.timeline
        ?.filter((item) => item.kind === "assistant")
        .map((item) => (item.kind === "assistant" ? item.text : ""));
    strict_1.default.deepEqual(assistantTexts, ["parent response", "new response"]);
});
(0, node_test_1.default)("backToMainRun aborts its replay stream before focusing a child again", async () => {
    const mainStreamStarted = deferred();
    const mainStreamAborted = deferred();
    async function* mainReplayStream(_runId, _afterSeq = 0, signal) {
        mainStreamStarted.resolve();
        await new Promise((resolve) => {
            if (signal?.aborted) {
                mainStreamAborted.resolve();
                resolve();
                return;
            }
            signal?.addEventListener("abort", () => {
                mainStreamAborted.resolve();
                resolve();
            }, { once: true });
        });
    }
    seedStore(makeClient({
        streamRun: mainReplayStream,
        focusAgentRun: () => emptyStream(),
        resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
    }), []);
    store_1.useStore.setState({
        runId: "child-run",
        mainRunId: "current-run",
        activeAgentRunId: "child-run",
        timeline: [{ kind: "assistant", id: "child-message", text: "child", finalized: true }],
        status: "completed",
        artifacts: [],
        _runSnapshots: {
            "current-run": {
                timeline: [{ kind: "assistant", id: "main-message", text: "main", finalized: true }],
                artifacts: [],
                status: "completed",
                pendingApprovals: [],
                pendingQuestions: [],
                recoverable: false,
            },
        },
    });
    store_1.useStore.getState().backToMainRun();
    await Promise.race([
        mainStreamStarted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("main replay stream did not start")), 100)),
    ]);
    await store_1.useStore.getState().focusAgentRun("child-run");
    await Promise.race([
        mainStreamAborted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("main replay stream was not aborted")), 100)),
    ]);
});
(0, node_test_1.default)("injectAgentFeedback queues busy mention feedback on the board state", async () => {
    seedStore(makeClient({
        injectAgentFeedback: async (_parentRunId, _toRunId, message) => ({
            parentRunId: "current-run",
            runs: [],
            edges: [],
            busMessages: [{ id: "bus-1", parentRunId: "current-run", kind: "user-feedback", message, queued: true, occurredAt: "2026-01-01T00:00:00Z" }],
            loopState: { status: "paused", round: 0, roundCap: 3, gateReason: "queued user-feedback" },
        }),
    }), []);
    await store_1.useStore.getState().injectAgentFeedback("agent-2", "please revise");
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.gateReason, "queued user-feedback");
    strict_1.default.equal(store_1.useStore.getState().agentBusMessages.at(-1)?.kind, "user-feedback");
});
(0, node_test_1.default)("orchestration board helpers surface empty and queued states", () => {
    strict_1.default.equal((0, OrchestrationBoard_1.getOrchestrationBoardEmptyCopy)(undefined, []), "No graph snapshot loaded. Refresh to inspect the loop.");
    strict_1.default.equal((0, OrchestrationBoard_1.getOrchestrationBoardEmptyCopy)({
        parentRunId: "current-run",
        runs: [],
        edges: [],
        busMessages: [],
        loopState: { status: "running", round: 0, roundCap: 3 },
    }, []), "No child agents yet.");
    strict_1.default.equal((0, OrchestrationBoard_1.getBusMessageLabel)({
        id: "bus-1",
        parentRunId: "current-run",
        kind: "user-feedback",
        message: "please revise",
        queued: true,
        occurredAt: "2026-01-01T00:00:00Z",
    }), "user-feedback: please revise (queued)");
});
(0, node_test_1.default)("openAgentSpawnGuide overwrites the preselected agent and clearAgentSpawnGuide resets it", async () => {
    seedStore(makeClient(), []);
    store_1.useStore.getState().openAgentSpawnGuide("architect");
    strict_1.default.equal(store_1.useStore.getState().agentSpawnGuideOpen, true);
    strict_1.default.equal(store_1.useStore.getState().agentSpawnGuideAgentName, "architect");
    store_1.useStore.getState().openAgentSpawnGuide("reviewer");
    strict_1.default.equal(store_1.useStore.getState().agentSpawnGuideOpen, true);
    strict_1.default.equal(store_1.useStore.getState().agentSpawnGuideAgentName, "reviewer");
    store_1.useStore.getState().clearAgentSpawnGuide();
    strict_1.default.equal(store_1.useStore.getState().agentSpawnGuideOpen, false);
    strict_1.default.equal(store_1.useStore.getState().agentSpawnGuideAgentName, undefined);
});
// BUG-109: orchestration stream advances _runReplaySeq[mainRunId] via agent_graph_updated
// events while viewing a child. backToMainRun must use restore.lastEventSeq (not the
// inflated _runReplaySeq) so it doesn't skip real timeline events in the gap.
(0, node_test_1.default)("backToMainRun replays from snapshot lastEventSeq, not from orchestration-inflated _runReplaySeq", async () => {
    const mainStreamSeqs = [];
    async function* mainStream(_runId, afterSeq = 0) {
        mainStreamSeqs.push(afterSeq);
        // No events — just record what afterSeq was used
    }
    seedStore(makeClient({
        streamRun: mainStream,
        focusAgentRun: () => emptyStream(),
        resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
    }), []);
    // Real flow: the user is VIEWING the child (runId === child-run) when they click
    // "back to main agent". backToMainRun snapshots the child and restores the main
    // snapshot captured earlier during focusAgentRun.
    store_1.useStore.setState({
        runId: "child-run",
        mainRunId: "current-run",
        activeAgentRunId: "child-run",
        timeline: [{ kind: "assistant", id: "child-a", text: "child output", finalized: true }],
        status: "completed",
        artifacts: [],
        _runSnapshots: {
            "current-run": {
                timeline: [{ kind: "prompt", id: "main-prompt", text: "main prompt" }],
                artifacts: [],
                status: "running",
                pendingApprovals: [],
                pendingQuestions: [],
                recoverable: false,
                lastEventSeq: 10,
            },
        },
        // _runReplaySeq[mainRunId] inflated to 50 by orchestration stream processing
        // agent_graph_updated events (seq 11..50) while the user was viewing the child.
        _runReplaySeq: { "current-run": 50, "child-run": 3 },
    });
    store_1.useStore.getState().backToMainRun();
    await new Promise((resolve) => setTimeout(resolve, 0));
    // The timeline replay (consumeAgentStream, the FIRST streamRun call) must start from
    // the snapshot's lastEventSeq (10), NOT the orchestration-inflated _runReplaySeq (50).
    // Starting at 50 would skip real timeline events with seqs 11..50 (e.g. message_completed).
    // The orchestration stream legitimately resumes from 50 (the second call). (BUG-109)
    strict_1.default.equal(mainStreamSeqs[0], 10, "timeline replay must use snapshot lastEventSeq");
    strict_1.default.equal(mainStreamSeqs.includes(50), true, "orchestration stream resumes from its cursor");
});
// BUG-109: consumeAgentStream must not re-process agent_graph_updated / agent_bus_message
// events when replaying the gap between restore.lastEventSeq and the orchestration-advanced
// _runReplaySeq. Doing so would append duplicate entries to agentBusMessages.
(0, node_test_1.default)("backToMainRun replaying the gap does not duplicate agentBusMessages", async () => {
    const busMessage = {
        id: "bus-1",
        parentRunId: "current-run",
        kind: "handoff",
        message: "work done",
        queued: false,
        occurredAt: "2026-01-01T00:00:00Z",
    };
    async function* mainStream(_runId, _afterSeq = 0) {
        // This event was already processed by the orchestration stream while in child view.
        // consumeAgentStream must skip it to avoid duplicating the bus message.
        yield {
            ...BASE_EVENT,
            seq: 15,
            workflowRunId: "current-run",
            type: "agent_bus_message",
            agentBusMessage: busMessage,
        };
    }
    seedStore(makeClient({
        streamRun: mainStream,
        focusAgentRun: () => emptyStream(),
        resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
    }), []);
    store_1.useStore.setState({
        runId: "current-run",
        mainRunId: "current-run",
        activeAgentRunId: undefined,
        timeline: [{ kind: "prompt", id: "main-prompt", text: "main prompt" }],
        status: "running",
        artifacts: [],
        // agentBusMessages already has the bus message from the orchestration stream
        agentBusMessages: [busMessage],
        _runSnapshots: {
            "current-run": {
                timeline: [{ kind: "prompt", id: "main-prompt", text: "main prompt" }],
                artifacts: [],
                status: "running",
                pendingApprovals: [],
                pendingQuestions: [],
                recoverable: false,
                lastEventSeq: 10,
            },
        },
        _runReplaySeq: { "current-run": 50 },
    });
    store_1.useStore.getState().backToMainRun();
    await new Promise((resolve) => setTimeout(resolve, 0));
    // agentBusMessages must stay at 1 entry — consumeAgentStream skips orchestration events.
    strict_1.default.equal(store_1.useStore.getState().agentBusMessages.length, 1);
});
(0, node_test_1.default)("parseMentionRouting resolves missing, busy, and idle child targets", () => {
    const runs = [
        { agentName: "architect", runId: "agent-1", status: "completed" },
        { agentName: "builder", runId: "agent-2", status: "running" },
    ];
    strict_1.default.deepEqual((0, ChatInput_1.parseMentionRouting)("@architect do work", runs), {
        kind: "focus",
        agentName: "architect",
        runId: "agent-1",
        prompt: "do work",
    });
    strict_1.default.deepEqual((0, ChatInput_1.parseMentionRouting)("@builder do work", runs), {
        kind: "busy",
        agentName: "builder",
        runId: "agent-2",
        prompt: "do work",
    });
    strict_1.default.deepEqual((0, ChatInput_1.parseMentionRouting)("@reviewer do work", runs), {
        kind: "missing",
        agentName: "reviewer",
    });
});
(0, node_test_1.default)("shouldShowAgentTimelineHeader stays hidden for single-agent runs", async () => {
    strict_1.default.equal((0, Timeline_1.shouldShowAgentTimelineHeader)(undefined, undefined, 0), false);
    strict_1.default.equal((0, Timeline_1.shouldShowAgentTimelineHeader)("current-run", "current-run", 0), false);
    strict_1.default.equal((0, Timeline_1.shouldShowAgentTimelineHeader)(undefined, "current-run", 0), false);
    strict_1.default.equal((0, Timeline_1.shouldShowAgentTimelineHeader)("child-run", "current-run", 1), true);
});
(0, node_test_1.default)("syncHistoryRun updates local run sync metadata", async () => {
    seedStore(makeClient(), [
        {
            runId: "run-sync",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
            runKind: "chat",
        },
    ]);
    await store_1.useStore.getState().syncHistoryRun("run-sync");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runHistory[0]?.syncStatus, "synced");
    strict_1.default.equal(state.runHistory[0]?.sourceMachineId, "mch_sync");
    strict_1.default.equal(state.runHistory[0]?.sourceRunId, "run-sync");
});
(0, node_test_1.default)("syncHistoryRun marks syncing state while the request is in flight", async () => {
    const gate = deferred();
    seedStore(makeClient({
        syncChatRun: async () => gate.promise,
    }), [
        {
            runId: "run-sync",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
            runKind: "chat",
        },
    ]);
    const pending = store_1.useStore.getState().syncHistoryRun("run-sync");
    strict_1.default.equal(store_1.useStore.getState().runHistory[0]?.syncStatus, "syncing");
    gate.resolve({
        runId: "run-sync",
        sourceMachineId: "mch_sync",
        sourceRunId: "run-sync",
        syncStatus: "synced",
        syncedAt: "2026-06-17T10:10:00Z",
        remotePath: "chat-sessions/runs/mch_sync/run-sync/manifest.json",
    });
    await pending;
});
(0, node_test_1.default)("syncHistoryRun typed error preserves current timeline", async () => {
    seedStore(makeClient({
        syncChatRun: async () => {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "session_unavailable", "session data not found on this machine");
        },
    }), [
        {
            runId: "run-sync",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
            runKind: "chat",
        },
    ]);
    await store_1.useStore.getState().syncHistoryRun("run-sync");
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runId, "current-run");
    strict_1.default.deepEqual(state.timeline, [{ kind: "prompt", id: "prompt-1", text: "keep current timeline" }]);
    strict_1.default.equal(state.runHistory[0]?.syncStatus, "failed");
    strict_1.default.equal(state.runHistory[0]?.unavailableReason, "session data not found on this machine");
});
(0, node_test_1.default)("loadRemoteChatSessions stores remote summaries", async () => {
    const summary = {
        runId: "run-remote",
        projectId: "project-1",
        providerKey: "codex",
        sourceMachineId: "mch_remote",
        sourceRunId: "run-remote",
    };
    seedStore(makeClient({
        listRemoteChatSessions: async () => [summary],
    }), []);
    await store_1.useStore.getState().loadRemoteChatSessions();
    const state = store_1.useStore.getState();
    strict_1.default.deepEqual(state.remoteChatSessions, [summary]);
    strict_1.default.equal(state.remoteHistoryLoading, false);
    strict_1.default.equal(state.remoteHistoryLoadError, undefined);
});
(0, node_test_1.default)("restoreRemoteChatSession retries cwd_remap_required with the selected project path", async () => {
    const calls = [];
    const summary = {
        runId: "run-remote",
        projectId: "project-1",
        providerKey: "codex",
        sourceMachineId: "mch_remote",
        sourceRunId: "run-remote",
    };
    seedStore(makeClient({
        listRunHistory: async () => [],
        listRemoteChatSessions: async () => [summary],
        restoreChatRun: async (input) => {
            calls.push(input.cwd ?? "");
            if (!input.cwd) {
                throw new HttpWsRunnerClient_1.RunnerApiError(409, "cwd_remap_required", "select a local project path before restoring this chat");
            }
            return {
                runId: input.sourceRunId,
                sourceMachineId: input.sourceMachineId,
                sourceRunId: input.sourceRunId,
                providerKey: "codex",
                restoreStatus: "restored",
            };
        },
    }), []);
    store_1.useStore.setState({
        projects: [{ id: "project-1", name: "Project 1", path: "/tmp/project-1" }],
        remoteChatSessions: [summary],
    });
    await store_1.useStore.getState().restoreRemoteChatSession(summary);
    strict_1.default.deepEqual(calls, ["", "/tmp/project-1"]);
});
(0, node_test_1.default)("restoreRemoteChatSession adds restored run to history", async () => {
    const summary = {
        runId: "run-remote",
        projectId: "project-1",
        providerKey: "codex",
        sourceMachineId: "mch_remote",
        sourceRunId: "run-remote",
    };
    const restoredHistory = {
        runId: "run-remote",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
        sourceMachineId: "mch_remote",
        sourceRunId: "run-remote",
        syncStatus: "restored",
    };
    seedStore(makeClient({
        restoreChatRun: async () => ({
            runId: "run-remote",
            sourceMachineId: "mch_remote",
            sourceRunId: "run-remote",
            providerKey: "codex",
            restoreStatus: "restored",
        }),
        listRunHistory: async () => [restoredHistory],
        listRemoteChatSessions: async () => [summary],
    }), []);
    store_1.useStore.setState({ remoteChatSessions: [summary] });
    await store_1.useStore.getState().restoreRemoteChatSession(summary);
    const state = store_1.useStore.getState();
    strict_1.default.equal(state.runHistory[0]?.runId, "run-remote");
    strict_1.default.equal(state.runHistory[0]?.providerKey, "codex");
});
(0, node_test_1.default)("restoreRemoteChatSession marks remote entries unavailable on typed restore errors", async () => {
    const summary = {
        runId: "run-remote",
        projectId: "project-1",
        providerKey: "codex",
        sourceMachineId: "mch_remote",
        sourceRunId: "run-remote",
    };
    seedStore(makeClient({
        restoreChatRun: async () => {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "sync_integrity_failed", "remote provider session file failed integrity validation");
        },
    }), []);
    store_1.useStore.setState({ remoteChatSessions: [summary] });
    await store_1.useStore.getState().restoreRemoteChatSession(summary);
    strict_1.default.equal(store_1.useStore.getState().remoteChatSessions[0]?.unavailableReason, "remote provider session file failed integrity validation");
});
// ── Account-switch tests ───────────────────────────────────────────────────
(0, node_test_1.default)("usage-limit turn_failed with valid Codex candidate sets pendingAccountSwitch", async () => {
    const acc1 = makeAccount("acc-1", "codex", { isActive: true, remaining5hPercent: 0, remaining7dPercent: 90, slotIndex: 0 });
    const acc2 = makeAccount("acc-2", "codex", { isActive: false, remaining5hPercent: 50, remaining7dPercent: 60, slotIndex: 1 });
    seedStore(makeClient({
        startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
        sendTurn: () => turnFailedStream("usage limit reached", false),
        listRunHistory: async () => [],
    }), []);
    store_1.useStore.setState({ providerAccounts: [acc1, acc2], selectedProvider: "codex", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });
    await store_1.useStore.getState().sendPrompt("hello");
    const state = store_1.useStore.getState();
    strict_1.default.ok(state.pendingAccountSwitch !== undefined, "pendingAccountSwitch should be set");
    strict_1.default.equal(state.pendingAccountSwitch?.failedAccountId, "acc-1");
    strict_1.default.equal(state.pendingAccountSwitch?.candidateAccount.id, "acc-2");
    strict_1.default.deepEqual(state._accountSwitchTriedIds, ["acc-1"]);
});
(0, node_test_1.default)("candidate ranking: higher remaining5hPercent wins over higher remaining7dPercent", async () => {
    const active = makeAccount("active", "codex", { isActive: true, remaining5hPercent: 0, remaining7dPercent: 0, slotIndex: 0 });
    const accA = makeAccount("acc-A", "codex", { isActive: false, remaining5hPercent: 80, remaining7dPercent: 20, slotIndex: 1 });
    const accB = makeAccount("acc-B", "codex", { isActive: false, remaining5hPercent: 60, remaining7dPercent: 90, slotIndex: 2 });
    seedStore(makeClient({
        startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
        sendTurn: () => turnFailedStream("usage limit reached", false),
        listRunHistory: async () => [],
    }), []);
    store_1.useStore.setState({ providerAccounts: [active, accA, accB], selectedProvider: "codex", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });
    await store_1.useStore.getState().sendPrompt("hello");
    strict_1.default.equal(store_1.useStore.getState().pendingAccountSwitch?.candidateAccount.id, "acc-A");
});
(0, node_test_1.default)("account with remaining5hPercent=0 is excluded from ranked path; no switch offered when no fallback exists", async () => {
    const active = makeAccount("active", "codex", { isActive: true, remaining5hPercent: 0, slotIndex: 0 });
    const zeroH = makeAccount("zero-5h", "codex", { isActive: false, remaining5hPercent: 0, remaining7dPercent: 80, slotIndex: 1 });
    seedStore(makeClient({
        startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
        sendTurn: () => turnFailedStream("usage limit reached", false),
        listRunHistory: async () => [],
    }), []);
    // zeroH has 5h=0 → excluded from ranked path; no null-quota fallback either
    store_1.useStore.setState({ providerAccounts: [active, zeroH], selectedProvider: "codex", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });
    await store_1.useStore.getState().sendPrompt("hello");
    strict_1.default.equal(store_1.useStore.getState().pendingAccountSwitch, undefined, "no valid candidate → no switch offered");
});
(0, node_test_1.default)("cancelAccountSwitch clears pendingAccountSwitch without activating", () => {
    const candidate = makeAccount("acc-2", "codex");
    seedStore(makeClient(), []);
    store_1.useStore.setState({
        pendingAccountSwitch: {
            providerKey: "codex",
            failedAccountId: "acc-1",
            failedAccountLabel: "acc-1@example.com",
            candidateAccount: candidate,
            reason: "usage_limit",
        },
    });
    store_1.useStore.getState().cancelAccountSwitch();
    strict_1.default.equal(store_1.useStore.getState().pendingAccountSwitch, undefined);
    strict_1.default.equal(store_1.useStore.getState().status, "running");
});
(0, node_test_1.default)("confirmAccountSwitch activates account and retries with original lastTurnInput", async () => {
    const sentTurns = [];
    const activatedIds = [];
    const candidate = makeAccount("acc-2", "codex");
    const savedTurnInput = {
        runId: "run-1",
        stepId: "chat-run-1",
        prompt: "original prompt",
    };
    seedStore(makeClient({
        activateProviderAccount: async (id) => { activatedIds.push(id); },
        sendTurn: (input) => { sentTurns.push(input); return emptyStream(); },
        listRunHistory: async () => [],
        listProviderAccounts: async () => [],
    }), []);
    store_1.useStore.setState({
        pendingAccountSwitch: {
            providerKey: "codex",
            failedAccountId: "acc-1",
            failedAccountLabel: "acc-1@example.com",
            candidateAccount: candidate,
            reason: "usage_limit",
        },
        lastTurnInput: savedTurnInput,
        status: "failed",
    });
    await store_1.useStore.getState().confirmAccountSwitch();
    strict_1.default.deepEqual(activatedIds, ["acc-2"], "should activate candidate account");
    strict_1.default.equal(sentTurns.length, 1, "should resend exactly one turn");
    strict_1.default.equal(sentTurns[0]?.prompt, "original prompt", "should resend original prompt");
    strict_1.default.equal(sentTurns[0]?.runId, "run-1", "should reuse original runId");
    strict_1.default.equal(store_1.useStore.getState().pendingAccountSwitch, undefined);
    strict_1.default.equal(store_1.useStore.getState().accountSwitchLoading, false);
});
(0, node_test_1.default)("confirmAccountSwitch with reason=manual switches account but does NOT auto-retry", async () => {
    const sentTurns = [];
    const activatedIds = [];
    const candidate = makeAccount("acc-2", "codex");
    const savedTurnInput = {
        runId: "run-1",
        stepId: "chat-run-1",
        prompt: "original prompt",
    };
    seedStore(makeClient({
        activateProviderAccount: async (id) => { activatedIds.push(id); },
        sendTurn: (input) => { sentTurns.push(input); return emptyStream(); },
        listRunHistory: async () => [],
        listProviderAccounts: async () => [],
    }), []);
    store_1.useStore.setState({
        pendingAccountSwitch: {
            providerKey: "codex",
            failedAccountId: "acc-1",
            failedAccountLabel: "acc-1@example.com",
            candidateAccount: candidate,
            reason: "manual",
        },
        lastTurnInput: savedTurnInput,
        status: "idle",
    });
    await store_1.useStore.getState().confirmAccountSwitch();
    strict_1.default.deepEqual(activatedIds, ["acc-2"], "should activate candidate account");
    strict_1.default.equal(sentTurns.length, 0, "manual switch must NOT auto-retry");
    strict_1.default.equal(store_1.useStore.getState().pendingAccountSwitch, undefined);
    strict_1.default.equal(store_1.useStore.getState().accountSwitchLoading, false);
});
(0, node_test_1.default)("already-tried account is not offered as switch candidate again", async () => {
    const active = makeAccount("acc-1", "codex", { isActive: true, remaining5hPercent: 0, remaining7dPercent: 0, slotIndex: 0 });
    const tried = makeAccount("acc-2", "codex", { isActive: false, remaining5hPercent: 80, remaining7dPercent: 80, slotIndex: 1 });
    const fresh = makeAccount("acc-3", "codex", { isActive: false, remaining5hPercent: 40, remaining7dPercent: 40, slotIndex: 2 });
    seedStore(makeClient({
        startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
        sendTurn: () => turnFailedStream("usage limit reached", false),
        listRunHistory: async () => [],
    }), []);
    store_1.useStore.setState({
        providerAccounts: [active, tried, fresh],
        selectedProvider: "codex",
        runId: undefined,
        activeStepId: undefined,
        status: "idle",
        timeline: [],
        _accountSwitchTriedIds: ["acc-2"], // acc-2 already tried
    });
    await store_1.useStore.getState().sendPrompt("hello");
    strict_1.default.equal(store_1.useStore.getState().pendingAccountSwitch?.candidateAccount.id, "acc-3", "should skip already-tried acc-2");
});
(0, node_test_1.default)("requestManualAccountSwitch sets pendingAccountSwitch with reason=manual without updating triedIds", () => {
    const active = makeAccount("acc-1", "codex", { isActive: true, remaining5hPercent: 80, remaining7dPercent: 80, slotIndex: 0 });
    const backup = makeAccount("acc-2", "codex", { isActive: false, remaining5hPercent: 60, remaining7dPercent: 60, slotIndex: 1 });
    seedStore(makeClient(), []);
    store_1.useStore.setState({ providerAccounts: [active, backup], selectedProvider: "codex" });
    store_1.useStore.getState().requestManualAccountSwitch();
    const state = store_1.useStore.getState();
    strict_1.default.ok(state.pendingAccountSwitch !== undefined);
    strict_1.default.equal(state.pendingAccountSwitch?.reason, "manual");
    strict_1.default.equal(state.pendingAccountSwitch?.candidateAccount.id, "acc-2");
    strict_1.default.deepEqual(state._accountSwitchTriedIds, [], "_accountSwitchTriedIds must not be touched by manual switch");
});
(0, node_test_1.default)("confirmProviderSwitch records handoff diagnostics in the new timeline", async () => {
    const timeline = [{ kind: "prompt", id: "prompt-1", text: "existing" }];
    seedStore(makeClient({
        handoffContext: async () => ({
            sourceRunId: "run-old",
            sourceProviderKey: "claude",
            targetProviderKey: "codex",
            prompt: "handoff prompt",
            includedTurnCount: 3,
            omittedTurnCount: 0,
            truncated: false,
            handoffMode: "hybrid",
        }),
        startRun: async () => ({ runId: "run-new", providerSessionId: "session-new", providerKey: "codex", status: "running", stepId: "chat-run-new" }),
        sendTurn: () => emptyStream(),
        listRunHistory: async () => [],
    }), []);
    store_1.useStore.setState({
        pendingProviderSwitch: {
            sourceRunId: "run-old",
            sourceProviderKey: "claude",
            sourceRunStatus: "completed",
            targetProviderKey: "codex",
            targetModel: undefined,
        },
        selectedProjectId: "project-1",
        chatMode: "normal_chat",
        runId: "run-old",
        selectedProvider: "claude",
        selectedModel: "claude-sonnet-4",
        status: "completed",
        timeline,
    });
    await store_1.useStore.getState().confirmProviderSwitch();
    const notice = store_1.useStore.getState().timeline.find((item) => item.kind === "system" && item.id.startsWith("handoff-mode-"));
    strict_1.default.ok(notice, "handoff diagnostic should be stored in timeline");
    strict_1.default.ok(notice.text.includes("hybrid"), "handoff diagnostic should include handoff mode");
});
(0, node_test_1.default)("Claude usage-limit failure with null quota offers fallback switch candidate", async () => {
    const active = makeAccount("cl-1", "claude", { isActive: true, remaining5hPercent: null, remaining7dPercent: null, slotIndex: 0 });
    const backup = makeAccount("cl-2", "claude", { isActive: false, remaining5hPercent: null, remaining7dPercent: null, slotIndex: 1 });
    seedStore(makeClient({
        startRun: async () => ({ runId: "run-c", providerSessionId: "sc", providerKey: "claude", status: "running", stepId: "chat-run-c" }),
        sendTurn: () => turnFailedStream("usage limit reached", false, "run-c"),
        listRunHistory: async () => [],
    }), []);
    store_1.useStore.setState({ providerAccounts: [active, backup], selectedProvider: "claude", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });
    await store_1.useStore.getState().sendPrompt("hello");
    const state = store_1.useStore.getState();
    strict_1.default.ok(state.pendingAccountSwitch !== undefined, "should offer switch even without quota telemetry");
    strict_1.default.equal(state.pendingAccountSwitch?.candidateAccount.id, "cl-2");
});
(0, node_test_1.default)("Claude account switch retries the original failed turn", async () => {
    const sentTurns = [];
    const activatedIds = [];
    const candidate = makeAccount("cl-2", "claude", {
        remaining5hPercent: null,
        remaining7dPercent: null,
    });
    const savedTurnInput = {
        runId: "run-claude",
        stepId: "chat-run-claude",
        prompt: "continue the Claude task",
        model: "claude-sonnet-4",
    };
    seedStore(makeClient({
        activateProviderAccount: async (id) => { activatedIds.push(id); },
        sendTurn: (input) => { sentTurns.push(input); return emptyStream(); },
        listRunHistory: async () => [],
        listProviderAccounts: async () => [],
    }), []);
    store_1.useStore.setState({
        selectedProvider: "claude",
        pendingAccountSwitch: {
            providerKey: "claude",
            failedAccountId: "cl-1",
            failedAccountLabel: "cl-1@example.com",
            candidateAccount: candidate,
            reason: "usage_limit",
        },
        lastTurnInput: savedTurnInput,
        status: "failed",
    });
    await store_1.useStore.getState().confirmAccountSwitch();
    strict_1.default.deepEqual(activatedIds, ["cl-2"]);
    strict_1.default.deepEqual(sentTurns, [savedTurnInput]);
    strict_1.default.equal(store_1.useStore.getState().selectedProvider, "claude");
});
(0, node_test_1.default)("MockRunnerClient preserves parent orchestration state across refresh pause resume feedback and stop", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    const spawn = await client.spawnAgent({ parentRunId: "parent-1", agent: "architect", prompt: "draft the design" });
    const initial = await client.refreshAgentGraph("parent-1");
    const paused = await client.pauseAgentLoop("parent-1");
    const feedback = await client.injectAgentFeedback("parent-1", spawn.runId, "revise the draft");
    const resumed = await client.resumeAgentLoop("parent-1");
    const stopped = await client.stopAgentLoop("parent-1");
    strict_1.default.equal(initial.runs[0]?.runId, spawn.runId);
    strict_1.default.equal(paused.loopState.status, "paused");
    strict_1.default.equal(feedback.busMessages.at(-1)?.message, "revise the draft");
    strict_1.default.equal(resumed.busMessages.at(-1)?.message, "revise the draft");
    strict_1.default.equal(stopped.loopState.status, "stopped");
});
(0, node_test_1.default)("MockRunnerClient keeps child agent runs out of main history", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    const parent = await client.startRun({
        projectId: "proj-web",
        workflowId: "wf-feature",
        providerKey: "codex",
        chatMode: "normal_chat",
    });
    const child = await client.spawnAgent({
        parentRunId: parent.runId,
        agent: "reviewer",
        prompt: "review the main run",
    });
    const history = await client.listRunHistory("proj-web");
    strict_1.default.deepEqual(history.map((item) => item.runId), [parent.runId]);
    strict_1.default.equal(history.some((item) => item.runId === child.runId), false);
});
(0, node_test_1.default)("sendPrompt keeps the parent orchestration stream alive after the turn completes", async () => {
    const parentGraph = {
        parentRunId: "current-run",
        runs: [],
        edges: [],
        busMessages: [],
        loopState: { status: "running", round: 0, roundCap: 3 },
    };
    const graphEvent = {
        ...BASE_EVENT,
        seq: 2,
        type: "agent_graph_updated",
        agentGraphSnapshot: {
            ...parentGraph,
            busMessages: [
                {
                    id: "bus-live",
                    parentRunId: "current-run",
                    kind: "handoff",
                    message: "ready-for-review",
                    queued: false,
                    occurredAt: "2026-01-01T00:00:01Z",
                },
            ],
        },
    };
    const gate = deferred();
    seedStore(makeClient({
        sendTurn: async function* () {
            yield { ...BASE_EVENT, type: "turn_completed", finalMessage: "done" };
        },
        streamRun: async function* () {
            await gate.promise;
            yield graphEvent;
        },
        startRun: async () => ({ runId: "current-run", providerSessionId: "session-1", providerKey: "codex", status: "running", stepId: "chat-current-run" }),
        listRunHistory: async () => [],
    }), []);
    await store_1.useStore.getState().sendPrompt("hello");
    gate.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.roundCap, 3);
    strict_1.default.equal(store_1.useStore.getState().agentBusMessages.at(-1)?.message, "ready-for-review");
});
// BUG-110: consumeOrchestrationStream called applyEvent which called applyTimelineEvent,
// which unconditionally adds a thinking row for agent_graph_updated events. After history
// replay settled (no thinking row), the orchestration stream re-added the thinking row.
(0, node_test_1.default)("openHistoryRun orchestration stream does not add thinking row after history replay completes", async () => {
    const graphSnapshot = {
        parentRunId: "run-1",
        runs: [{ runId: "child-run", agentName: "child", role: "worker", status: "completed", createdAt: "2026-01-01T00:00:00Z" }],
        edges: [],
        busMessages: [],
        loopState: { status: "running", round: 1, roundCap: 3 },
    };
    const graphEvent = {
        ...BASE_EVENT,
        workflowRunId: "run-1",
        seq: 5,
        type: "agent_graph_updated",
        agentGraphSnapshot: graphSnapshot,
    };
    // streamRun is called twice: first for history replay, second for orchestration stream.
    // Call 1: emit turn_started + turn_completed so history replay finishes cleanly.
    // Call 2: emit agent_graph_updated (which must NOT add a thinking row).
    let streamCallCount = 0;
    const orchestrationGate = deferred();
    seedStore(makeClient({
        resumeRun: async () => ({
            runId: "run-1",
            providerSessionId: "session-1",
            providerKey: "codex",
            status: "completed",
            stepId: "chat-run-1",
        }),
        streamRun: async function* () {
            streamCallCount++;
            if (streamCallCount === 1) {
                // History replay stream: complete the run
                yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 1, type: "turn_started", providerTurnId: "t1", prompt: "hello" };
                yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 2, type: "message_completed", id: "msg-1", text: "hi" };
                yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 3, type: "turn_completed", finalMessage: "hi" };
            }
            else {
                // Orchestration stream: deliver agent_graph_updated then stop
                await orchestrationGate.promise;
                yield graphEvent;
            }
        },
        listSkills: async () => [],
    }), []);
    await store_1.useStore.getState().openHistoryRun("run-1");
    // Allow history replay to process
    await new Promise((resolve) => setTimeout(resolve, 0));
    // History replay is now done (turn_completed processed, no thinking row)
    strict_1.default.equal(store_1.useStore.getState().timeline.some((it) => it.kind === "thinking"), false, "no thinking after history replay");
    // Now let the orchestration stream emit its agent_graph_updated event
    orchestrationGate.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    strict_1.default.equal(store_1.useStore.getState().timeline.some((it) => it.kind === "thinking"), false, "no thinking after orchestration stream processes agent_graph_updated");
    strict_1.default.deepEqual(store_1.useStore.getState().agentRuns, graphSnapshot.runs, "agentRuns updated correctly");
});
(0, node_test_1.default)("openHistoryRun orchestration stream updates agent graph without touching timeline content", async () => {
    const graphSnapshot = {
        parentRunId: "run-1",
        runs: [{ runId: "child-run", agentName: "child", role: "worker", status: "running", createdAt: "2026-01-01T00:00:00Z" }],
        edges: [],
        busMessages: [{ id: "bus-1", parentRunId: "run-1", kind: "handoff", message: "done", queued: false, occurredAt: "2026-01-01T00:00:00Z" }],
        loopState: { status: "running", round: 1, roundCap: 5 },
    };
    let streamCallCount = 0;
    const orchestrationGate = deferred();
    seedStore(makeClient({
        resumeRun: async () => ({
            runId: "run-1",
            providerSessionId: "session-1",
            providerKey: "codex",
            status: "completed",
            stepId: "chat-run-1",
        }),
        streamRun: async function* () {
            streamCallCount++;
            if (streamCallCount === 1) {
                yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 1, type: "turn_started", providerTurnId: "t1", prompt: "hi" };
                yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 2, type: "message_completed", id: "msg-1", text: "response text" };
                yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 3, type: "turn_completed", finalMessage: "response text" };
            }
            else {
                await orchestrationGate.promise;
                yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 10, type: "agent_graph_updated", agentGraphSnapshot: graphSnapshot };
            }
        },
        listSkills: async () => [],
    }), []);
    await store_1.useStore.getState().openHistoryRun("run-1");
    await new Promise((resolve) => setTimeout(resolve, 0));
    const timelineBeforeOrchestration = store_1.useStore.getState().timeline.filter((it) => it.kind !== "thinking");
    orchestrationGate.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    const state = store_1.useStore.getState();
    // Timeline content must be identical (no items added or removed by orchestration stream)
    strict_1.default.deepEqual(state.timeline.filter((it) => it.kind !== "thinking"), timelineBeforeOrchestration, "timeline content unchanged by orchestration stream");
    // Agent graph data must be updated
    strict_1.default.equal(state.agentGraphSnapshot?.loopState.roundCap, 5);
    strict_1.default.equal(state.agentBusMessages[0]?.message, "done");
});
// BUG-112: re-opening a multi-turn completed run from the history panel must replay the
// WHOLE transcript. Previously the replay stopped at the first turn_completed, so the
// latest response was missing and the prior turn appeared as the latest.
(0, node_test_1.default)("openHistoryRun replays all turns of a completed multi-turn run (BUG-112)", async () => {
    const streamStarted = deferred();
    const handle = {
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "completed",
        stepId: "chat-run-1",
        lastEventSeq: 6, // last persisted event (turn 2's turn_completed)
    };
    async function* multiTurnStream() {
        streamStarted.resolve();
        // Turn 1
        yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 1, type: "turn_started", providerTurnId: "t1", prompt: "first" };
        yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 2, type: "message_completed", id: "m1", text: "first response" };
        yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 3, type: "turn_completed", finalMessage: "first response" };
        // Turn 2 — must NOT be skipped by an early stop at turn 1's turn_completed
        yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 4, type: "turn_started", providerTurnId: "t2", prompt: "second" };
        yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 5, type: "message_completed", id: "m2", text: "latest response" };
        yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 6, type: "turn_completed", finalMessage: "latest response" };
        // Open-ended (completed runs keep the SSE open); replay must stop on its own at seq 6.
        await new Promise(() => { });
    }
    seedStore(makeClient({
        resumeRun: async () => handle,
        streamRun: () => multiTurnStream(),
        listSkills: async () => [],
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    await Promise.race([
        streamStarted.promise,
        new Promise((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
    ]);
    await new Promise((resolve) => setTimeout(resolve, 0));
    const assistantTexts = store_1.useStore.getState().timeline
        .filter((item) => item.kind === "assistant")
        .map((item) => (item.kind === "assistant" ? item.text : ""));
    strict_1.default.deepEqual(assistantTexts, ["first response", "latest response"], "both turns must be replayed");
    strict_1.default.equal(store_1.useStore.getState().status, "completed");
    strict_1.default.equal(store_1.useStore.getState().timeline.some((item) => item.kind === "thinking"), false);
});
// BUG-118: opening a chat replays its transcript, driving status running→completed just like
// a live turn. _historyReplaying must be true for the duration so RunToast suppresses the
// spurious "AI response complete" notification, and cleared once the replay finishes.
(0, node_test_1.default)("openHistoryRun sets _historyReplaying during replay and clears it after (BUG-118)", async () => {
    const gate = deferred();
    async function* slowReplay() {
        await gate.promise; // keep the replay in-flight until released
    }
    seedStore(makeClient({
        resumeRun: async () => ({ runId: "run-1", providerSessionId: "s", providerKey: "codex", status: "completed", stepId: "chat-run-1" }),
        streamRun: () => slowReplay(),
        listSkills: async () => [],
    }), [
        {
            runId: "run-1",
            projectId: "project-1",
            providerKey: "codex",
            status: "completed",
            startedAt: "2026-06-17T10:00:00Z",
            updatedAt: "2026-06-17T10:05:00Z",
        },
    ]);
    await store_1.useStore.getState().openHistoryRun("run-1");
    strict_1.default.equal(store_1.useStore.getState()._historyReplaying, true, "flag must be set while the replay is in flight");
    gate.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    strict_1.default.equal(store_1.useStore.getState()._historyReplaying, false, "flag must clear once the replay completes");
});
