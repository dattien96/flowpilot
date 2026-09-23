import "./localStorageTestStub";
import test from "node:test";
import assert from "node:assert/strict";
import {
  DRAFT_CAP,
  draftKeyFor,
  isEmptyDraft,
  loadDrafts,
  pruneDrafts,
  saveDrafts,
  type DraftState,
} from "./drafts";
import { useStore } from "./store";
import { RunnerApiError } from "@/client/HttpWsRunnerClient";
import type { RunHistoryItem } from "@/types/contract";

// Task-432 (CP-84 P-4): per-chat prompt drafts — key derivation, persistence,
// LRU cap, and store wiring (clear-on-send, prune-on-delete, survive reset).

function draft(text: string): DraftState {
  return { text, updatedAt: Date.now() };
}

function seedDrafts(entries: Record<string, DraftState>): void {
  useStore.setState({ drafts: { ...entries } });
}

function resetDrafts(): void {
  useStore.setState({ drafts: {} });
}

test("draftKeyFor: chatId wins, runId fallback, <projectId>:new for new chat", () => {
  assert.equal(draftKeyFor("c1", "r1", "p1"), "c1");
  assert.equal(draftKeyFor(null, "r1", "p1"), "r1");
  assert.equal(draftKeyFor(null, null, "p1"), "p1:new");
  assert.equal(draftKeyFor(null, null, null), "global:new");
});

test("draft survives selectProject + resetRun", async () => {
  seedDrafts({ "p1:new": draft("half-typed in A"), c1: draft("chat draft") });
  useStore.setState({ selectedProjectId: "p1" });
  // selectProject triggers resetRun() on project change — drafts must be
  // untouched by both.
  await useStore.getState().selectProject("p2");
  assert.equal(useStore.getState().drafts["p1:new"]?.text, "half-typed in A");
  assert.equal(useStore.getState().drafts["c1"]?.text, "chat draft");
  useStore.getState().resetRun();
  assert.equal(useStore.getState().drafts["p1:new"]?.text, "half-typed in A");
  resetDrafts();
});

test("send success clears only that chat's draft", async () => {
  const st = useStore.getState();
  seedDrafts({ "p1:new": draft("first message"), other: draft("keep me") });
  useStore.setState({
    projects: [{ id: "p1", name: "P1", path: "/tmp/p1" }] as never,
    selectedProjectId: "p1",
    selectedProvider: "codex",
    chatMode: "normal_chat",
    runId: undefined,
    chatId: undefined,
    timeline: [],
    status: "idle",
  });
  await st.sendPrompt("hello");
  assert.equal(useStore.getState().drafts["p1:new"], undefined);
  assert.equal(useStore.getState().drafts["other"]?.text, "keep me");
  resetDrafts();
});

test("send failure keeps draft", async () => {
  const st = useStore.getState();
  const client = st.client as unknown as {
    sendTurn: (input: unknown) => AsyncIterable<unknown>;
  };
  const original = client.sendTurn;
  client.sendTurn = async function* () {
    throw new RunnerApiError(422, "provider_unavailable", "nope");
  };
  try {
    seedDrafts({ "p1:new": draft("unsent text") });
    useStore.setState({
      projects: [{ id: "p1", name: "P1", path: "/tmp/p1" }] as never,
      selectedProjectId: "p1",
      selectedProvider: "codex",
      chatMode: "normal_chat",
      runId: undefined,
      chatId: undefined,
      timeline: [],
      status: "idle",
    });
    await st.sendPrompt("unsent text");
    const kept = useStore.getState().drafts["p1:new"];
    assert.ok(kept, "draft must be restored after a failed send");
    assert.equal(kept.text, "unsent text");
  } finally {
    client.sendTurn = original;
    resetDrafts();
  }
});

test("drafts persist to localStorage and reload", () => {
  saveDrafts({ k1: draft("persisted"), k2: draft("other") });
  const loaded = loadDrafts();
  assert.equal(loaded.k1?.text, "persisted");
  assert.equal(loaded.k2?.text, "other");
  localStorage.removeItem("fp:promptDrafts");
});

test("new-chat draft keyed '<projectId>:new' survives project switch", async () => {
  seedDrafts({ "p1:new": draft("draft for p1"), "p2:new": draft("draft for p2") });
  useStore.setState({ selectedProjectId: "p1" });
  await useStore.getState().selectProject("p2");
  assert.equal(useStore.getState().drafts["p1:new"]?.text, "draft for p1");
  assert.equal(useStore.getState().drafts["p2:new"]?.text, "draft for p2");
  resetDrafts();
});

test("deleting a chat prunes its draft keys", async () => {
  const item = {
    runId: "rDel",
    projectId: "p1",
    providerKey: "codex",
    status: "completed",
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    runKind: "chat",
    chatId: "cDel",
  } as RunHistoryItem;
  seedDrafts({ rDel: draft("by run"), cDel: draft("by chat"), keep: draft("untouched") });
  useStore.setState({ runHistory: [item], runId: undefined });
  await useStore.getState().deleteHistoryRun("rDel");
  assert.equal(useStore.getState().drafts["rDel"], undefined);
  assert.equal(useStore.getState().drafts["cDel"], undefined);
  assert.equal(useStore.getState().drafts["keep"]?.text, "untouched");
  resetDrafts();
});

test("draft map prunes oldest beyond DRAFT_CAP", () => {
  const many: Record<string, DraftState> = {};
  for (let i = 0; i < DRAFT_CAP + 10; i++) {
    many[`k${i}`] = { text: `t${i}`, updatedAt: i };
  }
  saveDrafts(many);
  const loaded = loadDrafts();
  assert.equal(Object.keys(loaded).length, DRAFT_CAP);
  // Oldest 10 evicted, newest retained.
  assert.equal(loaded["k0"], undefined);
  assert.equal(loaded[`k${DRAFT_CAP + 9}`]?.text, `t${DRAFT_CAP + 9}`);
  localStorage.removeItem("fp:promptDrafts");
});

test("empty draft is not persisted", () => {
  useStore.getState().setDraft("kEmpty", { text: "  ", updatedAt: 1 });
  assert.equal(useStore.getState().drafts["kEmpty"], undefined);
  assert.ok(isEmptyDraft({ text: "", updatedAt: 1 }));
  assert.ok(!isEmptyDraft({ text: "x", updatedAt: 1 }));
  resetDrafts();
});

test("pruneDrafts retains only the keep set", () => {
  const pruned = pruneDrafts(
    { a: draft("1"), b: draft("2"), c: draft("3") },
    new Set(["a", "c"]),
  );
  assert.deepEqual(Object.keys(pruned).sort(), ["a", "c"]);
});
