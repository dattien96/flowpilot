import test from "node:test";
import assert from "node:assert/strict";
import { deriveBoardSections } from "./boardModel";
import type { RunHistoryItem, Project } from "../types/contract.js";
import type { AttentionItem } from "./attentionQueue.js";

// Task-422: board derivation — pure model over projectHistoryById +
// attentionItems. Component wiring is smoke-tested live; all grouping,
// ordering, and flag logic is covered here.

function hist(runId: string, projectId: string, over: Partial<RunHistoryItem> = {}): RunHistoryItem {
  return {
    runId,
    projectId,
    providerKey: "codex",
    status: "running",
    startedAt: "2026-02-14T00:00:00Z",
    updatedAt: "2026-02-14T01:00:00Z",
    lastPrompt: `prompt for ${runId}`,
    ...over,
  } as RunHistoryItem;
}

function proj(id: string, name = id): Project {
  return { id, name, path: `/p/${id}` } as Project;
}

test("board groups runs by project and marks focused row", () => {
  const sections = deriveBoardSections(
    {
      p1: [hist("r1", "p1"), hist("r2", "p1")],
      p2: [hist("r3", "p2")],
    },
    [],
    [proj("p1", "Alpha"), proj("p2", "Beta")],
    "r3",
  );
  assert.equal(sections.length, 2);
  assert.equal(sections[0].projectName, "Alpha");
  assert.equal(sections[0].rows.length, 2);
  assert.equal(sections[1].rows.length, 1);
  assert.equal(sections[1].rows[0].isFocused, true);
  assert.equal(sections[0].rows[0].isFocused, false);
});

test("board row carries waitingKind only for waiting runs", () => {
  const attention: AttentionItem[] = [
    { runId: "r2", chatId: "c2", projectId: "p1", runTitle: "t", kind: "approval", waitingSince: "2026-02-14T01:00:00Z" },
  ];
  const sections = deriveBoardSections({ p1: [hist("r1", "p1"), hist("r2", "p1", { status: "waiting_approval" })] }, attention, [proj("p1")], null);
  const rows = sections[0].rows;
  assert.equal(rows.find((r) => r.runId === "r2")?.waitingKind, "approval");
  assert.equal(rows.find((r) => r.runId === "r1")?.waitingKind, undefined);
});

test("board orders sections by project registry order, runs by updatedAt desc", () => {
  const sections = deriveBoardSections(
    {
      // p2 listed first in the map but p1 first in registry — registry wins.
      p2: [hist("r3", "p2")],
      p1: [
        hist("old", "p1", { updatedAt: "2026-02-14T00:10:00Z" }),
        hist("new", "p1", { updatedAt: "2026-02-14T02:00:00Z" }),
        hist("mid", "p1", { updatedAt: "2026-02-14T01:00:00Z" }),
      ],
    },
    [],
    [proj("p1"), proj("p2")],
    null,
  );
  assert.deepEqual(sections.map((s) => s.projectId), ["p1", "p2"]);
  assert.deepEqual(sections[0].rows.map((r) => r.runId), ["new", "mid", "old"]);
});

test("board shows worktreeBound when worktreeSlug or worktreePath present", () => {
  const sections = deriveBoardSections(
    {
      p1: [
        hist("r1", "p1", { worktreeSlug: "chat-x" }),
        hist("r2", "p1", { worktreePath: "/p/p1/.flowpilot/worktrees/r2" }),
        hist("r3", "p1"),
      ],
    },
    [],
    [proj("p1")],
    null,
  );
  const rows = sections[0].rows;
  assert.equal(rows.find((r) => r.runId === "r1")?.worktreeBound, true);
  assert.equal(rows.find((r) => r.runId === "r2")?.worktreeBound, true);
  assert.equal(rows.find((r) => r.runId === "r3")?.worktreeBound, false);
});

test("board empty state when no project history loaded", () => {
  assert.deepEqual(deriveBoardSections({}, [], [proj("p1")], null), []);
  assert.deepEqual(deriveBoardSections({ p1: [] }, [], [proj("p1")], null), []);
});

test("board includes history slices for projects missing from registry", () => {
  const sections = deriveBoardSections(
    { ghost: [hist("r9", "ghost")] },
    [],
    [proj("p1")],
    null,
  );
  assert.equal(sections.length, 1);
  assert.equal(sections[0].projectId, "ghost");
});

test("row title falls back to runId and chatId falls back to runId", () => {
  const sections = deriveBoardSections(
    { p1: [hist("r1", "p1", { lastPrompt: "  ", lastMessage: undefined, chatId: undefined })] },
    [],
    [proj("p1")],
    null,
  );
  assert.equal(sections[0].rows[0].runTitle, "r1");
  assert.equal(sections[0].rows[0].chatId, "r1");
});
