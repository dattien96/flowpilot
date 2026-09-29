import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import type { RunnerClient, StartRunInput, TurnInput } from "../types/contract";

// CP-89 chat-then-forward (Task-452 desktop wiring): a vibe-mode first send
// creates the run with flowArm:"pending" — the pinned flow does NOT launch on
// the first turn, chat turns stay plain, and only an explicit
// forwardArmedFlow() call sends forwardFlow:true on the same run.

async function* emptyStream() {}

interface Captured {
  startRun?: StartRunInput;
  turns: TurnInput[];
}

function makeClient(captured: Captured, handleExtra: Record<string, unknown> = {}) {
  return {
    listProjects: async () => [],
    listWorkflows: async () => [],
    listSteps: async () => [],
    listProviderAccounts: async () => [],
    listRunHistory: async () => [],
    listRemoteChatSessions: async () => [],
    listAgents: async () => [],
    listAgentRuns: async () => [],
    listArtifacts: async () => [],
    listSkills: async () => [],
    listBuiltinOrchestrationOptions: async () => [],
    startRun: async (input: StartRunInput) => {
      captured.startRun = input;
      return {
        runId: "run-armed",
        providerSessionId: "ses-1",
        providerKey: "codex",
        status: "idle",
        stepId: "chat-run-armed",
        chatId: "cht_armed",
        legSeq: 0,
        flowRef: input.flowRef,
        flowArm: input.flowArm,
        ...handleExtra,
      } as never;
    },
    resumeRun: async (runId: string) => ({
      runId,
      providerSessionId: "ses-1",
      providerKey: "codex",
      status: "idle",
      stepId: `chat-${runId}`,
      chatId: "cht_armed",
      legSeq: 0,
      ...handleExtra,
    } as never),
    sendTurn: (input: TurnInput) => {
      captured.turns.push(input);
      return emptyStream() as never;
    },
    streamRun: () => emptyStream() as never,
    chatTimeline: async () => ({ chatId: "cht_armed", legs: [], records: [], nextSeq: 0, truncated: false, degraded: false } as never),
    getChatPosture: async () => ({ active: "code", profiles: {} } as never),
  } as unknown as RunnerClient;
}

function seedVibeChat(): void {
  useStore.setState({
    projects: [{ id: "p1", name: "App", path: "/tmp/app" } as never],
    selectedProjectId: "p1",
    selectedProjectPath: "/tmp/app",
    selectedProvider: "codex",
    chatMode: "normal_chat",
    workingMode: "vibe",
    chatSourceDocId: "requirements/07-coding-plan/CP-88-test.md",
    status: "idle",
    timeline: [],
    runId: undefined,
    chatId: undefined,
    mainRunId: undefined,
    pendingFlowArm: undefined,
    agentRuns: [],
    pendingApprovals: [],
    pendingQuestions: [],
    runHistory: [],
  } as never);
}

test("vibe-mode first send creates the run armed pending, not launched", async () => {
  const captured: Captured = { turns: [] };
  seedVibeChat();
  useStore.setState({ client: makeClient(captured) } as never);

  await useStore.getState().sendPrompt("discuss the plan with me first");

  assert.equal(captured.startRun?.flowArm, "pending", "vibe send must arm the flow pending");
  assert.equal(captured.startRun?.flowRef, "vibe-cp-ingest");
  assert.equal(captured.startRun?.sourceDocId, "requirements/07-coding-plan/CP-88-test.md");
  assert.equal(captured.turns.length, 1);
  assert.equal(captured.turns[0].forwardFlow, undefined, "first chat turn must not forward");
  assert.equal(useStore.getState().pendingFlowArm?.flowRef, "vibe-cp-ingest");
});

test("a second plain chat turn stays plain while the flow is armed", async () => {
  const captured: Captured = { turns: [] };
  seedVibeChat();
  useStore.setState({ client: makeClient(captured) } as never);

  await useStore.getState().sendPrompt("first message");
  await useStore.getState().sendPrompt("now write the SS doc");

  assert.equal(captured.turns.length, 2);
  assert.equal(captured.turns[1].forwardFlow, undefined, "discussion turns never forward");
  assert.equal(useStore.getState().pendingFlowArm?.flowRef, "vibe-cp-ingest", "still armed");
});

test("forwardArmedFlow sends forwardFlow:true on the same run and disarms", async () => {
  const captured: Captured = { turns: [] };
  seedVibeChat();
  useStore.setState({ client: makeClient(captured) } as never);

  await useStore.getState().sendPrompt("discuss first");
  // An empty stub stream leaves status "running"; the real runner emits
  // turn_completed which settles it — simulate that before forwarding.
  useStore.setState({ status: "idle" } as never);
  await useStore.getState().forwardArmedFlow("docs are ready, start");

  assert.equal(captured.turns.length, 2);
  const fwd = captured.turns[1];
  assert.equal(fwd.forwardFlow, true);
  assert.equal(fwd.prompt, "docs are ready, start");
  assert.equal(fwd.runId, "run-armed", "forward rides the same run — no second run minted");
  assert.equal(useStore.getState().pendingFlowArm, undefined, "armed affordance clears on forward");
});

test("forwardArmedFlow is a no-op with nothing armed or while busy", async () => {
  const captured: Captured = { turns: [] };
  seedVibeChat();
  useStore.setState({ client: makeClient(captured) } as never);

  await useStore.getState().forwardArmedFlow("nothing armed");
  assert.equal(captured.turns.length, 0, "no run + no arm → no turn");

  await useStore.getState().sendPrompt("first message");
  useStore.setState({ status: "running" } as never);
  await useStore.getState().forwardArmedFlow("double forward");
  assert.equal(captured.turns.length, 1, "busy composer must not forward");
});

test("openHistoryRun restores the armed affordance from a pending flowArm row", async () => {
  const captured: Captured = { turns: [] };
  const client = makeClient(captured, { flowArm: "pending", flowRef: "vibe-cp-ingest" });
  seedVibeChat();
  useStore.setState({ client, runId: "other-run" } as never);

  await useStore.getState().openHistoryRun("run-armed", {
    runId: "run-armed",
    projectId: "p1",
    chatId: "cht_armed",
    providerKey: "codex",
    status: "completed",
    startedAt: "2026-06-14T10:00:00.000Z",
    updatedAt: "2026-06-14T10:00:00.000Z",
    runKind: "chat",
    flowRef: "vibe-cp-ingest",
    flowArm: "pending",
  } as never);

  assert.equal(useStore.getState().pendingFlowArm?.flowRef, "vibe-cp-ingest");
});

test("openHistoryRun leaves pendingFlowArm unset for a started/completed latch", async () => {
  const captured: Captured = { turns: [] };
  const client = makeClient(captured, { flowArm: "started", flowRef: "vibe-cp-ingest" });
  seedVibeChat();
  useStore.setState({ client, runId: "other-run" } as never);

  await useStore.getState().openHistoryRun("run-armed", {
    runId: "run-armed",
    projectId: "p1",
    chatId: "cht_armed",
    providerKey: "codex",
    status: "completed",
    startedAt: "2026-06-14T10:00:00.000Z",
    updatedAt: "2026-06-14T10:00:00.000Z",
    runKind: "chat",
    flowRef: "vibe-cp-ingest",
    flowArm: "started",
  } as never);

  assert.equal(useStore.getState().pendingFlowArm, undefined);
});
