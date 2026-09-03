import test from "node:test";
import assert from "node:assert/strict";
import { groupRunsByChatId, flattenGroupedHistory } from "./chatHistory";
import type { RunHistoryItem } from "../types/contract";

const item = (over: Partial<RunHistoryItem>): RunHistoryItem => ({
  runId: "run-x",
  projectId: "p",
  providerKey: "codex",
  status: "completed",
  startedAt: "",
  updatedAt: "",
  ...over,
});

test("groupRunsByChatId collapses legs under one chat with the latest head", () => {
  const rows = groupRunsByChatId([
    item({ runId: "run-1", chatId: "cht_a", legSeq: 0, providerKey: "codex" }),
    item({ runId: "run-2", chatId: "cht_a", legSeq: 1, providerKey: "grok" }),
    item({ runId: "run-wf" }),
  ]);
  assert.equal(rows.length, 2);
  assert.equal(rows[0].group?.legs.length, 2);
  assert.equal(rows[0].item.runId, "run-2"); // latest leg is the face
  assert.equal(rows[0].group?.legs[0].runId, "run-1"); // legs keep insertion order (oldest first)
  assert.equal(rows[0].group?.legs[1].runId, "run-2");
  assert.equal(rows[1].group, undefined); // untagged passes through 1:1
});

test("flattenGroupedHistory exposes a legsCount chip", () => {
  const rows = groupRunsByChatId([
    item({ runId: "run-1", chatId: "cht_a", legSeq: 0 }),
    item({ runId: "run-2", chatId: "cht_a", legSeq: 1 }),
  ]);
  const flat = flattenGroupedHistory(rows);
  assert.equal(flat.length, 1);
  assert.equal(flat[0].legsCount, 2);
});
