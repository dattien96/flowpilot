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
import test from "node:test";
import assert from "node:assert/strict";
import {
  deriveOrchestrationRunStatus,
  settleCompletedFlowTimeline,
  useStore,
  type TimelineItem,
} from "./store";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import type {
  AgentGraphSnapshot,
  ProviderEventDTO,
  ProviderKey,
  RunHistoryItem,
  RunnerClient,
} from "../types/contract";

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

const PROVIDERS = ["codex", "claude", "grok"] as const satisfies readonly ProviderKey[];
const BLOCK_REASONS = ["cap", "escalate", "member_stalled"] as const;

function blockedGraph(
  parentRunId: string,
  providerKey: ProviderKey,
  blockReason: (typeof BLOCK_REASONS)[number] = "cap",
): AgentGraphSnapshot {
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

function makeClient(overrides: Partial<RunnerClient> = {}): RunnerClient {
  const base: RunnerClient = {
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

function seedRun(client: RunnerClient, extras: Record<string, unknown> = {}): void {
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
    timeline: [{ kind: "thinking", id: "thinking-stale", text: "Thinking..." }] as TimelineItem[],
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
    ...extras,
  });
}

function awaitingUserError(kind: "code" | "message409"): RunnerApiError {
  if (kind === "code") {
    return new RunnerApiError(
      409,
      "flow_awaiting_user",
      "flow is waiting for your decision (Continue/Stop); resolve the form before a new turn",
    );
  }
  // code may be generic but message + 409 still maps
  return new RunnerApiError(409, "conflict", "waiting for your decision before a new turn");
}

// --- freeform 409 mapping matrix ---
for (const providerKey of PROVIDERS) {
  for (const errKind of ["code", "message409"] as const) {
    for (const blockReason of BLOCK_REASONS) {
      test(`sendPrompt 409→blocked ${providerKey}/${errKind}/${blockReason}`, async () => {
        const graph = blockedGraph("run-63960", providerKey, blockReason);
        seedRun(
          makeClient({
            sendTurn: () =>
              (async function* (): AsyncIterable<ProviderEventDTO> {
                throw awaitingUserError(errKind);
              })(),
            refreshAgentGraph: async () => graph,
          }),
        );
        await useStore.getState().sendPrompt("ok");
        await new Promise((r) => setTimeout(r, 25));
        const s = useStore.getState();
        assert.equal(s.status, "blocked");
        assert.ok(!s.timeline.some((it) => it.kind === "thinking"));
        assert.ok(s.timeline.some((it) => it.kind === "system" && it.tone === "warn"));
        assert.equal(s.agentGraphSnapshot?.loopState.status, "blocked");
        assert.equal(s.agentGraphSnapshot?.loopState.blockReason, blockReason);
      });
    }
  }
}

// --- history open matrix ---
for (const providerKey of PROVIDERS) {
  for (const blockReason of BLOCK_REASONS) {
    test(`openHistoryRun seeds graph ${providerKey}/${blockReason}`, async () => {
      const graph = blockedGraph("run-63960", providerKey, blockReason);
      let refresh = 0;
      seedRun(
        makeClient({
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
        }),
        {
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
          ] as RunHistoryItem[],
        },
      );
      await useStore.getState().openHistoryRun("run-63960");
      await new Promise((r) => setTimeout(r, 40));
      const s = useStore.getState();
      assert.equal(s.status, "blocked");
      assert.ok(refresh >= 1);
      assert.equal(s.agentGraphSnapshot?.loopState.blockReason, blockReason);
    });
  }
}

// --- Continue / Stop after 409 ---
for (const providerKey of PROVIDERS) {
  test(`Continue after 409 unparks ${providerKey}`, async () => {
    seedRun(
      makeClient({
        sendTurn: () =>
          (async function* (): AsyncIterable<ProviderEventDTO> {
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
      }),
    );
    await useStore.getState().sendPrompt("ok");
    await new Promise((r) => setTimeout(r, 25));
    await useStore.getState().continueFlow("go");
    assert.equal(useStore.getState().agentGraphSnapshot?.loopState.status, "running");
  });

  test(`Stop after 409 seals ${providerKey}`, async () => {
    seedRun(
      makeClient({
        sendTurn: () =>
          (async function* (): AsyncIterable<ProviderEventDTO> {
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
      }),
    );
    await useStore.getState().sendPrompt("ok");
    await new Promise((r) => setTimeout(r, 25));
    await useStore.getState().stop();
    const s = useStore.getState();
    assert.equal(s.status, "cancelled");
    assert.equal(s.agentGraphSnapshot?.loopState.status, "stopped");
  });
}

// --- degraded / race ---
test("graph refresh failure still leaves status blocked (not Failed)", async () => {
  seedRun(
    makeClient({
      sendTurn: () =>
        (async function* (): AsyncIterable<ProviderEventDTO> {
          throw awaitingUserError("code");
        })(),
      refreshAgentGraph: async () => {
        throw new Error("network down");
      },
    }),
  );
  await useStore.getState().sendPrompt("ok");
  await new Promise((r) => setTimeout(r, 25));
  assert.equal(useStore.getState().status, "blocked");
  assert.equal(useStore.getState().agentGraphSnapshot, undefined);
});

test("switching run discards late graph for previous blocked refresh", async () => {
  let resolveRefresh!: (snap: AgentGraphSnapshot) => void;
  const pending = new Promise<AgentGraphSnapshot>((r) => {
    resolveRefresh = r;
  });
  seedRun(
    makeClient({
      sendTurn: () =>
        (async function* (): AsyncIterable<ProviderEventDTO> {
          throw awaitingUserError("code");
        })(),
      refreshAgentGraph: async () => pending,
    }),
  );
  await useStore.getState().sendPrompt("ok");
  // Switch to another run before refresh resolves.
  useStore.setState({
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
  const s = useStore.getState();
  assert.equal(s.runId, "run-other");
  assert.equal(s.agentGraphSnapshot?.parentRunId, "run-other");
  assert.equal(s.agentGraphSnapshot?.loopState.status, "running");
});

test("late blocked refresh does not overwrite Stop", async () => {
  let resolveRefresh!: (snap: AgentGraphSnapshot) => void;
  const pending = new Promise<AgentGraphSnapshot>((r) => {
    resolveRefresh = r;
  });
  seedRun(
    makeClient({
      sendTurn: () =>
        (async function* (): AsyncIterable<ProviderEventDTO> {
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
    }),
  );
  await useStore.getState().sendPrompt("ok");
  await useStore.getState().stop();
  resolveRefresh(blockedGraph("run-63960", "codex"));
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(useStore.getState().agentGraphSnapshot?.loopState.status, "stopped");
});

// --- non-regression false positives ---
test("409 invalid_request still failed", async () => {
  seedRun(
    makeClient({
      sendTurn: () =>
        (async function* (): AsyncIterable<ProviderEventDTO> {
          throw new RunnerApiError(409, "invalid_request", "bad shape");
        })(),
    }),
  );
  await useStore.getState().sendPrompt("ok");
  assert.equal(useStore.getState().status, "failed");
});

test("500 with decision wording still failed", async () => {
  seedRun(
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

// --- status derivation + timeline settle for all block reasons ---
for (const blockReason of BLOCK_REASONS) {
  test(`derive+settle blocked ${blockReason}`, () => {
    const snap = blockedGraph("run-x", "codex", blockReason);
    assert.equal(deriveOrchestrationRunStatus("running", snap), "blocked");
    const timeline: TimelineItem[] = [
      { kind: "tool", id: "t1", toolName: "flowpilot__submit_review_outcome", status: "running" },
      { kind: "thinking", id: "th", text: "Thinking..." },
    ];
    const settled = settleCompletedFlowTimeline(timeline);
    assert.ok(!settled.some((it) => it.kind === "thinking"));
    assert.equal(settled.find((it) => it.kind === "tool")?.status, "success");
  });
}

// BUG-231 legacy: running child still wins over blocked loop for status.
test("running child still wins over blocked loop (BUG-231)", () => {
  const snap: AgentGraphSnapshot = {
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
  assert.equal(deriveOrchestrationRunStatus("running", snap), "running");
});
