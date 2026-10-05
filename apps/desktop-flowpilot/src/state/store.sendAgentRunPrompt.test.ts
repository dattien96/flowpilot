/**
 * BUG-1191 — the focused member-run composer. Before this fix a focused
 * child transcript was read-only: the only un-stick path for a parked or
 * waiting member that owed work (no gate card to answer) was a hand-rolled
 * POST /turns against the member's run id — an API call no desktop user can
 * make. sendAgentRunPrompt posts the turn to the FOCUSED run id and streams
 * the reply into that transcript.
 */
import test from "node:test";
import assert from "node:assert/strict";
import { useStore, type TimelineItem } from "./store";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import type { ProviderEventDTO, RunnerClient, TurnInput } from "../types/contract";

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

function seedFocusedChild(client: RunnerClient): void {
  useStore.setState({
    client,
    projects: [{ id: "project-1", name: "p", path: "/tmp/p" }] as never,
    workflows: [],
    steps: [],
    skills: [],
    providerAccounts: [],
    supportedModels: [],
    selectedProjectId: "project-1",
    launchMode: "workflow",
    chatMode: "normal_chat",
    selectedProvider: "codex",
    runId: "child-rev",
    mainRunId: "run-parent",
    activeAgentRunId: "child-rev",
    activeStepId: "reviewer",
    chatId: "chat-child",
    agentRuns: [
      {
        runId: "child-rev",
        agentName: "reviewer",
        role: "reviewer",
        status: "completed",
        agentStatus: "waiting_user_approval",
        parentRunId: "run-parent",
        createdAt: "2026-10-05T00:00:00Z",
        providerKey: "codex",
      },
    ] as never,
    status: "running",
    timeline: [] as TimelineItem[],
    runHistory: [],
    drafts: {},
    _streamRunSeq: 0,
    _runSnapshots: {},
    _runReplaySeq: {},
  });
}

test("sendAgentRunPrompt posts the turn to the focused child run id", async () => {
  let captured: TurnInput | undefined;
  const client = {
    sendTurn: (input: TurnInput) => {
      captured = input;
      return emptyStream();
    },
    loadRunHistory: async () => ({ runs: [], attention: [] }) as never,
  } as unknown as RunnerClient;
  seedFocusedChild(client);

  await useStore.getState().sendAgentRunPrompt("please re-check scope");

  assert.ok(captured, "sendTurn was not invoked");
  assert.equal(captured!.runId, "child-rev");
  assert.equal(captured!.stepId, "reviewer");
  assert.equal(captured!.prompt, "please re-check scope");
  assert.ok(captured!.idempotencyKey, "idempotency key must be set");
  const s = useStore.getState();
  assert.ok(
    s.timeline.some((it) => it.kind === "prompt" && it.text === "please re-check scope"),
    "prompt bubble missing from the focused transcript",
  );
});

test("sendAgentRunPrompt falls back to sendPrompt when nothing child-focused", async () => {
  let sent: TurnInput | undefined;
  const client = {
    sendTurn: (input: TurnInput) => {
      sent = input;
      return emptyStream();
    },
    loadRunHistory: async () => ({ runs: [], attention: [] }) as never,
  } as unknown as RunnerClient;
  seedFocusedChild(client);
  // Focus is on the MAIN run — the child path must not fire.
  useStore.setState({ runId: "run-parent", activeAgentRunId: "run-parent" });

  await useStore.getState().sendAgentRunPrompt("hello main");

  assert.ok(sent, "sendTurn was not invoked");
  assert.equal(sent!.runId, "run-parent");
});

test("sendAgentRunPrompt maps flow_awaiting_user to blocked, not failed", async () => {
  const client = {
    sendTurn: () =>
      (async function* (): AsyncIterable<ProviderEventDTO> {
        throw new RunnerApiError(409, "flow_awaiting_user", "flow is waiting for your decision");
      })(),
    loadRunHistory: async () => ({ runs: [], attention: [] }) as never,
  } as unknown as RunnerClient;
  seedFocusedChild(client);

  await useStore.getState().sendAgentRunPrompt("nudge");

  const s = useStore.getState();
  assert.equal(s.status, "blocked");
  assert.ok(!s.timeline.some((it) => it.kind === "thinking"));
  assert.ok(s.timeline.some((it) => it.kind === "system" && it.tone === "warn"));
});
