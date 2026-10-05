// BUG-1193: focusing an agent/coder run replays its persisted backlog via
// consumeAgentStream, which used to apply every frame through its own set()
// calls (3 store updates per event). A coder leg with thousands of events
// therefore rendered thousands of times on open — the "opening the coder chat
// lags" report. The fix mirrors consumeHistoryReplayStream: buffered apply of
// the persisted backlog (one set()), per-event handling only for the live tail.
import test from "node:test";
import assert from "node:assert/strict";
import { useStore, type TimelineItem } from "./store";
import type { ProviderEventDTO, ProviderKey, RunnerClient } from "../types/contract";

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

const BASE_EVENT = {
  id: "evt",
  workflowRunId: "child-coder",
  seq: 0,
  providerSessionId: "ses-fix",
  occurredAt: "2026-01-01T00:00:00Z",
  ts: "2026-07-23T21:00:00Z",
  providerKey: "codex" as ProviderKey,
};

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

function seed(client: RunnerClient, extras: Record<string, unknown> = {}): void {
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
    timeline: [{ kind: "prompt", id: "p1", text: "hub timeline" }] as TimelineItem[],
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

function bigBacklog(count: number): ProviderEventDTO[] {
  const events: ProviderEventDTO[] = [
    {
      ...BASE_EVENT,
      seq: 1,
      type: "turn_started",
      providerTurnId: "child-turn",
      prompt: "implement the thing",
    } as ProviderEventDTO,
  ];
  for (let i = 2; i <= count; i++) {
    events.push({
      ...BASE_EVENT,
      seq: i,
      type: "message_delta",
      text: `chunk-${i} `,
    } as ProviderEventDTO);
  }
  events.push({
    ...BASE_EVENT,
    seq: count + 1,
    type: "turn_completed",
    finalMessage: `chunk-${count} `,
  } as ProviderEventDTO);
  return events;
}

test("BUG-1193: focusAgentRun batches the persisted backlog instead of one render storm per event", async () => {
  const N = 120;
  const events = bigBacklog(N);
  seed(
    makeClient({
      resumeRun: async (runId) => ({
        runId,
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "completed",
        lastEventSeq: N + 1,
      }),
      focusAgentRun: async function* (): AsyncIterable<ProviderEventDTO> {
        for (const e of events) yield e;
      },
    }),
  );

  let updates = 0;
  const unsubscribe = useStore.subscribe(() => {
    updates++;
  });
  await useStore.getState().focusAgentRun("child-coder");
  await new Promise((resolve) => setTimeout(resolve, 20));
  unsubscribe();

  // Whole backlog lands via the buffered path: a handful of updates for setup,
  // flush, and settle — never one-per-event.
  assert.ok(
    updates <= 20,
    `expected batched replay (<=20 store updates), got ${updates} for ${N} backlog events`,
  );
  const assistant = useStore
    .getState()
    .timeline.filter((it) => it.kind === "assistant")
    .map((it) => (it.kind === "assistant" ? it.text : ""))
    .join("");
  assert.ok(assistant.includes(`chunk-${N}`), "replayed backlog still lands on the timeline");
});

test("BUG-1193: live tail past the durable boundary still applies per-event", async () => {
  const backlog = bigBacklog(4);
  const liveTail: ProviderEventDTO = {
    ...BASE_EVENT,
    seq: 99,
    type: "message_delta",
    text: "live-after-boundary ",
  } as ProviderEventDTO;
  seed(
    makeClient({
      resumeRun: async (runId) => ({
        runId,
        providerSessionId: "session-1",
        providerKey: "codex",
        status: "running",
        lastEventSeq: backlog.length,
      }),
      focusAgentRun: async function* (): AsyncIterable<ProviderEventDTO> {
        for (const e of backlog) yield e;
        yield liveTail;
      },
    }),
  );

  await useStore.getState().focusAgentRun("child-coder");
  await new Promise((resolve) => setTimeout(resolve, 20));

  const assistant = useStore
    .getState()
    .timeline.filter((it) => it.kind === "assistant")
    .map((it) => (it.kind === "assistant" ? it.text : ""))
    .join("");
  assert.ok(assistant.includes("live-after-boundary"), "live tail event reached the timeline");
});
