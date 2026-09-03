"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
/**
 * run-75035 desktop isolation matrix (additive-tests-only).
 *
 * Backend seed pollution is the primary fix. These tests lock the shared-timeline
 * desktop contracts that cannot catch backend-tagged child pollution but still
 * prevent live main freeform / orchestration events from bleeding into a focused child.
 *
 * Provider-agnostic Case 1 (store path does not branch on providerKey) — matrix
 * still exercises codex/claude/grok seeds for parity documentation.
 */
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
async function* emptyStream() { }
const PROVIDERS = ["codex", "claude", "grok"];
const BASE_EVENT = {
    id: "evt",
    workflowRunId: "main-run",
    seq: 0, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
    ts: "2026-07-23T21:00:00Z",
    providerKey: "codex",
};
function deferred() {
    let resolve;
    let reject;
    const promise = new Promise((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
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
            parentRunId: "main-run",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "done", round: 1, roundCap: 3 },
        }),
        pauseAgentLoop: async () => ({
            parentRunId: "main-run",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "paused", round: 0, roundCap: 3 },
        }),
        resumeAgentLoop: async () => ({
            parentRunId: "main-run",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 0, roundCap: 3 },
        }),
        injectAgentFeedback: async () => ({
            parentRunId: "main-run",
            runs: [],
            edges: [],
            busMessages: [],
            loopState: { status: "running", round: 0, roundCap: 3 },
        }),
        stopAgentLoop: async () => ({
            parentRunId: "main-run",
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
            runId: "main-run",
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
            prompt: "[mock]",
            includedTurnCount: 0,
            omittedTurnCount: 0,
            truncated: false,
            handoffMode: "raw",
        }),
        generateChatSummary: async (runId) => ({ runId, generated: true, skipped: false }),
        syncChatRun: async (runId) => ({
            runId,
            sourceMachineId: "m",
            sourceRunId: runId,
            syncStatus: "synced",
            syncedAt: "2026-07-23T00:00:00Z",
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
function seed(client, extras = {}) {
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
        chatMode: "workflow_step_auto",
        selectedProvider: "codex",
        selectedModel: undefined,
        reasoningEffort: undefined,
        yoloMode: false,
        chatStartMode: "normal",
        chatSourceDocId: "",
        runId: "main-run",
        mainRunId: "main-run",
        activeAgentRunId: undefined,
        agentRuns: [
            {
                runId: "child-coder",
                agentName: "coder-agent",
                role: "coder",
                status: "completed",
                parentRunId: "main-run",
                createdAt: "2026-07-23T21:00:00Z",
                providerKey: "codex",
            },
        ],
        agentBusMessages: [],
        activeStepId: "chat-main-run",
        status: "completed",
        timeline: [{ kind: "prompt", id: "p1", text: "hub timeline" }],
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
        _orchestrationStreamSeq: 0,
        ...extras,
    });
}
for (const provider of PROVIDERS) {
    (0, node_test_1.default)(`run75035: orchestration freeform message_delta does not bleed into focused child (${provider})`, async () => {
        const gate = deferred();
        seed(makeClient({
            streamRun: async function* (runId) {
                if (runId !== "main-run")
                    return;
                await gate.promise;
                yield {
                    ...BASE_EVENT,
                    id: "hub-delta",
                    providerKey: provider,
                    workflowRunId: "main-run",
                    seq: 10, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
                    type: "message_delta",
                    text: "Đúng, đã xong rồi",
                };
                yield {
                    ...BASE_EVENT,
                    id: "hub-done",
                    providerKey: provider,
                    workflowRunId: "main-run",
                    seq: 11, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
                    type: "turn_completed",
                    finalMessage: "Đúng, đã xong rồi",
                };
            },
        }), {
            selectedProvider: provider,
            // start orchestration stream for main while we will focus child
            status: "running",
        });
        // Kick orchestration consumer by calling backToMainRun path indirectly:
        // openHistoryRun-like: startOrchestrationStream is started by backToMainRun / openHistoryRun.
        // Manually focus child first with a clean timeline, then ensure main orchestration is live.
        store_1.useStore.setState({
            runId: "child-coder",
            activeAgentRunId: "child-coder",
            mainRunId: "main-run",
            timeline: [{ kind: "assistant", id: "coder-only", text: "CHILD_OWN_ONLY", finalized: true }],
            status: "completed",
        });
        // Re-enter main orchestration stream while still displaying child (simulates
        // openHistoryRun startOrchestrationStream + later focus child, or residual stream).
        // openHistoryRun sets runId=main then startOrchestrationStream; we reverse: stream
        // bound to main with runId already child via startOrchestration through backToMain then focus.
        seed(makeClient({
            streamRun: async function* (runId) {
                if (runId !== "main-run")
                    return;
                await gate.promise;
                yield {
                    ...BASE_EVENT,
                    id: "hub-delta",
                    providerKey: provider,
                    workflowRunId: "main-run",
                    seq: 10, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
                    type: "message_delta",
                    text: "Đúng, đã xong rồi",
                };
            },
            focusAgentRun: async function* () {
                yield {
                    ...BASE_EVENT,
                    id: "child-1",
                    providerKey: provider,
                    workflowRunId: "child-coder",
                    seq: 1, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
                    type: "message_delta",
                    text: "CHILD_OWN_ONLY",
                };
            },
            resumeRun: async (runId) => ({
                runId,
                providerSessionId: "s",
                providerKey: provider,
                status: "completed",
            }),
        }), { selectedProvider: provider, status: "running", timeline: [] });
        // Establish main + orchestration
        store_1.useStore.setState({
            runId: "main-run",
            mainRunId: "main-run",
            activeAgentRunId: undefined,
            timeline: [{ kind: "prompt", id: "hub-p", text: "done r hả" }],
            status: "running",
            _runReplaySeq: {},
        });
        // trigger startOrchestrationStream via backToMainRun after a fake child focus
        store_1.useStore.setState({
            _runSnapshots: {
                "main-run": {
                    status: "running",
                    timeline: [{ kind: "prompt", id: "hub-p", text: "done r hả" }],
                    artifacts: [],
                    pendingApprovals: [],
                    pendingQuestions: [],
                    lastEventSeq: 0,
                    recoverable: false,
                },
            },
        });
        // focus child (caches main, shows child stream)
        await store_1.useStore.getState().focusAgentRun("child-coder");
        await new Promise((r) => setTimeout(r, 0));
        // While child focused, main freeform arrives on orchestration stream.
        // consumeOrchestrationStream is started by focus? No — by backToMain/openHistory.
        // Ensure orchestration is running: call backToMain then re-focus.
        store_1.useStore.getState().backToMainRun();
        await new Promise((r) => setTimeout(r, 0));
        await store_1.useStore.getState().focusAgentRun("child-coder");
        await new Promise((r) => setTimeout(r, 0));
        const before = store_1.useStore.getState().timeline.map((i) => ("text" in i ? i.text : ""));
        gate.resolve();
        await new Promise((r) => setTimeout(r, 20));
        const texts = store_1.useStore
            .getState()
            .timeline.map((i) => ("text" in i ? String(i.text) : ""))
            .join("\n");
        strict_1.default.equal(texts.includes("Đúng, đã xong rồi"), false, `hub freeform must not appear on focused child timeline (${provider}); before=${JSON.stringify(before)} after=${texts}`);
    });
}
for (const provider of PROVIDERS) {
    (0, node_test_1.default)(`run75035: focusAgentRun ignores main-tagged freeform replay (${provider})`, async () => {
        seed(makeClient({
            resumeRun: async (runId) => ({
                runId,
                providerSessionId: "s",
                providerKey: provider,
                status: "completed",
            }),
            focusAgentRun: async function* () {
                yield {
                    ...BASE_EVENT,
                    providerKey: provider,
                    workflowRunId: "main-run",
                    seq: 1, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
                    type: "message_delta",
                    text: "Đúng, đã xong rồi",
                };
                yield {
                    ...BASE_EVENT,
                    providerKey: provider,
                    workflowRunId: "child-coder",
                    seq: 2, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
                    type: "message_delta",
                    text: "CHILD_OWN_ONLY",
                };
            },
        }), {
            selectedProvider: provider,
            timeline: [{ kind: "assistant", id: "hub-a", text: "hub old", finalized: true }],
        });
        await store_1.useStore.getState().focusAgentRun("child-coder");
        await new Promise((r) => setTimeout(r, 0));
        const assistantTexts = store_1.useStore
            .getState()
            .timeline.filter((i) => i.kind === "assistant")
            .map((i) => (i.kind === "assistant" ? i.text : ""));
        strict_1.default.deepEqual(assistantTexts, ["CHILD_OWN_ONLY"]);
        strict_1.default.equal(assistantTexts.some((t) => t.includes("Đúng, đã xong rồi")), false);
    });
}
(0, node_test_1.default)("run75035: backToMainRun restores hub snapshot without child-only assistant", async () => {
    seed(makeClient({
        resumeRun: async (runId) => ({
            runId,
            providerSessionId: "s",
            providerKey: "codex",
            status: "completed",
        }),
        focusAgentRun: async function* () {
            yield {
                ...BASE_EVENT,
                workflowRunId: "child-coder",
                seq: 1, providerSessionId: "ses-fix", occurredAt: "2026-01-01T00:00:00Z",
                type: "message_delta",
                text: "CHILD_OWN_ONLY",
            };
        },
        streamRun: () => emptyStream(),
    }), {
        timeline: [{ kind: "assistant", id: "hub-a", text: "HUB_ONLY", finalized: true }],
        status: "completed",
    });
    await store_1.useStore.getState().focusAgentRun("child-coder");
    await new Promise((r) => setTimeout(r, 0));
    strict_1.default.equal(store_1.useStore.getState().timeline.some((i) => i.kind === "assistant" && i.text.includes("CHILD_OWN_ONLY")), true);
    store_1.useStore.getState().backToMainRun();
    await new Promise((r) => setTimeout(r, 0));
    const texts = store_1.useStore
        .getState()
        .timeline.filter((i) => i.kind === "assistant")
        .map((i) => (i.kind === "assistant" ? i.text : ""));
    strict_1.default.equal(texts.includes("HUB_ONLY"), true);
    strict_1.default.equal(texts.some((t) => t.includes("CHILD_OWN_ONLY")), false);
});
