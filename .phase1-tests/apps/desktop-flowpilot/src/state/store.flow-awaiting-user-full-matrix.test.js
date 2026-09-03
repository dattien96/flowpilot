"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
/**
 * Full R3 matrix for run-63960 desktop awaiting-user / blocked restart UX.
 * additive-tests-only + cross-provider-parity.
 *
 * Axes:
 *  - providers: codex / claude / grok
 *  - blockReason: cap / escalate / member_stalled
 *  - error shape: code flow_awaiting_user | 409+message fallback
 *  - lifecycle: history open, freeform 409, Continue, Stop
 *  - degraded: graph refresh fails, switch-run discards stale graph
 *  - non-regression: other 409 / non-409 message still Failed
 */
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
const HttpWsRunnerClient_1 = require("../client/HttpWsRunnerClient");
async function* emptyStream() { }
const PROVIDERS = ["codex", "claude", "grok"];
const BLOCK_REASONS = ["cap", "escalate", "member_stalled"];
function blockedGraph(parentRunId, providerKey, blockReason = "cap") {
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
            blockReason,
            openIssues: blockReason === "cap" ? 2 : 1,
            gateReason: `${blockReason} gate reason`,
            activeNode: blockReason === "member_stalled" ? "reviewer_security" : "synthesis",
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
        refreshAgentGraph: async () => blockedGraph("run-63960", "codex"),
        pauseAgentLoop: async () => blockedGraph("run-63960", "codex"),
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
            loopState: { status: "stopped", round: 3, roundCap: 3, gateReason: "stopped" },
        }),
        continueFlow: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 4, roundCap: 5 },
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
function seedRun(client, extras = {}) {
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
        ...extras,
    });
}
function awaitingUserError(kind) {
    if (kind === "code") {
        return new HttpWsRunnerClient_1.RunnerApiError(409, "flow_awaiting_user", "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn");
    }
    // code may be generic but message + 409 still maps
    return new HttpWsRunnerClient_1.RunnerApiError(409, "conflict", "waiting for your decision before a new turn");
}
// --- freeform 409 mapping matrix ---
for (const providerKey of PROVIDERS) {
    for (const errKind of ["code", "message409"]) {
        for (const blockReason of BLOCK_REASONS) {
            (0, node_test_1.default)(`sendPrompt 409→blocked ${providerKey}/${errKind}/${blockReason}`, async () => {
                const graph = blockedGraph("run-63960", providerKey, blockReason);
                seedRun(makeClient({
                    sendTurn: () => (async function* () {
                        throw awaitingUserError(errKind);
                    })(),
                    refreshAgentGraph: async () => graph,
                }));
                await store_1.useStore.getState().sendPrompt("ok");
                await new Promise((r) => setTimeout(r, 25));
                const s = store_1.useStore.getState();
                strict_1.default.equal(s.status, "blocked");
                strict_1.default.ok(!s.timeline.some((it) => it.kind === "thinking"));
                strict_1.default.ok(s.timeline.some((it) => it.kind === "system" && it.tone === "warn"));
                strict_1.default.equal(s.agentGraphSnapshot?.loopState.status, "blocked");
                strict_1.default.equal(s.agentGraphSnapshot?.loopState.blockReason, blockReason);
            });
        }
    }
}
// --- history open matrix ---
for (const providerKey of PROVIDERS) {
    for (const blockReason of BLOCK_REASONS) {
        (0, node_test_1.default)(`openHistoryRun seeds graph ${providerKey}/${blockReason}`, async () => {
            const graph = blockedGraph("run-63960", providerKey, blockReason);
            let refresh = 0;
            seedRun(makeClient({
                resumeRun: async (runId) => ({
                    runId,
                    providerSessionId: "session-1",
                    providerKey,
                    status: "blocked",
                }),
                refreshAgentGraph: async () => {
                    refresh += 1;
                    return graph;
                },
                streamRun: () => emptyStream(),
            }), {
                runId: undefined,
                mainRunId: undefined,
                status: "idle",
                timeline: [],
                runHistory: [
                    {
                        runId: "run-63960",
                        projectId: "project-1",
                        status: "blocked",
                        providerKey,
                        startedAt: "2026-07-23T16:00:00Z",
                        updatedAt: "2026-07-23T16:30:00Z",
                        runKind: "chat",
                        subMode: "bug",
                    },
                ],
            });
            await store_1.useStore.getState().openHistoryRun("run-63960");
            await new Promise((r) => setTimeout(r, 40));
            const s = store_1.useStore.getState();
            strict_1.default.equal(s.status, "blocked");
            strict_1.default.ok(refresh >= 1);
            strict_1.default.equal(s.agentGraphSnapshot?.loopState.blockReason, blockReason);
        });
    }
}
// --- Continue / Stop after 409 ---
for (const providerKey of PROVIDERS) {
    (0, node_test_1.default)(`Continue after 409 unparks ${providerKey}`, async () => {
        seedRun(makeClient({
            sendTurn: () => (async function* () {
                throw awaitingUserError("code");
            })(),
            refreshAgentGraph: async () => blockedGraph("run-63960", providerKey),
            continueFlow: async () => ({
                parentRunId: "run-63960",
                runs: [],
                edges: [],
                busMessages: [],
                loopState: { status: "running", round: 4, roundCap: 5 },
            }),
        }));
        await store_1.useStore.getState().sendPrompt("ok");
        await new Promise((r) => setTimeout(r, 25));
        await store_1.useStore.getState().continueFlow("go");
        strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.status, "running");
    });
    (0, node_test_1.default)(`Stop after 409 seals ${providerKey}`, async () => {
        seedRun(makeClient({
            sendTurn: () => (async function* () {
                throw awaitingUserError("code");
            })(),
            refreshAgentGraph: async () => blockedGraph("run-63960", providerKey),
            stopAgentLoop: async () => ({
                parentRunId: "run-63960",
                runs: [],
                edges: [],
                busMessages: [],
                loopState: { status: "stopped", round: 3, roundCap: 3, gateReason: "stopped" },
            }),
        }));
        await store_1.useStore.getState().sendPrompt("ok");
        await new Promise((r) => setTimeout(r, 25));
        await store_1.useStore.getState().stop();
        const s = store_1.useStore.getState();
        strict_1.default.equal(s.status, "cancelled");
        strict_1.default.equal(s.agentGraphSnapshot?.loopState.status, "stopped");
    });
}
// --- degraded / race ---
(0, node_test_1.default)("graph refresh failure still leaves status blocked (not Failed)", async () => {
    seedRun(makeClient({
        sendTurn: () => (async function* () {
            throw awaitingUserError("code");
        })(),
        refreshAgentGraph: async () => {
            throw new Error("network down");
        },
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    await new Promise((r) => setTimeout(r, 25));
    strict_1.default.equal(store_1.useStore.getState().status, "blocked");
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot, undefined);
});
(0, node_test_1.default)("switching run discards late graph for previous blocked refresh", async () => {
    let resolveRefresh;
    const pending = new Promise((r) => {
        resolveRefresh = r;
    });
    seedRun(makeClient({
        sendTurn: () => (async function* () {
            throw awaitingUserError("code");
        })(),
        refreshAgentGraph: async () => pending,
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    // Switch to another run before refresh resolves.
    store_1.useStore.setState({
        runId: "run-other",
        mainRunId: "run-other",
        status: "running",
        agentGraphSnapshot: {
            parentRunId: "run-other",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 0, roundCap: 3 },
        },
    });
    resolveRefresh(blockedGraph("run-63960", "codex"));
    await new Promise((r) => setTimeout(r, 30));
    const s = store_1.useStore.getState();
    strict_1.default.equal(s.runId, "run-other");
    strict_1.default.equal(s.agentGraphSnapshot?.parentRunId, "run-other");
    strict_1.default.equal(s.agentGraphSnapshot?.loopState.status, "running");
});
(0, node_test_1.default)("late blocked refresh does not overwrite Stop", async () => {
    let resolveRefresh;
    const pending = new Promise((r) => {
        resolveRefresh = r;
    });
    seedRun(makeClient({
        sendTurn: () => (async function* () {
            throw awaitingUserError("code");
        })(),
        refreshAgentGraph: async () => pending,
        stopAgentLoop: async () => ({
            parentRunId: "run-63960",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "stopped", round: 3, roundCap: 3, gateReason: "stopped" },
        }),
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    await store_1.useStore.getState().stop();
    resolveRefresh(blockedGraph("run-63960", "codex"));
    await new Promise((r) => setTimeout(r, 30));
    strict_1.default.equal(store_1.useStore.getState().agentGraphSnapshot?.loopState.status, "stopped");
});
// --- non-regression false positives ---
(0, node_test_1.default)("409 invalid_request still failed", async () => {
    seedRun(makeClient({
        sendTurn: () => (async function* () {
            throw new HttpWsRunnerClient_1.RunnerApiError(409, "invalid_request", "bad shape");
        })(),
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    strict_1.default.equal(store_1.useStore.getState().status, "failed");
});
(0, node_test_1.default)("500 with decision wording still failed", async () => {
    seedRun(makeClient({
        sendTurn: () => (async function* () {
            throw new HttpWsRunnerClient_1.RunnerApiError(500, "internal", "waiting for your decision later");
        })(),
    }));
    await store_1.useStore.getState().sendPrompt("ok");
    strict_1.default.equal(store_1.useStore.getState().status, "failed");
});
// --- status derivation + timeline settle for all block reasons ---
for (const blockReason of BLOCK_REASONS) {
    (0, node_test_1.default)(`derive+settle blocked ${blockReason}`, () => {
        const snap = blockedGraph("run-x", "codex", blockReason);
        strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snap), "blocked");
        const timeline = [
            { kind: "tool", id: "t1", toolName: "flowpilot__submit_review_outcome", status: "running" },
            { kind: "thinking", id: "th", text: "Thinking..." },
        ];
        const settled = (0, store_1.settleCompletedFlowTimeline)(timeline);
        strict_1.default.ok(!settled.some((it) => it.kind === "thinking"));
        strict_1.default.equal(settled.find((it) => it.kind === "tool")?.status, "success");
    });
}
// BUG-231 legacy: running child still wins over blocked loop for status.
(0, node_test_1.default)("running child still wins over blocked loop (BUG-231)", () => {
    const snap = {
        parentRunId: "run-x",
        runs: [
            {
                runId: "c1",
                agentName: "coder",
                role: "coder",
                status: "running",
                parentRunId: "run-x",
                createdAt: "2026-07-23T16:00:00Z",
                providerKey: "codex",
            },
        ],
        edges: [],
        busMessages: [],
        loopState: {
            status: "blocked",
            round: 3,
            roundCap: 3,
            blockReason: "cap",
        },
    };
    strict_1.default.equal((0, store_1.deriveOrchestrationRunStatus)("running", snap), "running");
});
