/**
 * run-63960: after restart, freeform chat while loop is blocked returns
 * flow_awaiting_user 409. Desktop must map that to status=blocked (not Failed)
 * and refresh agent graph so FlowAwaitingUserCard (Continue/Stop) can render.
 *
 * additive-tests-only + cross-provider-parity (shared UI path).
 */
import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import type {
  AgentGraphSnapshot,
  ProviderEventDTO,
  ProviderKey,
  RunHistoryItem,
  RunnerClient,
} from "../types/contract";

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

function blockedGraph(parentRunId: string, providerKey: ProviderKey): AgentGraphSnapshot {
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

function makeClient(overrides: Partial<RunnerClient> = {}): RunnerClient {
  const base: RunnerClient = {
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
    deleteRun: async () => {},
    restoreChatRun: async (input) => ({
      runId: input.sourceRunId,
      sourceMachineId: input.sourceMachineId,
      sourceRunId: input.sourceRunId,
      providerKey: "codex",
      restoreStatus: "restored",
    }),
    sendTurn: () => emptyStream(),
    submitApproval: async () => {},
    answerQuestion: async () => {},
    interrupt: async () => {},
    streamRun: () => emptyStream(),
    focusAgentRun: () => emptyStream(),
    listArtifacts: async () => [],
    listSkills: async () => [],
    connectProviderAccount: async () => {},
    activateProviderAccount: async () => {},
    applyGrokYoloPosture: async () => {},
    openProviderAccountTerminal: async () => {},
    restartStack: async () => {},
    shutdownStack: async () => {},
  };
  return { ...base, ...overrides };
}

function seedBlockedRun(client: RunnerClient): void {
  useStore.setState({
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
    runHistory: [] as RunHistoryItem[],
    agentGraphSnapshot: undefined,
    agentSpawnGuideOpen: false,
    _runReplaySeq: {},
    _runSnapshots: {},
    _historyReplaying: false,
    _streamRunSeq: 0,
    _agentGraphLoadSeq: 0,
  });
}

for (const providerKey of ["codex", "claude", "grok"] as const satisfies ProviderKey[]) {
  test(`sendPrompt maps flow_awaiting_user 409 to blocked + graph refresh for ${providerKey}`, async () => {
    let refreshCalls = 0;
    const graph = blockedGraph("run-63960", providerKey);
    seedBlockedRun(
      makeClient({
        sendTurn: () =>
          (async function* (): AsyncIterable<ProviderEventDTO> {
            throw new RunnerApiError(
              409,
              "flow_awaiting_user",
              "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn",
            );
          })(),
        refreshAgentGraph: async () => {
          refreshCalls += 1;
          return graph;
        },
      }),
    );

    await useStore.getState().sendPrompt("ok");

    // Allow best-effort graph refresh promise to settle.
    await new Promise((r) => setTimeout(r, 30));

    const state = useStore.getState();
    assert.equal(state.status, "blocked", "must not map awaiting-user to Failed");
    assert.equal(state.recoverable, false);
    assert.ok(
      !state.timeline.some((it) => it.kind === "thinking"),
      "stale Thinking must clear",
    );
    assert.ok(
      state.timeline.some(
        (it) => it.kind === "system" && it.tone === "warn" && /waiting for your decision/i.test(it.text),
      ),
      "warn system row should explain awaiting user",
    );
    assert.ok(refreshCalls >= 1, "must refresh agent graph so Continue/Stop card has loopState");
    assert.equal(state.agentGraphSnapshot?.loopState.status, "blocked");
    assert.equal(state.agentGraphSnapshot?.loopState.blockReason, "cap");
  });
}

test("other 409 codes still map to failed (non-regression)", async () => {
  seedBlockedRun(
    makeClient({
      sendTurn: () =>
        (async function* (): AsyncIterable<ProviderEventDTO> {
          throw new RunnerApiError(409, "invalid_request", "bad prompt shape");
        })(),
    }),
  );

  await useStore.getState().sendPrompt("ok");

  const state = useStore.getState();
  assert.equal(state.status, "failed");
  assert.ok(state.timeline.some((it) => it.kind === "system" && it.tone === "error"));
});

test("non-409 errors with decision wording still fail (no false blocked)", async () => {
  seedBlockedRun(
    makeClient({
      sendTurn: () =>
        (async function* (): AsyncIterable<ProviderEventDTO> {
          throw new RunnerApiError(500, "internal", "waiting for your decision later");
        })(),
    }),
  );

  await useStore.getState().sendPrompt("ok");
  assert.equal(useStore.getState().status, "failed");
});

test("openHistoryRun seeds agent graph early for blocked resume", async () => {
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
  useStore.setState({
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

  await useStore.getState().openHistoryRun("run-63960");
  await new Promise((r) => setTimeout(r, 40));

  const state = useStore.getState();
  assert.equal(state.status, "blocked");
  assert.ok(refreshCalls >= 1, "early refreshAgentGraph on history open");
  assert.equal(state.agentGraphSnapshot?.loopState.status, "blocked");
});

test("late blocked HTTP graph refresh does not overwrite post-Continue graph", async () => {
  let resolveRefresh!: (snap: AgentGraphSnapshot) => void;
  const refreshPromise = new Promise<AgentGraphSnapshot>((r) => {
    resolveRefresh = r;
  });
  seedBlockedRun(
    makeClient({
      sendTurn: () =>
        (async function* (): AsyncIterable<ProviderEventDTO> {
          throw new RunnerApiError(
            409,
            "flow_awaiting_user",
            "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn",
          );
        })(),
      refreshAgentGraph: async () => refreshPromise,
      continueFlow: async () => ({
        parentRunId: "run-63960",
        runs: [],
        edges: [],
        busMessages: [],
        loopState: { status: "running", round: 4, roundCap: 6 },
      }),
    }),
  );

  const sendP = useStore.getState().sendPrompt("ok");
  await sendP;
  // Continue bumps graph load seq before late refresh resolves.
  await useStore.getState().continueFlow("more rounds");
  resolveRefresh(blockedGraph("run-63960", "codex"));
  await new Promise((r) => setTimeout(r, 30));

  const state = useStore.getState();
  assert.equal(state.agentGraphSnapshot?.loopState.status, "running");
  assert.notEqual(state.agentGraphSnapshot?.loopState.status, "blocked");
});
