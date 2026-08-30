import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import { groupRunsByChatId, flattenGroupedHistory } from "./chatHistory";
import type { RunHistoryItem } from "../types/contract";

// CP-59 Task-316 (CA-700): chat-scoped provider switch keeps the timeline,
// appends exactly one divider, mints exactly one leg per confirm, and the
// legacy Task-078 path stays verbatim when the chat SSOT is unknown.

const baseState = () => ({
  client: new MockRunnerClient(),
  selectedProjectId: "proj-1",
  selectedProvider: "codex" as const,
  selectedModel: "gpt-5.4",
  reasoningEffort: "medium",
  yoloMode: false,
  chatMode: "normal_chat" as const,
  runId: "run-1",
  chatId: "cht_a",
  providerSwitchLoading: false,
  pendingProviderSwitch: {
    sourceRunId: "run-1",
    sourceProviderKey: "codex" as const,
    sourceRunStatus: "idle" as const,
    targetProviderKey: "grok" as const,
    targetModel: "grok-4.5",
  },
});

test("provider switch keeps the timeline and appends exactly one divider", async () => {
  const client = new MockRunnerClient();
  const state = { ...baseState(), client };
  useStore.setState(state);
  useStore.setState({
    timeline: [
      { kind: "prompt", id: "u1", text: "hello ban la model gi" },
      { kind: "assistant", id: "a1", text: "toi la Muse Spark", finalized: true },
    ] as never,
  });

  await useStore.getState().confirmProviderSwitch();

  const s = useStore.getState();
  assert.equal(s.runId, `mock-run-cht_a-1`);
  assert.equal(s.chatId, "cht_a");
  assert.equal(s.providerSwitchLoading, false);
  assert.equal(s.pendingProviderSwitch, undefined);
  const dividers = s.timeline.filter((t) => t.id === "seed-divider-mock-run-cht_a-1");
  assert.equal(dividers.length, 1, "exactly one divider");
  assert.ok(String((dividers[0] as { text?: string }).text ?? "").includes("carried 2 turns (raw)"));
  // Transcript continuity: the two prior messages are still in place.
  assert.equal(s.timeline.length, 3);
  assert.equal(s.timeline[0].kind, "prompt");
});

test("double confirm mints one leg", async () => {
  const client = new MockRunnerClient();
  useStore.setState({ ...baseState(), client });
  const p1 = useStore.getState().confirmProviderSwitch();
  const p2 = useStore.getState().confirmProviderSwitch(); // second while in flight
  await Promise.allSettled([p1, p2]);
  assert.equal(useStore.getState().runId, "mock-run-cht_a-1");
});

test("operational state resets, artifacts retained", async () => {
  const client = new MockRunnerClient();
  useStore.setState({ ...baseState(), client, artifacts: [{ id: "art-1" }] as never });
  await useStore.getState().confirmProviderSwitch();
  const s = useStore.getState();
  assert.equal(s.pendingApprovals.length, 0);
  assert.equal(s.gateBlock, undefined);
  assert.equal((s.artifacts as unknown[]).length, 1, "artifacts must be retained");
});

test("legacy fallback path unchanged when the chat SSOT is unknown", async () => {
  const client = new MockRunnerClient();
  let handoffCalled = false;
  let startRunCalls = 0;
  client.handoffContext = async (runId, input) => {
    handoffCalled = true;
    return {
      sourceRunId: runId,
      sourceProviderKey: "codex",
      targetProviderKey: input.targetProviderKey,
      prompt: "[mock handoff]",
      includedTurnCount: 0,
      omittedTurnCount: 0,
      truncated: false,
      handoffMode: "raw",
    };
  };
  const origStart = client.startRun.bind(client);
  client.startRun = async (input) => {
    startRunCalls++;
    return origStart(input);
  };
  useStore.setState({ ...baseState(), client, chatId: undefined });
  useStore.setState({
    timeline: [{ kind: "prompt", id: "u1", text: "old" }] as never,
  });
  await useStore.getState().confirmProviderSwitch();
  assert.equal(handoffCalled, true);
  assert.equal(startRunCalls, 1);
  const s = useStore.getState();
  // Legacy path resets the timeline (Task-078 parity).
  assert.equal(s.timeline.filter((t) => t.id === "u1").length, 0);
  assert.equal(s.runId, "mock-run-1");
});

test("same-provider chip selection skips the endpoint (in-place)", async () => {
  const client = new MockRunnerClient();
  let switchCalls = 0;
  client.switchChatProvider = async (chatId, input) => {
    switchCalls++;
    throw new Error("should not be called");
  };
  useStore.setState({
    client,
    chatId: "cht_a",
    runId: "run-1",
    selectedProvider: "codex" as const,
    chatMode: "normal_chat" as const,
  });
  useStore.getState().selectProvider("codex");
  assert.equal(switchCalls, 0);
});

test("run history groups legs under one chat", () => {
  const rows = groupRunsByChatId([
    { runId: "run-1", projectId: "p", providerKey: "codex", status: "completed", startedAt: "", updatedAt: "", chatId: "cht_a", legSeq: 0 },
    { runId: "run-2", projectId: "p", providerKey: "grok", status: "running", startedAt: "", updatedAt: "", chatId: "cht_a", legSeq: 1 },
  ] as RunHistoryItem[]);
  const flat = flattenGroupedHistory(rows);
  assert.equal(flat.length, 1);
  assert.equal(flat[0].legsCount, 2);
});
