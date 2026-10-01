import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { draftKeyFor } from "./drafts";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
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
    flowStarted: false,
    drafts: {},
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

// --- Review-fix regressions (CP-89 desktop wiring, CA-1052 follow-up) ---

test("a detached reattach leg re-declares the armed pin and latch", async () => {
  const captured: Captured = { turns: [] };
  seedVibeChat();
  useStore.setState({
    client: makeClient(captured),
    // A chat restored from history has no live leg — the next prompt reattaches.
    runId: "run-old",
    chatId: "cht_armed",
    chatDetached: true,
    pendingFlowArm: { flowRef: "vibe-cp-ingest", sourceDocId: "requirements/07-coding-plan/CP-88-test.md" },
  } as never);

  await useStore.getState().sendPrompt("one more question");

  assert.equal(captured.startRun?.chatId, "cht_armed", "reattach rides the same chat");
  assert.equal(captured.startRun?.switchFromRunId, "run-old");
  assert.equal(captured.startRun?.flowArm, "pending", "the pending latch must travel to the new leg");
  assert.equal(captured.startRun?.flowRef, "vibe-cp-ingest");
  assert.equal(captured.startRun?.sourceDocId, "requirements/07-coding-plan/CP-88-test.md");
});

test("a reattach whose leg adopted a started latch clears the affordance", async () => {
  const captured: Captured = { turns: [] };
  // The runner echoes the latch it actually adopted on the new leg — a flow
  // that already started must never resurface as armed (double-launch risk).
  const client = makeClient(captured, { flowArm: "started", flowRef: "vibe-cp-ingest" });
  seedVibeChat();
  useStore.setState({
    client,
    runId: "run-old",
    chatId: "cht_armed",
    chatDetached: true,
    pendingFlowArm: { flowRef: "vibe-cp-ingest" },
  } as never);

  await useStore.getState().sendPrompt("ping");

  assert.equal(useStore.getState().pendingFlowArm, undefined, "stale arm must clear on a started echo");
  assert.equal(useStore.getState().flowStarted, true);
});

test("an unarmed vibe chat can late-attach a flow via the forward turn", async () => {
  const captured: Captured = { turns: [] };
  seedVibeChat();
  useStore.setState({
    client: makeClient(captured),
    // A plain chat already running — no armed pin, the user picks the flow at
    // Start-flow time (CP-89 late arm+forward).
    runId: "run-armed",
    chatId: "cht_armed",
    activeStepId: "chat-run-armed",
    pendingFlowArm: undefined,
  } as never);

  await useStore.getState().forwardArmedFlow("use the CP we just wrote", {
    flowRef: "vibe-cp-ingest",
    sourceDocId: "requirements/07-coding-plan/CP-88-test.md",
  });

  assert.equal(captured.turns.length, 1);
  const fwd = captured.turns[0];
  assert.equal(fwd.forwardFlow, true);
  assert.equal(fwd.flowRef, "vibe-cp-ingest", "the turn carries the final flow choice");
  assert.equal(fwd.sourceDocId, "requirements/07-coding-plan/CP-88-test.md");
  assert.ok(fwd.idempotencyKey, "forward carries an idempotency key for post-commit retries");
  assert.equal(useStore.getState().flowStarted, true, "a committed forward hides the picker");
  assert.equal(useStore.getState().pendingFlowArm, undefined, "late-attach failure was never armed");
});

test("a rejected forward restores the arm and the composed draft", async () => {
  const captured: Captured = { turns: [] };
  const client = makeClient(captured);
  client.sendTurn = (input: TurnInput) => {
    captured.turns.push(input);
    return (async function* () {
      throw new RunnerApiError(422, "invalid_cp_source", "bad CP path");
    })() as never;
  };
  seedVibeChat();
  useStore.setState({ client } as never);

  await useStore.getState().sendPrompt("discuss first");
  // sendPrompt's own failure also restores a draft under the same lane key —
  // clear it so the assert below isolates the FORWARD's restore.
  useStore.setState({ status: "idle", drafts: {} } as never);
  const key = draftKeyFor("cht_armed", "run-armed", "p1");
  await useStore.getState().forwardArmedFlow("go with this CP");

  const s = useStore.getState();
  assert.equal(s.status, "failed");
  assert.equal(s.pendingFlowArm?.flowRef, "vibe-cp-ingest", "arm restores so the user can retry");
  assert.equal(s.flowStarted, false);
  assert.equal(s.drafts[key]?.text, "go with this CP", "rejected forward must not lose the typed text");
});

test("a stale forward result never stamps failure onto the chat the user switched to", async () => {
  const captured: Captured = { turns: [] };
  const client = makeClient(captured);
  client.sendTurn = (input: TurnInput) => {
    captured.turns.push(input);
    return (async function* () {
      throw new RunnerApiError(422, "invalid_cp_source", "bad CP path");
    })() as never;
  };
  seedVibeChat();
  useStore.setState({ client } as never);

  await useStore.getState().sendPrompt("discuss first");
  useStore.setState({ status: "idle" } as never);
  // Fire, then immediately simulate the user opening a different chat before
  // the rejection lands — the late result must not touch run B's state.
  const p = useStore.getState().forwardArmedFlow("go");
  useStore.setState({
    runId: "run-b",
    chatId: "cht_b",
    status: "idle",
    pendingFlowArm: { flowRef: "vibe-ingest" },
    timeline: [],
    _streamRunSeq: useStore.getState()._streamRunSeq + 1,
  } as never);
  await p;

  const s = useStore.getState();
  assert.equal(s.status, "idle", "run B keeps its own status");
  assert.equal(s.pendingFlowArm?.flowRef, "vibe-ingest", "run B's own arm survives");
  assert.ok(!s.timeline.some((it) => it.id.startsWith("err-fwd-")), "no error row lands on run B");
});

// --- CA-1083: vibe-mode Workflow-tab picks arm pending (chat_then_forward)
// instead of immediate-launching — the user chats first, "Start flow" sends
// the forwardFlow turn. Fixes the live 422 invalid_cp_source on vibe-tasks
// (picker path never collected a CP source before firing the fence).

function seedVibeWorkflowTab(workflowId = "wf-tasks"): void {
  seedVibeChat();
  useStore.setState({
    chatMode: "workflow_step_auto",
    launchMode: "workflow",
    chatSourceDocId: "",
    selectedWorkflowId: workflowId,
    selectedModel: "gpt-5.5",
    projects: [{ id: "p1", name: "App", path: "/tmp/app", model: "devin/swe-2-high" } as never],
    workflows: [
      { id: "wf-tasks", projectId: "p1", name: "Vibe Tasks", packFlowId: "vibe-tasks", model: "devin/swe-2-high" },
      { id: "wf-cp", projectId: "p1", name: "Vibe CP Ingest", packFlowId: "vibe-cp-ingest" },
      { id: "wf-harness", projectId: "p1", name: "Task Harness", packFlowId: "task-harness" },
    ] as never,
  } as never);
}

test("vibe Workflow-tab pick of vibe-tasks arms pending as a chat run", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab();
  useStore.setState({ client: makeClient(captured) } as never);

  await useStore.getState().sendPrompt("which tasks does this CP cover?");

  const start = captured.startRun!;
  assert.equal(start.chatMode, "normal_chat", "armed pick is a chat run, not a workflow launch");
  assert.equal(start.workflowId, undefined, "no immediate workflowId execution");
  assert.equal(start.flowArm, "pending");
  assert.equal(start.flowRef, "vibe-tasks", "picker's packFlowId resolves to the bare flow ref");
  assert.equal(start.workingMode, "vibe");
  assert.equal(
    start.model,
    "devin/swe-2-high",
    "armed run stamps the flow's configured model — composer gpt-5.5 must not leak (live run-2198)",
  );
  assert.equal(captured.turns.length, 1);
  assert.equal(captured.turns[0].forwardFlow, undefined, "first chat turn must not forward");
  assert.equal(captured.turns[0].stepId, "chat-run-armed", "turn rides the run's synthetic chat step");
  assert.equal(useStore.getState().pendingFlowArm?.flowRef, "vibe-tasks");
  assert.equal(useStore.getState().flowStarted, false);
});

test("vibe Workflow-tab pick arms vibe-cp-ingest too", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab("wf-cp");
  useStore.setState({ client: makeClient(captured) } as never);

  await useStore.getState().sendPrompt("review this CP with me");

  assert.equal(captured.startRun?.flowArm, "pending");
  assert.equal(captured.startRun?.flowRef, "vibe-cp-ingest");
  assert.equal(captured.startRun?.workflowId, undefined);
  // No model on the wf-cp row → project default beats the composer pick.
  assert.equal(captured.startRun?.model, "devin/swe-2-high");
});

test("vibe Workflow-tab pick of a non-vibe flow stays fail-closed immediate", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab("wf-harness");
  useStore.setState({ client: makeClient(captured) } as never);

  await useStore.getState().sendPrompt("run it");

  // task-harness is not user-selectable in vibe — the send keeps the
  // workflowId path so the runner's create-time mode gate rejects it.
  assert.equal(captured.startRun?.workflowId, "wf-harness");
  assert.equal(captured.startRun?.flowArm, undefined);
  assert.equal(useStore.getState().pendingFlowArm, undefined);
});

test("dev-mode Workflow-tab launch is unchanged (immediate workflowId)", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab("wf-harness");
  useStore.setState({
    client: makeClient(captured),
    workingMode: "dev",
  } as never);

  await useStore.getState().sendPrompt("run it");

  assert.equal(captured.startRun?.workflowId, "wf-harness");
  assert.equal(captured.startRun?.chatMode, undefined, "workflow launch sends no chatMode");
  assert.equal(captured.startRun?.flowArm, undefined);
  assert.equal(useStore.getState().pendingFlowArm, undefined);
});

// --- CA-1084: the armed bar must show the instant a vibe flow is picked —
// no chat-first requirement — and Start flow on an empty session mints the
// armed run and forwards in one gesture.

test("selectWorkflow in vibe mode pre-arms the pick before any run", async () => {
  seedVibeWorkflowTab();
  useStore.setState({ client: makeClient({ turns: [] }) } as never);

  await useStore.getState().selectWorkflow("wf-tasks");

  assert.equal(useStore.getState().pendingFlowArm?.flowRef, "vibe-tasks",
    "the armed bar must render immediately — the run is minted on send or on Start flow");
});

test("selectWorkflow pre-arm never rewrites a live run's latch", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab();
  useStore.setState({ client: makeClient(captured) } as never);
  await useStore.getState().sendPrompt("hi");

  await useStore.getState().selectWorkflow("wf-cp");

  assert.equal(useStore.getState().pendingFlowArm?.flowRef, "vibe-tasks",
    "live arm keeps the run's pin; a re-pick only feeds the forward's final ref");
});

test("a pre-armed Start flow mints the run and forwards in one gesture", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab();
  useStore.setState({ client: makeClient(captured) } as never);
  await useStore.getState().selectWorkflow("wf-tasks");

  await useStore.getState().forwardArmedFlow("do tasks 21-25", {
    sourceDocId: "requirements/07-Coding-Plan/todo/CP-90-x.md",
  });

  assert.equal(captured.startRun?.chatMode, "normal_chat");
  assert.equal(captured.startRun?.flowArm, "pending");
  assert.equal(captured.startRun?.flowRef, "vibe-tasks");
  assert.equal(captured.startRun?.model, "devin/swe-2-high", "flow's configured model, not composer's");
  assert.equal(captured.startRun?.sourceDocId, "requirements/07-Coding-Plan/todo/CP-90-x.md");
  assert.equal(captured.turns.length, 1, "no wasted chat turn — the forward is the only turn");
  const fwd = captured.turns[0];
  assert.equal(fwd.forwardFlow, true);
  assert.equal(fwd.flowRef, "vibe-tasks");
  assert.equal(fwd.sourceDocId, "requirements/07-Coding-Plan/todo/CP-90-x.md");
  assert.equal(fwd.runId, "run-armed");
  assert.equal(useStore.getState().runId, "run-armed");
});

test("a CP picked before the run rides the armed create as the source pin", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab();
  useStore.setState({ client: makeClient(captured) } as never);
  await useStore.getState().selectWorkflow("wf-tasks");
  // The dropdown mirrors the pick onto the arm (setPendingFlowArmSourceDoc).
  useStore.getState().setPendingFlowArmSourceDoc("requirements/07-Coding-Plan/todo/CP-90-x.md");

  await useStore.getState().sendPrompt("walk me through the plan");

  assert.equal(captured.startRun?.flowArm, "pending");
  assert.equal(captured.startRun?.sourceDocId, "requirements/07-Coding-Plan/todo/CP-90-x.md");
  const s = useStore.getState();
  assert.equal(s.pendingFlowArm?.flowRef, "vibe-tasks");
  assert.equal(s.pendingFlowArm?.sourceDocId, "requirements/07-Coding-Plan/todo/CP-90-x.md",
    "the pick survives the re-seed after the run is minted");
});

test("a bar-only flow pick (no tab select) arms the first chat send", async () => {
  const captured: Captured = { turns: [] };
  seedVibeWorkflowTab();
  useStore.setState({
    client: makeClient(captured),
    selectedWorkflowId: undefined,
  } as never);
  // The composer's flow select mirrors a pre-run pick via armPendingFlow.
  useStore.getState().armPendingFlow("vibe-cp-ingest");

  await useStore.getState().sendPrompt("let's talk about the CP first");

  assert.equal(captured.startRun?.flowArm, "pending");
  assert.equal(captured.startRun?.flowRef, "vibe-cp-ingest");
  assert.equal(captured.startRun?.workflowId, undefined);
});

test("switching to dev with no run discards a pre-arm", async () => {
  seedVibeWorkflowTab();
  useStore.setState({ client: makeClient({ turns: [] }) } as never);
  await useStore.getState().selectWorkflow("wf-tasks");
  assert.equal(useStore.getState().pendingFlowArm?.flowRef, "vibe-tasks");

  useStore.getState().setWorkingMode("dev");

  assert.equal(useStore.getState().pendingFlowArm, undefined);
});
