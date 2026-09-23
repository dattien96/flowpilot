import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { TIMELINE_WINDOW_MAX, type TimelineItem } from "./timelineReducer";
import type { ChatTranscriptRecord, ProviderEventDTO, RunHistoryItem, RunnerClient } from "../types/contract";

// Task-421 (windowed timeline store), store-level: openHistoryRun must fetch
// only the transcript TAIL page; loadEarlierTimeline must prepend strictly
// older records, advance the backward-paging anchor, and never duplicate rows
// at the seam between paged transcript items and live-rendered rows.
// New file — additive only.

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

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
    refreshAgentGraph: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } }),
    pauseAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "paused", round: 0, roundCap: 3 } }),
    resumeAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } }),
    injectAgentFeedback: async (_p, _t, message) => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [{ id: "bus-1", parentRunId: "current-run", kind: "user-feedback", message, queued: true, occurredAt: "2026-01-01T00:00:00Z" }], loopState: { status: "running", round: 0, roundCap: 3 } }),
    stopAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "stopped", round: 0, roundCap: 3 } }),
    spawnAgent: async () => ({ runId: "agent-1", providerSessionId: "session-agent", providerKey: "codex", status: "completed" }),
    startRun: async () => ({ runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running" }),
    resumeRun: async (runId) => ({ runId, providerSessionId: "session-1", providerKey: "codex", status: "completed", chatId: "cht-1" }),
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
    syncChatRun: async (runId) => ({ runId, sourceMachineId: "mch", sourceRunId: runId, syncStatus: "synced", syncedAt: "2026-06-17T10:10:00Z", remotePath: "p" }),
    deleteRun: async () => {},
    restoreChatRun: async (input) => ({ runId: input.sourceRunId, sourceMachineId: input.sourceMachineId, sourceRunId: input.sourceRunId, providerKey: "codex", restoreStatus: "restored" }),
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

function seedWindowedState(client: RunnerClient, extras: Record<string, unknown> = {}): void {
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
    selectedProvider: "claude",
    selectedModel: undefined,
    reasoningEffort: undefined,
    yoloMode: false,
    chatStartMode: "normal",
    chatSourceDocId: "",
    runId: "run-1",
    chatId: "cht-1",
    mainRunId: "run-1",
    activeAgentRunId: undefined,
    agentRuns: [],
    agentBusMessages: [],
    activeStepId: "chat-run-1",
    status: "running",
    timeline: [],
    artifacts: [],
    runHistory: [],
    projectHistoryById: {},
    historyLoading: false,
    historyLoadError: undefined,
    pendingApprovals: [],
    pendingQuestions: [],
    lastTurnInput: undefined,
    latestTokenUsage: undefined,
    recoverable: false,
    scenario: "normal",
    _streamingAssistantId: undefined,
    _streamRunSeq: 0,
    _runSnapshots: {},
    _runReplaySeq: {},
    _historyReplaying: false,
    timelineHasOlder: true,
    _timelineAnchorSeq: 50,
    _timelineLoadingEarlier: false,
    _timelineEvictedIds: new Set<string>(),
    ...extras,
  } as never);
}

function chatRec(chatSeq: number, type: string, payload: unknown, legRunId = "run-0"): ChatTranscriptRecord {
  return { chatId: "cht-1", chatSeq, legRunId, type, payload };
}

test("loadEarlierTimeline prepends a backward page, advances the anchor, and pins paged rows", async () => {
  const calls: { beforeSeq?: number; limit?: number }[] = [];
  seedWindowedState(
    makeClient({
      chatTimeline: async (_chatId, _afterSeq, limit, beforeSeq) => {
        calls.push({ beforeSeq, limit });
        return {
          chatId: "cht-1",
          legs: [],
          records: [
            chatRec(46, "turn_started", { prompt: "older q" }),
            chatRec(47, "message_completed", { text: "older a" }),
          ],
          nextSeq: 47,
          truncated: true,
          degraded: false,
        };
      },
    }),
    {
      timeline: [{ kind: "prompt", id: "prompt-live", text: "latest prompt" } as TimelineItem],
    },
  );

  await useStore.getState().loadEarlierTimeline();

  const s = useStore.getState();
  assert.equal(calls.length, 1, "one backward fetch");
  assert.equal(calls[0].beforeSeq, 50, "pages from the anchor");
  assert.equal(s.timeline.length, 3);
  assert.equal(s.timeline[0].kind, "prompt");
  assert.equal((s.timeline[0] as { text: string }).text, "older q");
  assert.equal(s.timeline[2].id, "prompt-live", "retained rows stay at the tail");
  assert.equal(s._timelineAnchorSeq, 46, "anchor moves to the oldest materialized seq");
  assert.equal(s.timelineHasOlder, true, "truncated page keeps hasOlder");
  assert.equal((s.timeline[0] as { pinned?: boolean }).pinned, true, "paged rows are pinned against eviction");
  assert.equal(s._timelineLoadingEarlier, false);
});

test("loadEarlierTimeline is a no-op once the transcript is exhausted", async () => {
  let calls = 0;
  seedWindowedState(
    makeClient({
      chatTimeline: async () => {
        calls++;
        return { chatId: "cht-1", legs: [], records: [], nextSeq: 0, truncated: false, degraded: false };
      },
    }),
    { timelineHasOlder: false, _timelineAnchorSeq: 10 },
  );

  await useStore.getState().loadEarlierTimeline();
  assert.equal(calls, 0, "exhausted transcript must not fetch again");
});

test("loadEarlierTimeline first page uses beforeSeq=-1 when no anchor exists", async () => {
  const calls: (number | undefined)[] = [];
  seedWindowedState(
    makeClient({
      chatTimeline: async (_c, _a, _l, beforeSeq) => {
        calls.push(beforeSeq);
        return {
          chatId: "cht-1",
          legs: [],
          records: [chatRec(90, "message_completed", { text: "tail rec" })],
          nextSeq: 90,
          truncated: false,
          degraded: false,
        };
      },
    }),
    { _timelineAnchorSeq: undefined },
  );

  await useStore.getState().loadEarlierTimeline();
  assert.equal(calls[0], -1, "anchorless first page fetches the latest page");
  const s = useStore.getState();
  assert.equal(s.timelineHasOlder, false, "non-truncated page ends paging");
  assert.equal(s._timelineAnchorSeq, 90);
});

test("loadEarlierTimeline does not duplicate a retained row at the seam", async () => {
  // A paged transcript record whose text matches a live-rendered row (the
  // window's head overlaps the fetched page) must not appear twice.
  seedWindowedState(
    makeClient({
      chatTimeline: async () => ({
        chatId: "cht-1",
        legs: [],
        records: [
          chatRec(48, "turn_started", { prompt: "same prompt" }),
          chatRec(49, "message_completed", { text: "same answer" }),
        ],
        nextSeq: 49,
        truncated: false,
        degraded: false,
      }),
    }),
    {
      timeline: [
        { kind: "prompt", id: "prompt-local-7", text: "same prompt" },
        { kind: "assistant", id: "evt-live", text: "same answer", finalized: true },
      ] as TimelineItem[],
    },
  );

  await useStore.getState().loadEarlierTimeline();
  const s = useStore.getState();
  const promptCount = s.timeline.filter((it) => it.kind === "prompt" && it.text === "same prompt").length;
  const answerCount = s.timeline.filter((it) => it.kind === "assistant" && it.text === "same answer").length;
  assert.equal(promptCount, 1, "seam prompt deduped");
  assert.equal(answerCount, 1, "seam assistant deduped");
});

test("loadEarlierTimeline aborts cleanly when the run switched mid-fetch", async () => {
  let resolveFetch: (v: import("../types/contract").ChatTimelineResponse) => void = () => {};
  seedWindowedState(
    makeClient({
      chatTimeline: () => new Promise((res) => { resolveFetch = res; }),
    }),
    { timeline: [{ kind: "prompt", id: "p1", text: "x" } as TimelineItem] },
  );

  const pending = useStore.getState().loadEarlierTimeline();
  // Switch runs before the page resolves — stale write must be dropped.
  useStore.setState({ runId: "run-2", chatId: "cht-2" });
  resolveFetch({
    chatId: "cht-1",
    legs: [],
    records: [chatRec(40, "turn_started", { prompt: "stale" })],
    nextSeq: 40,
    truncated: true,
    degraded: false,
  });
  await pending;

  const s = useStore.getState();
  assert.equal(s.timeline.length, 1, "stale page not prepended to the new run");
  assert.equal(s.runId, "run-2");
});

test("openHistoryRun fetches only the transcript tail page and records window state", async () => {
  const calls: { beforeSeq?: number; limit?: number }[] = [];
  const records: ChatTranscriptRecord[] = [];
  // 40 prior-leg records — under the window, all materialize.
  for (let i = 1; i <= 40; i++) {
    records.push(chatRec(i, i % 2 === 1 ? "turn_started" : "message_completed", i % 2 === 1 ? { prompt: `q${i}` } : { text: `a${i}` }));
  }
  seedWindowedState(
    makeClient({
      chatTimeline: async (_chatId, _afterSeq, limit, beforeSeq) => {
        calls.push({ beforeSeq, limit });
        return { chatId: "cht-1", legs: [], records, nextSeq: 40, truncated: true, degraded: false };
      },
    }),
    { timelineHasOlder: false, _timelineAnchorSeq: undefined },
  );
  useStore.setState({
    runHistory: [
      {
        runId: "run-hist",
        projectId: "project-1",
        providerKey: "claude",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
        runKind: "chat",
        chatId: "cht-1",
      } as RunHistoryItem,
    ],
  });

  await useStore.getState().openHistoryRun("run-hist");

  const s = useStore.getState();
  assert.equal(calls.length, 1, "one transcript fetch on open");
  assert.equal(calls[0].beforeSeq, -1, "open fetches the latest page, not the full log");
  assert.equal(s.timeline.length, 40, "prior-leg records materialize");
  assert.equal(s.timelineHasOlder, true, "truncated tail page reports older history");
  assert.equal(s._timelineAnchorSeq, 1, "anchor = oldest materialized seq");
  assert.equal(s._timelineEvictedIds instanceof Set, true);
});

test("openHistoryRun bounds the hydrated timeline to the window for very long chats", async () => {
  const records: ChatTranscriptRecord[] = [];
  const total = TIMELINE_WINDOW_MAX + 120;
  for (let i = 1; i <= total; i++) {
    records.push(chatRec(i, "message_completed", { text: `answer-${i}` }));
  }
  seedWindowedState(
    makeClient({
      chatTimeline: async () => ({ chatId: "cht-1", legs: [], records, nextSeq: total, truncated: true, degraded: false }),
    }),
    { timelineHasOlder: false },
  );
  useStore.setState({
    runHistory: [
      {
        runId: "run-huge",
        projectId: "project-1",
        providerKey: "claude",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
        runKind: "chat",
        chatId: "cht-1",
      } as RunHistoryItem,
    ],
  });

  await useStore.getState().openHistoryRun("run-huge");

  const s = useStore.getState();
  assert.ok(s.timeline.length <= TIMELINE_WINDOW_MAX, `hydrated timeline bounded, got ${s.timeline.length}`);
  assert.equal(s.timelineHasOlder, true);
  // The tail must be the newest records, not the oldest.
  const tail = s.timeline[s.timeline.length - 1] as { text?: string };
  assert.equal(tail.text, `answer-${total}`);
});

test("openHistoryRun below the window keeps everything and reports hasOlder=false", async () => {
  const records = [
    chatRec(1, "turn_started", { prompt: "only q" }),
    chatRec(2, "message_completed", { text: "only a" }),
  ];
  seedWindowedState(
    makeClient({
      chatTimeline: async () => ({ chatId: "cht-1", legs: [], records, nextSeq: 2, truncated: false, degraded: false }),
    }),
  );
  useStore.setState({
    runHistory: [
      {
        runId: "run-small",
        projectId: "project-1",
        providerKey: "claude",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
        runKind: "chat",
        chatId: "cht-1",
      } as RunHistoryItem,
    ],
  });

  await useStore.getState().openHistoryRun("run-small");

  const s = useStore.getState();
  assert.equal(s.timeline.length, 2, "short chats materialize fully");
  assert.equal(s.timelineHasOlder, false, "non-truncated page → no Load earlier");
  assert.equal(s._timelineAnchorSeq, 1);
});
